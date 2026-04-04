# Tools — Built-in Tool Specifications and Execution Logic

## Package: `internal/tools/`

---

## ToolSpec Structure

Each tool is defined by a `ToolSpec`:

```go
type ToolSpec struct {
    Name              string
    Description       string
    InputSchema       map[string]interface{} // JSON Schema
    RequiredPermission PermissionMode
}
```

---

## Tool Execution Interface

```go
type ToolExecutor interface {
    ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error)
}

type ToolOutput struct {
    Content string
    IsError bool
}
```

---

## 19 Built-in Tools — Detailed Logic

### 1. bash

**Permission:** `DangerFullAccess`

**Input Schema:**
```json
{
    "command": {"type": "string", "description": "Shell command to execute"},
    "timeout": {"type": "integer", "description": "Timeout in milliseconds"},
    "working_dir": {"type": "string", "description": "Working directory"},
    "env": {"type": "object", "description": "Environment variables"}
}
```

**Execution Logic:**
```
1. Parse input: extract command, timeout, working_dir, env
2. IF sandbox enabled:
     Build sandbox command (see sandbox.md)
     Wrap command with unshare/namespace isolation
3. Create subprocess:
     - Command: "/bin/sh" with args ["-c", command]
     - WorkingDir: working_dir (default: workspace root)
     - Env: merge OS env + input env
     - Stdout/Stderr: capture combined
4. Start process with context deadline (timeout)
5. Wait for completion
6. Return stdout+stderr as content
7. IF exit code != 0: set IsError = true
```

---

### 2. read_file

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "path": {"type": "string", "description": "Absolute file path"},
    "offset": {"type": "integer", "description": "Start line (0-based)"},
    "limit": {"type": "integer", "description": "Max lines to read"}
}
```

**Execution Logic:**
```
1. Validate path is absolute
2. Open file
3. IF offset provided: skip to line N
4. Read up to limit lines (default: 2000)
5. Format with line numbers: "{line_num}\t{content}"
6. Return formatted content
```

---

### 3. write_file

**Permission:** `WorkspaceWrite`

**Input Schema:**
```json
{
    "path": {"type": "string", "description": "Absolute file path"},
    "content": {"type": "string", "description": "File content to write"}
}
```

**Execution Logic:**
```
1. Validate path is absolute
2. Create parent directories if needed (os.MkdirAll)
3. Write content to file (create or overwrite)
4. Return "File written: {path}"
```

---

### 4. edit_file

**Permission:** `WorkspaceWrite`

**Input Schema:**
```json
{
    "path": {"type": "string", "description": "Absolute file path"},
    "old_string": {"type": "string", "description": "Text to find"},
    "new_string": {"type": "string", "description": "Replacement text"},
    "replace_all": {"type": "boolean", "description": "Replace all occurrences"}
}
```

**Execution Logic:**
```
1. Read entire file content
2. Find old_string in content
3. IF not found: return error "old_string not found"
4. IF not replace_all AND old_string appears more than once:
     Return error "old_string is not unique, provide more context"
5. IF replace_all:
     Replace ALL occurrences of old_string with new_string
   ELSE:
     Replace first occurrence
6. Write modified content back to file
7. Return "Edited {path}: replaced {count} occurrence(s)"
```

---

### 5. glob_search

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "pattern": {"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
    "path": {"type": "string", "description": "Directory to search"}
}
```

**Execution Logic:**
```
1. Use filepath.Glob or recursive walk with pattern matching
2. Collect matching file paths
3. Sort by modification time (most recent first)
4. Return formatted list of paths
```

---

### 6. grep_search

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "pattern": {"type": "string", "description": "Regex pattern"},
    "path": {"type": "string", "description": "File or directory to search"},
    "glob": {"type": "string", "description": "File pattern filter"},
    "output_mode": {"type": "string", "enum": ["content", "files_with_matches", "count"]},
    "context_lines": {"type": "integer", "description": "Lines of context"}
}
```

**Execution Logic:**
```
1. Compile regex pattern
2. Walk directory tree (or read single file)
3. Filter by glob pattern if provided
4. For each file, search for regex matches
5. Format output based on output_mode:
   - "content": show matching lines with line numbers and context
   - "files_with_matches": just file paths
   - "count": match counts per file
6. Return formatted results (capped at 250 entries)
```

---

### 7. WebFetch

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "url": {"type": "string", "description": "URL to fetch"},
    "prompt": {"type": "string", "description": "What to extract from the page"}
}
```

**Execution Logic:**
```
1. HTTP GET the URL
2. Detect content type:
   - HTML: convert to markdown/text, extract title
   - JSON: pretty-print
   - Other text: return as-is
3. IF prompt provided: summarize/extract based on prompt
4. Return fetched content with metadata (title, URL)
```

---

### 8. WebSearch

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "query": {"type": "string", "description": "Search query"},
    "allowed_domains": {"type": "array", "items": {"type": "string"}},
    "blocked_domains": {"type": "array", "items": {"type": "string"}}
}
```

**Execution Logic:**
```
1. Build DuckDuckGo HTML search URL: "https://html.duckduckgo.com/html/?q={query}"
2. HTTP GET the URL
3. Parse HTML response, extract search result blocks
4. For each result: extract title, URL, snippet
5. Filter results:
   - IF allowed_domains: only include matching domains
   - IF blocked_domains: exclude matching domains
6. Deduplicate by URL
7. Format as markdown list with links
8. Return formatted results
```

---

### 9. TodoWrite

**Permission:** `WorkspaceWrite`

**Input Schema:**
```json
{
    "todos": {
        "type": "array",
        "items": {
            "type": "object",
            "properties": {
                "id": {"type": "string"},
                "subject": {"type": "string"},
                "status": {"type": "string", "enum": ["pending", "in_progress", "completed"]},
                "description": {"type": "string"}
            }
        }
    }
}
```

**Execution Logic:**
```
1. Parse todo list from input
2. Validate each todo has id, subject, status
3. Write todo list to .claw/todos.json (or in-memory)
4. Return formatted todo list summary
```

---

### 10. Skill

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "skill": {"type": "string", "description": "Skill name"},
    "args": {"type": "string", "description": "Skill arguments"}
}
```

**Execution Logic:**
```
1. Look up skill by name in skill registry
2. Load skill definition from .claw/skills/{name}.md (or bundled)
3. Execute skill logic (varies by skill)
4. Return skill output
```

---

### 11. Agent

**Permission:** `DangerFullAccess`

**Input Schema:**
```json
{
    "prompt": {"type": "string", "description": "Task for the sub-agent"},
    "type": {"type": "string", "enum": ["general-purpose", "Explore", "Plan", "Verification"]},
    "description": {"type": "string", "description": "Short task description"},
    "model": {"type": "string", "description": "Model override"},
    "isolation": {"type": "string", "enum": ["worktree"]}
}
```

**Execution Logic:**
```
1. Create AgentJob:
   - prompt from input
   - system_prompt: built from agent type
   - allowed_tools: filtered by agent type:
     * "Explore": read_file, glob_search, grep_search, web tools (read-only)
     * "Plan": read-only + todo_write
     * "Verification": read-only + bash (for tests)
     * "general-purpose": all tools except Agent itself
2. IF isolation == "worktree":
     Create git worktree for isolated execution
3. Spawn goroutine/agent:
   a. Create new ConversationRuntime with restricted tools
   b. Run agentic loop with the prompt
   c. Collect all output
4. Return agent output to caller
5. IF worktree: clean up (keep or remove based on result)
```

---

### 12. ToolSearch

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "query": {"type": "string", "description": "Search query"}
}
```

**Execution Logic:**
```
1. Get all available tools (built-in + MCP + plugins)
2. Case-insensitive match query against tool names and descriptions
3. Return matching tool specs
```

---

### 13. NotebookEdit

**Permission:** `WorkspaceWrite`

**Input Schema:**
```json
{
    "notebook_path": {"type": "string"},
    "cell_id": {"type": "string"},
    "cell_type": {"type": "string", "enum": ["code", "markdown"]},
    "new_source": {"type": "string"},
    "edit_mode": {"type": "string", "enum": ["replace", "insert", "delete"]}
}
```

**Execution Logic:**
```
1. Read .ipynb JSON file
2. Parse notebook structure (cells array)
3. Find target cell by cell_id (or use cell_number for insert)
4. Apply edit:
   - "replace": update cell source content
   - "insert": add new cell after specified cell
   - "delete": remove cell
5. Write modified notebook back to file
6. Return confirmation
```

---

### 14. Sleep

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "seconds": {"type": "integer", "description": "Duration in seconds"}
}
```

**Logic:** `time.Sleep(duration)`, return "Slept for {N}s"

---

### 15. SendUserMessage

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "message": {"type": "string"}
}
```

**Logic:** Display message to user via terminal output.

---

### 16. Config

**Permission:** `WorkspaceWrite`

**Input Schema:**
```json
{
    "action": {"type": "string", "enum": ["read", "write"]},
    "path": {"type": "string", "description": "Dot-notation config path"},
    "value": {"type": "any", "description": "Value to set (for write)"}
}
```

**Supported settings paths (16):**
model, permissions.mode, permissions.allow, permissions.deny, sandbox.enabled, hooks.pre_tool_use, hooks.post_tool_use, mcp_servers, plugins.enabled, plugins.external_dirs, appearance.theme, appearance.show_cost, streaming.enabled, api.base_url, api.timeout, api.max_retries

**Execution Logic:**
```
IF action == "read":
  Read settings.json
  Navigate dot-path
  Return value
IF action == "write":
  Read settings.json
  Navigate to parent of dot-path
  Set value at key
  Write back to settings.json
  Return "Updated {path}"
```

---

### 17. StructuredOutput

**Permission:** `ReadOnly`

**Input Schema:**
```json
{
    "data": {"type": "any"},
    "format": {"type": "string", "enum": ["json", "yaml", "csv"]}
}
```

**Logic:** Format the data value in the requested format and return.

---

### 18. REPL

**Permission:** `DangerFullAccess`

**Input Schema:**
```json
{
    "code": {"type": "string"},
    "language": {"type": "string", "enum": ["python", "node", "shell"]},
    "timeout": {"type": "integer"}
}
```

**Execution Logic:**
```
1. Select interpreter based on language:
   - "python": python3 with -c flag
   - "node": node with -e flag
   - "shell": /bin/sh -c
2. Execute code as subprocess with timeout
3. Capture stdout + stderr
4. Return output
```

---

### 19. PowerShell

**Permission:** `DangerFullAccess`

**Input Schema:**
```json
{
    "command": {"type": "string"},
    "timeout": {"type": "integer"},
    "run_in_background": {"type": "boolean"}
}
```

**Execution Logic:**
```
1. Find pwsh or powershell binary
2. IF run_in_background:
     Start process, return "Running in background (PID: {pid})"
     Store process reference for later output retrieval
   ELSE:
     Execute with timeout
     Return output
```

---

## GlobalToolRegistry

Manages all available tools across built-in, MCP, and plugin sources:

```go
type GlobalToolRegistry struct {
    builtinTools  map[string]ToolSpec
    mcpTools      map[string]McpTool
    pluginTools   map[string]PluginToolDefinition
    allowedNames  map[string]bool   // normalized name whitelist
    aliases       map[string]string // name -> canonical name
}
```

**Tool resolution order:**
1. Check aliases map for normalized name
2. Look up in builtinTools
3. Look up in mcpTools
4. Look up in pluginTools
5. Return error if not found
