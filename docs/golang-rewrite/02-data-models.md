# Data Models and Types

All types below correspond to the Rust implementation. Field names use Go conventions (PascalCase).

---

## API Types (`internal/api/`)

### MessageRole (enum)
```
MessageRole string // "user" | "assistant"
```

### StopReason (enum)
```
StopReason string // "end_turn" | "tool_use" | "max_tokens" | "stop_sequence"
```

### ContentBlock (sum type)
```go
type ContentBlock struct {
    Type      string          // "text" | "tool_use" | "tool_result"
    Text      string          // for Type=="text"
    ID        string          // for Type=="tool_use" or "tool_result"
    Name      string          // for Type=="tool_use" -- tool name
    Input     json.RawMessage // for Type=="tool_use" -- tool input JSON
    ToolUseID string          // for Type=="tool_result" -- references tool_use ID
    Content   string          // for Type=="tool_result" -- result text
    IsError   bool            // for Type=="tool_result"
}
```

### ApiMessage
```go
type ApiMessage struct {
    Role    string         // MessageRole
    Content []ContentBlock `json:"content"`
}
```

### ToolDefinition
```go
type ToolDefinition struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema"`
}
```

### Usage
```go
type Usage struct {
    InputTokens              int `json:"input_tokens"`
    OutputTokens             int `json:"output_tokens"`
    CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
    CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}
```

### ApiRequest
```go
type ApiRequest struct {
    Model     string           `json:"model"`
    Messages  []ApiMessage     `json:"messages"`
    Tools     []ToolDefinition `json:"tools,omitempty"`
    MaxTokens int              `json:"max_tokens"`
    Stream    bool             `json:"stream"`
    System    string           `json:"system,omitempty"`
    ToolChoice string          `json:"tool_choice,omitempty"` // "auto" | "any" | "none"
}
```

### ApiResponse
```go
type ApiResponse struct {
    ID         string        `json:"id"`
    Content    []ContentBlock `json:"content"`
    StopReason StopReason    `json:"stop_reason"`
    Usage      Usage         `json:"usage"`
    RequestID  string        `json:"-"` // from x-request-id header
}
```

---

## Session Types (`internal/runtime/`)

### ConversationMessage
```go
type ConversationMessage struct {
    Role    string         // "user" | "assistant"
    Blocks  []ContentBlock
    Usage   *TokenUsage    // nil for user messages
}
```

### Session
```go
type Session struct {
    Version  int
    Messages []ConversationMessage
}
```

**Persistence:** Saved as JSON to `.claw/sessions/{session-id}.json`. Load/save functions read/write the file directly.

**Compaction:** When message count exceeds threshold, old messages are summarized via an API call and replaced with a single summary message. Recent N messages are preserved.

---

## Configuration Types (`internal/runtime/`)

### Config
```go
type Config struct {
    Model           string
    APIKey          string
    BaseURL         string
    MaxTokens       int
    Temperature     float64
    SystemPromptPath string
    PermissionMode  PermissionMode
    SandboxConfig   *SandboxConfig
    MCPServers      map[string]MCPServerConfig
    PluginConfig    *PluginManagerConfig
    HookConfig      *HookConfig
}
```

### ConfigSource
```go
type ConfigSource string // "user" | "project" | "local"
```

### Config Loading Order
1. `~/.claw/config.toml` (user-level)
2. `.claw/config.toml` (project-level)
3. `.claw.json` (project-level, JSON format)
4. Environment variable overrides (CLAW_*, ANTHROPIC_*)

Deep merge: later sources override earlier ones. Maps are merged, scalars are replaced.

---

## Permission Types

### PermissionMode
```go
type PermissionMode string
// Values:
// "read_only"          - Only read/search tools allowed
// "workspace_write"    - File editing within workspace allowed
// "danger_full_access" - All tools unrestricted
// "prompt"             - Ask user for each tool call
// "allow"              - Everything allowed silently
```

### Permission
```go
type Permission string
// Values: "read_file", "write_file", "execute_command", "network", etc.
```

### PermissionManager
```go
type PermissionManager struct {
    Mode         PermissionMode
    WorkspaceRoot string
    AllowedOps   map[Permission]bool
}
```

**Logic:**
- `ReadOnly`: allows read_file, glob_search, grep_search, web_fetch, web_search
- `WorkspaceWrite`: all of above + write_file, edit_file, notebook_edit, todo_write
- `DangerFullAccess`: everything including bash, agent, repl, powershell
- `Prompt`: check each operation interactively
- `Allow`: always true

---

## Tool Types (`internal/tools/`)

### ToolSpec
```go
type ToolSpec struct {
    Name              string
    Description       string
    InputSchema       json.RawMessage // JSON Schema object
    RequiredPermission PermissionMode
}
```

### ToolRegistry
```go
type ToolRegistry struct {
    Entries []ToolManifestEntry
}

type ToolManifestEntry struct {
    Name   string
    Source ToolSource // "base" | "conditional"
}
```

### 19 Built-in Tools

| # | Name | Permission | Purpose |
|---|------|-----------|---------|
| 1 | bash | DangerFullAccess | Execute shell commands |
| 2 | read_file | ReadOnly | Read file contents |
| 3 | write_file | WorkspaceWrite | Write/create files |
| 4 | edit_file | WorkspaceWrite | String replacement editing |
| 5 | glob_search | ReadOnly | Pattern-based file search |
| 6 | grep_search | ReadOnly | Regex content search |
| 7 | WebFetch | ReadOnly | HTTP fetch with HTML-to-text |
| 8 | WebSearch | ReadOnly | DuckDuckGo HTML scraping |
| 9 | TodoWrite | WorkspaceWrite | Task list management |
| 10 | Skill | ReadOnly | Invoke named skills |
| 11 | Agent | DangerFullAccess | Spawn sub-agent threads |
| 12 | ToolSearch | ReadOnly | Search available tools |
| 13 | NotebookEdit | WorkspaceWrite | Jupyter notebook editing |
| 14 | Sleep | ReadOnly | Delay execution |
| 15 | SendUserMessage | ReadOnly | Send message to user |
| 16 | Config | WorkspaceWrite | Read/write settings |
| 17 | StructuredOutput | ReadOnly | Structured data output |
| 18 | REPL | DangerFullAccess | Code execution (python3/node/shell) |
| 19 | PowerShell | DangerFullAccess | pwsh/powershell execution |

---

## Command Types (`internal/commands/`)

### SlashCommandSpec
```go
type SlashCommandSpec struct {
    Name           string
    Aliases        []string
    Summary        string
    ArgumentHint   string
    ResumeSupported bool
    Category       SlashCommandCategory
}

type SlashCommandCategory string
// "core" | "workspace" | "session" | "git" | "automation"
```

### 28 Slash Commands

| # | Name | Category | Summary |
|---|------|----------|---------|
| 1 | help | Core | Show help |
| 2 | status | Core | Show runtime status |
| 3 | compact | Core | Compact conversation history |
| 4 | model | Core | Change or show model |
| 5 | permissions | Core | Show/change permission mode |
| 6 | clear | Core | Clear conversation |
| 7 | cost | Core | Show cost summary |
| 8 | resume | Session | Resume previous session |
| 9 | config | Core | Read/write configuration |
| 10 | memory | Core | Manage memory/context |
| 11 | init | Workspace | Initialize .claw directory |
| 12 | diff | Workspace | Show pending file changes |
| 13 | version | Core | Show version |
| 14 | bughunter | Automation | Bug hunting mode |
| 15 | branch | Git | Git branch operations |
| 16 | worktree | Git | Git worktree operations |
| 17 | commit | Git | Git commit |
| 18 | commit-push-pr | Git | Commit, push, create PR |
| 19 | pr | Git | Pull request operations |
| 20 | issue | Git | GitHub issue operations |
| 21 | ultraplan | Automation | Detailed planning mode |
| 22 | teleport | Core | Remote session teleport |
| 23 | debug-tool-call | Core | Debug tool call tracing |
| 24 | export | Session | Export session data |
| 25 | session | Session | Session management |
| 26 | plugin | Core | Plugin management |
| 27 | agents | Core | Agent discovery/listing |
| 28 | skills | Core | Skill discovery/listing |

---

## Plugin Types (`internal/plugins/`)

### PluginManifest
```go
type PluginManifest struct {
    Name          string
    Version       string
    Description   string
    Permissions   []PluginPermission
    DefaultEnabled bool
    Hooks         PluginHooks
    Lifecycle     PluginLifecycle
    Tools         []PluginToolManifest
    Commands      []PluginCommandManifest
}
```

### PluginHooks
```go
type PluginHooks struct {
    PreToolUse  []HookDefinition
    PostToolUse []HookDefinition
}

type HookDefinition struct {
    Command string
    Timeout time.Duration
}
```

### PluginLifecycle
```go
type PluginLifecycle struct {
    Init    string // command to run on init
    Shutdown string // command to run on shutdown
}
```

### PluginKind
```go
type PluginKind string
// "builtin" | "bundled" | "external"
```

---

## LSP Types (`internal/lsp/`)

### LspServerConfig
```go
type LspServerConfig struct {
    Name                string
    Command             string
    Args                []string
    Env                 map[string]string
    WorkspaceRoot       string
    InitializationOptions json.RawMessage
    ExtensionToLanguage map[string]string // e.g. ".go" → "go"
}
```

### SymbolLocation
```go
type SymbolLocation struct {
    Path  string
    Line  int
    Column int
}
```

### LspContextEnrichment
```go
type LspContextEnrichment struct {
    FilePath    string
    Diagnostics []Diagnostic
    Definitions []SymbolLocation
    References  []SymbolLocation
}
```

---

## Sandbox Types (`internal/runtime/`)

### SandboxConfig
```go
type SandboxConfig struct {
    Enabled              *bool
    NamespaceRestrictions *bool
    NetworkIsolation     *bool
    FilesystemMode       *FilesystemIsolationMode
    AllowedMounts        []string
}

type FilesystemIsolationMode string
// "off" | "workspace_only" | "allow_list"
```

### SandboxStatus
```go
type SandboxStatus struct {
    // 14 boolean fields tracking what's enabled/supported/active
    // for namespace, network, and filesystem isolation
}
```

---

## Usage/Cost Types (`internal/runtime/`)

### ModelPricing
```go
type ModelPricing struct {
    InputCostPerMillion           float64 // e.g. 15.0 for sonnet
    OutputCostPerMillion          float64 // e.g. 75.0 for sonnet
    CacheCreationCostPerMillion   float64 // e.g. 18.75
    CacheReadCostPerMillion       float64 // e.g. 1.5
}
```

### TokenUsage
```go
type TokenUsage struct {
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int
    CacheReadInputTokens     int
}
```

### UsageTracker
```go
type UsageTracker struct {
    LatestTurn  TokenUsage
    Cumulative  TokenUsage
    Turns       int
}
```

### Pricing Table

| Model | Input/M | Output/M | CacheCreate/M | CacheRead/M |
|-------|---------|----------|---------------|-------------|
| haiku | $1.00 | $5.00 | $1.25 | $0.10 |
| opus | $15.00 | $75.00 | $18.75 | $1.50 |
| sonnet | $15.00 | $75.00 | $18.75 | $1.50 |
