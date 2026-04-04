# Runtime and Session Management

## Package: `internal/runtime/`

---

## ConversationRuntime

The central orchestrator that ties together the API client, tool executor, session, and all subsystems.

### Struct
```go
type ConversationRuntime struct {
    APIClient         ProviderClient
    ToolExecutor      ToolExecutor
    Session           *Session
    Config            *ResolvedConfig
    PermissionManager *PermissionManager
    UsageTracker      *UsageTracker
    HookRunner        *HookRunner
    MCPManager        *McpServerManager
    LSPManager        *LspManager
    PluginManager     *PluginManager
}
```

### Core Methods

#### `Turn(ctx) -> (*TurnResult, error)`
Single agentic turn:
```
1. Build ApiRequest from session messages + tool definitions
2. IF Config.Stream:
     Call APIClient.StreamMessage()
     Process SSE events incrementally
   ELSE:
     Call APIClient.SendMessage()
3. Append assistant message to session
4. FOR EACH tool_use content block:
     a. Check permission (PermissionManager.Check)
     b. IF permission denied: append tool_result with error, CONTINUE
     c. Run pre-tool hooks (HookRunner.RunPreToolUse)
     d. IF hook denies: append tool_result with denial message, CONTINUE
     e. Execute tool (ToolExecutor.ExecuteTool)
     f. Run post-tool hooks (HookRunner.RunPostToolUse)
     g. Append tool_result to session
5. Update usage tracker
6. Return TurnResult with stop_reason
```

#### `RunLoop(ctx) -> error`
Multi-turn agentic loop:
```
1. LOOP:
   result = Turn(ctx)
   IF result.StopReason == "end_turn": BREAK
   IF result.StopReason == "max_tokens": BREAK
   IF error: BREAK
   // If "tool_use": loop continues, sending tool results back
2. Return last error (if any)
```

---

## Bootstrap Sequence

### BootstrapPhases (ordered)

| Phase | Name | Purpose |
|-------|------|---------|
| 1 | CliEntry | Parse CLI arguments |
| 2 | FastPathVersion | Quick version check |
| 3 | StartupProfiler | Begin startup timing |
| 4 | SystemPromptFastPath | Load/construct system prompt |
| 5 | ChromeMcpFastPath | Initialize MCP connections |
| 6 | DaemonWorkerFastPath | Start background daemon |
| 7 | BridgeFastPath | Set up bridge transport |
| 8 | DaemonFastPath | Daemon handshake |
| 9 | BackgroundSessionFastPath | Resume background sessions |
| 10 | TemplateFastPath | Load prompt templates |
| 11 | EnvironmentRunnerFastPath | Set up environment |
| 12 | MainRuntime | Final runtime initialization |

### BootstrapPlan
```go
type BootstrapPlan struct {
    Phases []BootstrapPhase
}

func NewBootstrapPlan(phases ...BootstrapPhase) *BootstrapPlan
```

---

## Session Management

### Session Structure
```go
type Session struct {
    Version  int
    Messages []ConversationMessage
}

type ConversationMessage struct {
    Role   string         // "user" | "assistant"
    Blocks []ContentBlock
    Usage  *TokenUsage    // nil for user messages
}
```

### Persistence

**Save:**
```
save_session(session, path):
  1. Serialize session to JSON using custom JSON renderer
  2. Create parent directory if not exists
  3. Write to path (e.g. .claw/sessions/{id}.json)
```

**Load:**
```
load_session(path) -> Session:
  1. Read file content
  2. Parse JSON using custom JSON parser
  3. Reconstruct Session struct
  4. Rebuild UsageTracker from message usage fields
```

**Note on JSON handling:** The Rust implementation uses a custom JSON parser/renderer for session persistence to avoid serde dependencies. In Go, just use `encoding/json` with standard marshaling.

### Session Compaction

When conversation gets too long, compact to stay within context limits:

```
compact_session(session, config):
  1. IF len(session.Messages) <= config.MaxMessages: return (no-op)
  2. Split messages:
     - old_messages = all except last N (config.KeepRecent)
     - recent_messages = last N
  3. Generate summary of old_messages:
     - Build a prompt asking the model to summarize the conversation
     - Send to API with config.SummaryModel
     - Extract summary text
  4. Replace session messages with:
     - Single system message containing the summary
     - recent_messages preserved as-is
```

---

## System Prompt Construction

### SystemPromptBuilder

```go
type SystemPromptBuilder struct {
    ProjectContext     string   // file tree, language stats
    InstructionFiles   []string // CLAW.md, .claw/CLAW.md contents
    GitStatus          string   // output of git status
    GitDiff            string   // output of git diff
    ToolDescriptions   []string // formatted tool specs
    CustomInstructions string   // user-provided additions
}
```

**Build() logic:**
```
1. Start with base instructions ( hardcoded agent behavior rules )
2. Append project context section:
   "## Project Context\n{file_tree}\n{language_breakdown}"
3. Append instruction files section:
   FOR EACH instruction_file:
     Append "## Instructions from {filename}\n{content}"
4. Append git context (if in git repo):
   "## Git Status\n{git_status}\n## Git Diff\n{git_diff}"
5. Append tool descriptions:
   FOR EACH tool:
     Append formatted tool spec with name, description, schema
6. Append custom instructions
7. Return assembled string
```

**Instruction file discovery:**
1. `{project_root}/CLAW.md`
2. `{project_root}/.claw/CLAW.md`
3. Any `{project_root}/.claw/instructions/*.md` files

---

## Remote Session Support

### RemoteSessionContext
```go
type RemoteSessionContext struct {
    Enabled  bool
    SessionID string
    BaseURL   string
}
```

**Detection:** Read `CLAW_CODE_REMOTE` environment variable. If set, enable remote mode.

### Upstream Proxy

When running in remote mode with an upstream proxy:

```go
type UpstreamProxyState struct {
    Enabled      bool
    ProxyURL     string
    CABundlePath string
    NoProxy      []string
}
```

**Proxy environment keys (checked in order):**
1. `CLAUDE_CODE_PROXY`
2. `HTTPS_PROXY`
3. `https_proxy`
4. `HTTP_PROXY`
5. `http_proxy`
6. `ALL_PROXY`
7. `all_proxy`
8. `SOCKS_PROXY`

**No-proxy hosts (16 defaults):**
localhost, 127.0.0.1, ::1, *.local, *.internal, api.anthropic.com, statsig.anthropic.com, sentry.io, sentry.io:443, *.sentry.io, analytics.anthropic.com, claude.ai, claude.ai:443

---

## OAuth2 + PKCE Flow

### OAuthConfig
```go
type OAuthConfig struct {
    ClientID     string
    AuthorizeURL string
    TokenURL     string
    RedirectPort int // local HTTP server for callback
}
```

### PKCE Challenge
```go
type PkceChallenge struct {
    CodeVerifier  string // random 43-128 char string
    CodeChallenge string // SHA256(verifier), base64url encoded
}
```

### Flow
```
authorize():
  1. Generate PKCE code_verifier (random alphanumeric)
  2. Compute code_challenge = base64url(sha256(code_verifier))
  3. Open browser to authorize_url with:
     - client_id
     - redirect_uri = "http://localhost:{port}/callback"
     - response_type = "code"
     - code_challenge
     - code_challenge_method = "S256"
  4. Start local HTTP server on redirect_port
  5. Wait for callback with authorization code
  6. Return code

exchange_code(code, pkce):
  1. POST to token_url:
     - grant_type = "authorization_code"
     - code = {authorization_code}
     - redirect_uri = {callback_url}
     - client_id = {client_id}
     - code_verifier = {pkce.code_verifier}
  2. Parse response into OAuthToken
  3. Save credentials to ~/.claw/credentials

refresh_token(token):
  1. IF token not expired: return token
  2. POST to token_url:
     - grant_type = "refresh_token"
     - refresh_token = {token.RefreshToken}
     - client_id = {client_id}
  3. Parse response, update token
  4. Save updated credentials
```

---

## SSE Event Parser (runtime layer)

### IncrementalSseParser

```go
type IncrementalSseParser struct {
    buffer    strings.Builder
    eventName string
    dataLines []string
    id        string
    retry     *int
}
```

**Algorithm:**
```
PushChunk(chunk string) []SseEvent:
  Append chunk to internal buffer
  WHILE "\n\n" found in buffer:
    Extract text before "\n\n" as raw_event
    FOR each line in raw_event:
      IF line starts with ":": continue (comment)
      IF line starts with "event: ": store event name
      IF line starts with "data: ": append to dataLines
      IF line starts with "id: ": store id
      IF line starts with "retry: ": parse and store int
    Build SseEvent{Event, Data (joined with \n), ID, Retry}
    Reset parser state
    Remove processed text + delimiter from buffer
  Return collected events
```
