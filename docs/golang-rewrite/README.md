# Golang Rewrite Documentation — Index

Complete documentation of the Claw Code project for a Go rewrite. Based on the Rust production implementation.

## Documents

| # | File | Contents |
|---|------|----------|
| 01 | [architecture-overview.md](01-architecture-overview.md) | Project purpose, crate-to-package mapping, dependency graph, entry points, REPL loop overview |
| 02 | [data-models.md](02-data-models.md) | All structs, enums, interfaces: API types, session types, config, permissions, tool specs, command specs, plugin types, LSP types, sandbox types, usage/cost types |
| 03 | [api-client.md](03-api-client.md) | Provider client architecture (Anthropic/xAI/OpenAI-compat), request/response flows, SSE streaming, retry logic, error handling |
| 04 | [runtime-and-session.md](04-runtime-and-session.md) | ConversationRuntime, agentic turn/loop, bootstrap sequence, session persistence/compaction, system prompt construction, remote sessions, OAuth2+PKCE, SSE parser |
| 05 | [tools.md](05-tools.md) | All 19 built-in tools with input schemas, execution logic, error handling, GlobalToolRegistry |
| 06 | [commands.md](06-commands.md) | All 28 slash commands, parsing logic, fuzzy suggestion, dispatch table, git operations, agent/skill discovery |
| 07 | [plugins-and-hooks.md](07-plugins-and-hooks.md) | Plugin manifest, lifecycle (install/enable/disable/uninstall), external plugin tool execution via subprocess, hook system (pre/post tool use), shell command resolution |
| 08 | [mcp.md](08-mcp.md) | MCP server config, JSON-RPC over stdio, protocol messages (initialize/list tools/call tool), McpServerManager, tool routing |
| 09 | [lsp.md](09-lsp.md) | LSP server config, LspManager, lazy client init, document lifecycle, goToDefinition, findReferences, diagnostics, context enrichment for system prompt |
| 10 | [sandbox-and-security.md](10-sandbox-and-security.md) | Sandbox config, container detection, namespace isolation (unshare), permission model, hook-based security, security invariants |
| 11 | [server.md](11-server.md) | HTTP REST API with SSE streaming, routes, handlers, session store, broadcast channel pattern |
| 12 | [cli-and-repl.md](12-cli-and-repl.md) | CLI argument parsing, main flow, REPL loop with streaming/tool execution, modal line editor (vim-like), terminal rendering (markdown→ANSI), syntax highlighting |
| 13 | [compat-harness.md](13-compat-harness.md) | Upstream TypeScript manifest extraction for parity tracking (dev-only) |

## Suggested Go Project Layout

```
claw-go/
├── cmd/
│   └── claw/
│       └── main.go              # Entry point
├── internal/
│   ├── api/
│   │   ├── client.go            # ProviderClient interface + Anthropic/xAI implementations
│   │   ├── types.go             # API request/response types
│   │   ├── sse.go               # SSE parser
│   │   └── error.go             # ApiError types
│   ├── runtime/
│   │   ├── conversation.go      # ConversationRuntime (agentic loop)
│   │   ├── session.go           # Session persistence
│   │   ├── config.go            # Config loading/merging
│   │   ├── permissions.go       # PermissionManager
│   │   ├── prompt.go            # SystemPromptBuilder
│   │   ├── compact.go           # Session compaction
│   │   ├── oauth.go             # OAuth2 + PKCE
│   │   ├── remote.go            # Remote session + upstream proxy
│   │   ├── sandbox.go           # Sandbox detection + command building
│   │   ├── usage.go             # Token usage + cost tracking
│   │   ├── hooks.go             # Hook runner
│   │   ├── bash.go              # Bash executor
│   │   └── file_ops.go          # File read/write/edit/glob/grep
│   ├── tools/
│   │   ├── registry.go          # GlobalToolRegistry
│   │   ├── specs.go             # 19 tool specs
│   │   ├── bash.go              # Bash tool
│   │   ├── files.go             # File tools
│   │   ├── search.go            # Glob + grep tools
│   │   ├── web.go               # WebFetch + WebSearch
│   │   ├── agent.go             # Agent tool (sub-agent spawning)
│   │   ├── notebook.go          # NotebookEdit
│   │   └── config_tool.go       # Config tool
│   ├── commands/
│   │   ├── parser.go            # Slash command parsing + fuzzy match
│   │   ├── dispatch.go          # Command dispatch table
│   │   ├── git.go               # Git commands (commit, pr, branch, etc.)
│   │   └── discovery.go         # Agent/skill discovery
│   ├── plugins/
│   │   ├── manager.go           # PluginManager
│   │   ├── registry.go          # PluginRegistry
│   │   ├── manifest.go          # Manifest parsing + validation
│   │   ├── hooks.go             # Hook runner
│   │   └── tool.go              # Plugin tool subprocess execution
│   ├── mcp/
│   │   ├── manager.go           # McpServerManager
│   │   ├── stdio.go             # McpStdioProcess (JSON-RPC over stdio)
│   │   ├── types.go             # MCP protocol types
│   │   └── config.go            # MCP server config parsing
│   ├── lsp/
│   │   ├── manager.go           # LspManager
│   │   ├── client.go            # LspClient (JSON-RPC over stdio)
│   │   ├── types.go             # LSP types
│   │   └── error.go             # LspError
│   ├── server/
│   │   ├── server.go            # HTTP server + routes
│   │   └── sse.go               # SSE streaming handler
│   ├── cli/
│   │   ├── repl.go              # REPL loop
│   │   ├── input.go             # Modal line editor
│   │   ├── render.go            # Markdown → ANSI rendering
│   │   └── args.go              # CLI argument definitions
│   └── compat/
│       └── harness.go           # Upstream manifest extraction
├── go.mod
├── go.sum
└── Makefile
```

## Key Go Libraries to Consider

| Purpose | Library |
|---------|---------|
| CLI args | `cobra` or stdlib `flag` |
| HTTP client | `net/http` |
| HTTP server | `net/http` or `gin`/`chi` |
| JSON | `encoding/json` |
| Regex | `regexp` |
| Glob | `path/filepath` |
| Terminal raw mode | `golang.org/x/term` or `github.com/charmbracelet/bubbletea` |
| Syntax highlighting | `github.com/alecthomas/chroma` |
| Git operations | `os/exec` calling `git` CLI |
| GitHub CLI | `os/exec` calling `gh` CLI |
| SSE parsing | Custom using `bufio.Scanner` |
| TOML config | `github.com/BurntSushi/toml` |
| YAML | `gopkg.in/yaml.v3` |
