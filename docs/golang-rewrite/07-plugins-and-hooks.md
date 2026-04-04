# Plugin System and Hook Execution

## Package: `internal/plugins/`

---

## Plugin Architecture Overview

Three plugin kinds:
- **Builtin:** Compiled into the binary (Go code)
- **Bundled:** Shipped with the distribution (pre-installed in bundle directory)
- **External:** User-installed from local path or git URL

---

## Plugin Manifest (`plugin.json`)

```json
{
    "name": "my-plugin",
    "version": "1.0.0",
    "description": "Plugin description",
    "permissions": ["read", "write", "execute"],
    "default_enabled": true,
    "hooks": {
        "pre_tool_use": [
            {"command": "sh validate.sh", "timeout": 5000}
        ],
        "post_tool_use": [
            {"command": "sh notify.sh", "timeout": 3000}
        ]
    },
    "lifecycle": {
        "init": "sh init.sh",
        "shutdown": "sh cleanup.sh"
    },
    "tools": [
        {
            "name": "my_tool",
            "description": "What it does",
            "input_schema": { ... },
            "required_permission": "workspace_write"
        }
    ],
    "commands": [
        {
            "name": "my-cmd",
            "aliases": ["mc"],
            "summary": "Command summary",
            "argument_hint": "[args]"
        }
    ]
}
```

### Manifest Validation Rules

```go
type ValidationError struct {
    Type    ValidationErrType
    Message string
}

// Error types:
// EmptyField         - name/version/description is empty
// EmptyEntryField    - tool/command name is empty
// InvalidPermission  - permission not in [read, write, execute]
// DuplicatePermission - duplicate in permissions list
// DuplicateEntry     - duplicate tool/command name
// MissingPath        - external plugin path doesn't exist
// InvalidToolInputSchema   - tool input_schema is not valid JSON Schema
// InvalidToolRequiredPerm  - tool permission not a valid mode
```

---

## PluginManager

```go
type PluginManager struct {
    Config    PluginManagerConfig
    Registry  *PluginRegistry
    Installed *InstalledPluginRegistry
}

type PluginManagerConfig struct {
    ConfigHome     string   // ~/.glaw
    EnabledPlugins []string // explicit enabled list (empty = all default_enabled)
    ExternalDirs   []string // additional plugin search paths
    InstallRoot    string   // where plugins are installed
    RegistryPath   string   // installed.json path
    BundledRoot    string   // bundled plugins source
}
```

### Plugin Lifecycle Methods

#### Discover()
```
1. Scan InstallRoot for plugin directories
2. FOR EACH directory:
   - Look for plugin.json in root OR .glaw-plugin/plugin.json
   - Parse manifest
   - Validate manifest
   - Create PluginDefinition (Bundled or External)
3. Scan ExternalDirs for additional plugins
4. Auto-sync bundled plugins:
   - Copy new bundled plugins to InstallRoot
   - Prune stale bundled entries from installed registry
5. Return all discovered plugins
```

#### Install(source, name)
```
IF source is LocalPath:
  1. Validate path exists
  2. Parse manifest from path
  3. Copy/symlink to InstallRoot/{name}
  4. Register in installed.json

IF source is GitUrl:
  1. Run git clone --depth 1 {url} {InstallRoot}/{name}
  2. Parse manifest from cloned directory
  3. Register in installed.json
```

#### Enable(name) / Disable(name)
```
1. Update settings.json:
   - Read current settings
   - Set plugins.{name}.enabled = true/false
   - Write back atomically
2. Plugin takes effect on next tool/command resolution
```

#### Uninstall(name)
```
1. Remove InstallRoot/{name} directory
2. Remove from installed.json
3. Remove from settings.json
```

#### Update(name)
```
IF plugin source is GitUrl:
  1. git -C {InstallRoot}/{name} pull
  2. Re-parse manifest
  3. Update registry
```

---

## Plugin Tool Execution

External plugin tools run as subprocesses:

```go
type PluginTool struct {
    PluginID   string
    PluginName string
    ToolName   string
    Command    string
}

func (t *PluginTool) Execute(ctx context.Context, input json.RawMessage) (*ToolOutput, error)
```

**Execution:**
```
1. Build subprocess command (from PluginTool.Command or manifest)
2. Set environment variables:
   - GLAW_PLUGIN_ID = {plugin_id}
   - GLAW_PLUGIN_NAME = {plugin_name}
   - GLAW_TOOL_NAME = {tool_name}
   - GLAW_TOOL_INPUT = {input_json}
3. Pipe input JSON to stdin
4. Capture stdout + stderr
5. Parse stdout as tool result
6. Return ToolOutput
```

---

## Hook System

### HookRunner

```go
type HookRunner struct {
    Hooks PluginHooks // merged from all active plugins
}
```

### Hook Events

```go
type HookEvent string
// "pre_tool_use" | "post_tool_use"
```

### Hook Execution Flow

#### RunPreToolUse

```go
func (r *HookRunner) RunPreToolUse(ctx context.Context, toolName string, input json.RawMessage) (*HookRunResult, error)
```

**Algorithm:**
```
1. Get merged pre_tool_use hooks from all enabled plugins
2. FOR EACH hook definition:
   a. Build JSON payload:
      {
        "event": "pre_tool_use",
        "tool_name": "{name}",
        "tool_input": {input}
      }
   b. Execute hook command:
      - Run as subprocess with JSON on stdin
      - Set env: HOOK_EVENT=pre_tool_use, HOOK_TOOL_NAME={name}, HOOK_TOOL_INPUT={input}
      - Timeout after hook.Timeout
   c. Check exit code:
      - 0: Allow (continue to next hook)
      - 2: Deny (abort tool execution)
      - Other/signal: Warn (log warning, continue)
   d. IF denied: capture denial message from stdout
3. Return HookRunResult:
   - Denied: true if any hook returned exit code 2
   - Messages: collected messages from hooks
```

#### RunPostToolUse

```go
func (r *HookRunner) RunPostToolUse(ctx context.Context, toolName string, input json.RawMessage, output *ToolOutput) error
```

**Algorithm:** Same as pre-tool but:
- JSON payload includes `tool_output` and `tool_is_error` fields
- Exit code 2 means "log denial but don't undo the tool"
- Environment includes HOOK_TOOL_OUTPUT and HOOK_TOOL_IS_ERROR

---

## Shell Command Resolution

```go
func shellCommand(cmd string) (string, []string)
```

**Logic:**
```
IF cmd ends with ".sh" or contains "/":
  // Treat as script file
  return "/bin/sh", [cmd]
ELSE:
  // Treat as literal command
  return "/bin/sh", ["-lc", cmd]
```

---

## Hook Merging

When multiple plugins define hooks for the same event:

```go
func (h PluginHooks) MergedWith(other PluginHooks) PluginHooks
```

**Logic:**
```
1. Concatenate pre_tool_use lists
2. Concatenate post_tool_use lists
3. Return merged PluginHooks
```

All hooks run in order. Any single denial aborts the entire chain.

---

## CommandWithStdin Helper

```go
type CommandWithStdin struct {
    Cmd  *exec.Cmd
    Stdin string // JSON to pipe to stdin
}

func (c *CommandWithStdin) Run(ctx context.Context) (*CommandResult, error)
```

**Logic:**
```
1. Create pipes for stdin, stdout, stderr
2. Start process
3. Write stdin data
4. Close stdin pipe
5. Read stdout and stderr concurrently
6. Wait for process to exit
7. Return CommandResult{ExitCode, Stdout, Stderr}
```
