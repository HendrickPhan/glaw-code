# Sandbox and Security Model

## Package: `internal/runtime/sandbox/`

---

## Sandbox Overview

The sandbox system provides process isolation for tool execution (primarily bash). It uses Linux namespace isolation when available, with fallback to unrestricted execution.

---

## SandboxConfig

```go
type SandboxConfig struct {
    Enabled              *bool
    NamespaceRestrictions *bool
    NetworkIsolation     *bool
    FilesystemMode       *FilesystemIsolationMode
    AllowedMounts        []string
}

type FilesystemIsolationMode string
// "off"            - No filesystem isolation
// "workspace_only" - Only workspace directory accessible
// "allow_list"     - Only explicitly allowed paths accessible
```

---

## Container Environment Detection

### detect_container_environment()

```go
func DetectContainerEnvironment() ContainerEnvironment
```

**Detection checks (in order):**

| Check | Method | Details |
|-------|--------|---------|
| /.dockerenv exists | File existence | Docker creates this file |
| /run/.containerenv exists | File existence | Podman creates this file |
| Environment: container | os.Getenv | Generic container marker |
| Environment: docker | os.Getenv | Docker-specific |
| Environment: podman | os.Getenv | Podman-specific |
| Environment: kubernetes_service_host | os.Getenv | Kubernetes detection |
| /proc/1/cgroup contains "docker\|kubepods" | File content check | Container cgroup detection |

```go
type ContainerEnvironment struct {
    InContainer bool
    Markers     []string // which checks detected container
}
```

---

## Sandbox Status Resolution

### resolve_sandbox_status()

```go
func ResolveSandboxStatus(config SandboxConfig) SandboxStatus
```

**Logic:**
```
1. Determine platform: Linux vs other
2. IF not Linux:
   - All namespace features: supported=false, enabled=false
   - Filesystem mode: off
   - Network isolation: false
   - Return status (sandbox unavailable on non-Linux)

3. IF Linux:
   a. Namespace support:
      - supported: true (Linux has unshare)
      - enabled: config.Enabled != nil && *config.Enabled
   b. Network isolation:
      - supported: true
      - enabled: config.NetworkIsolation != nil && *config.NetworkIsolation
   c. Filesystem mode:
      - Use config.FilesystemMode (default: "off")
      - active: mode != "off"
   d. Build SandboxStatus with all 14 fields
```

### SandboxStatus

```go
type SandboxStatus struct {
    // Namespace
    NamespaceSupported bool
    NamespaceEnabled   bool
    NamespaceActive    bool

    // Network
    NetworkSupported   bool
    NetworkEnabled     bool
    NetworkActive      bool

    // Filesystem
    FilesystemSupported bool
    FilesystemMode      FilesystemIsolationMode
    FilesystemActive    bool
    AllowedMounts       []string

    // General
    ContainerDetected bool
    Platform          string
    OverallActive     bool
}
```

---

## Linux Sandbox Command Builder

### build_linux_sandbox_command()

```go
func BuildLinuxSandboxCommand(config SandboxConfig, command string, workDir string) (*LinuxSandboxCommand, error)
```

**Logic:**
```
1. Resolve sandbox status from config
2. IF !status.OverallActive: return command as-is (no sandbox)

3. Build unshare command:
   base = "unshare"
   args = []

4. Add namespace flags:
   args = append(args,
     "--user",          // User namespace
     "--map-root-user", // Map to root in namespace
     "--mount",         // Mount namespace
     "--ipc",           // IPC namespace
     "--pid",           // PID namespace
     "--uts",           // UTS (hostname) namespace
     "--fork",          // Fork before executing
   )

5. IF network isolation enabled:
   args = append(args, "--net")  // Network namespace

6. Set environment overrides:
   env["HOME"] = ".sandbox-home"
   env["TMPDIR"] = ".sandbox-tmp"

7. Build final command:
   args = append(args, "--", "/bin/sh", "-c", command)

8. Return LinuxSandboxCommand{
     Program: "unshare",
     Args:    args,
     Env:     env,
   }
```

---

## Permission Model Integration

### PermissionManager

```go
type PermissionManager struct {
    Mode         PermissionMode
    WorkspaceRoot string
    Allowed      map[Permission]bool
    Denied       map[Permission]bool
}
```

### CheckPermission

```go
func (m *PermissionManager) CheckPermission(perm Permission) bool
```

**Decision matrix:**

| Mode | read_file | write_file | edit_file | bash | glob/grep | web | agent | config |
|------|-----------|------------|-----------|------|-----------|-----|-------|--------|
| ReadOnly | Y | N | N | N | Y | Y | N | N |
| WorkspaceWrite | Y | Y* | Y* | N | Y | Y | N | Y* |
| DangerFullAccess | Y | Y | Y | Y | Y | Y | Y | Y |
| Prompt | ASK | ASK | ASK | ASK | ASK | ASK | ASK | ASK |
| Allow | Y | Y | Y | Y | Y | Y | Y | Y |

`*` = Workspace-scoped: file path must be under WorkspaceRoot

### Prompt Mode

When mode is "prompt", for each tool call:

```
1. Display tool name and input to user
2. Ask: "Allow? (y/n/always)"
3. IF "always": add tool to Allowed map, switch effective behavior to Allow for this tool
4. IF "y": allow this one execution
5. IF "n": deny, append tool_result with denial message
```

---

## Hook-Based Security

Hooks provide an additional security layer:

### Pre-Tool-Use Hook

```
Before EVERY tool execution:
1. Collect all pre_tool_use hooks from enabled plugins
2. Build JSON payload:
   {
     "event": "pre_tool_use",
     "tool_name": "{name}",
     "tool_input": {input_json}
   }
3. FOR EACH hook:
   Execute hook command with JSON on stdin
   IF exit code == 2: DENY tool execution
   IF exit code == 0: allow (continue checking)
   ELSE: warn (log but continue)
4. IF any hook denied: abort tool execution
```

### Post-Tool-Use Hook

```
After EVERY tool execution (even if tool errored):
1. Collect all post_tool_use hooks
2. Build JSON payload:
   {
     "event": "post_tool_use",
     "tool_name": "{name}",
     "tool_input": {input_json},
     "tool_output": "{output}",
     "tool_is_error": {bool}
   }
3. FOR EACH hook: execute with JSON on stdin
   (Denials are logged but don't undo the tool)
```

---

## Security Invariants

1. **File path validation:** All file tools require absolute paths. Write operations check the path is within workspace (unless DangerFullAccess).

2. **No --no-verify for git:** The commit command never passes `--no-verify` to git, ensuring pre-commit hooks always run.

3. **Explicit staging:** `git add` always uses specific filenames, never `git add -A` or `git add .`.

4. **Sandbox HOME/TMPDIR:** When sandboxed, HOME and TMPDIR are redirected to sandbox-specific directories.

5. **Network isolation:** When enabled, the sandbox creates a new network namespace, preventing network access from tool commands.

6. **Container awareness:** Container detection prevents nested sandboxing and adjusts behavior for containerized environments.
