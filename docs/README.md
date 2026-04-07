# glaw-code

**glaw-code** is an open-source AI coding assistant written in Go. It provides a terminal REPL and web UI for interacting with AI models (Anthropic Claude, xAI Grok) to write, edit, search, and manage code.

## Features

- **Interactive REPL** with slash command autocomplete, markdown rendering, syntax highlighting, and animated tool execution
- **Web UI** — Next.js-based interface with real-time WebSocket communication, session management, and tool call visualization
- **23 built-in tools** — file read/write/edit, search, glob, bash execution, LSP integration, project analysis, and more
- **Multi-provider AI** — Anthropic Claude, xAI Grok, OpenAI-compatible endpoints
- **Session management** — save, resume, and switch sessions
- **Permission system** — configurable modes (read_only, workspace_write, danger_full_access) with interactive prompts
- **Undo/revert** — snapshot-based file revert without git
- **MCP support** — Model Context Protocol for extending tools via external servers
- **LSP integration** — Language Server Protocol for code intelligence
- **Slash commands** — 30+ commands for git, sessions, config, agents, and more

## Quick Start

### Install

```bash
# From source
git clone git@github.com:HendrickPhan/glaw-code.git
cd glaw-code
bash install.sh

# Or one-liner
curl -fsSL https://raw.githubusercontent.com/HendrickPhan/glaw-code/main/install.sh | bash
```

### Requirements

- Go 1.22+
- Node.js 20+ (for web UI only)
- `ANTHROPIC_API_KEY` or `XAI_API_KEY` environment variable

### Usage

```bash
# Interactive REPL
glaw

# One-shot mode
glaw "fix the bug in main.go"

# Web UI
glaw serve --addr :8080 --open

# With specific model
glaw --model claude-sonnet-4-6

# Resume session
glaw --session sess_1234567890
```

## Architecture

```
cmd/glaw/main.go          # Entry point, CLI flags, serve subcommand
internal/
  analyzer/              # Project source code analyzer (summary, dependency graphs)
  api/                    # Provider client (Anthropic, xAI, OpenAI-compat)
  cli/                    # REPL, terminal rendering, autocomplete, permissions UI
  commands/               # 30+ slash command handlers
  config/                 # Layered config loading (defaults → global → project → CLI)
  mcp/                    # MCP server manager (JSON-RPC over stdio)
  runtime/                # Core: ConversationRuntime, session, tools, permissions, snapshots
  tools/                  # 23 built-in tool implementations
  web/                    # HTTP/WebSocket server, session store, static UI embedding
  tasks/                  # Task management for multi-step operations
  lsp/                    # LSP client for code intelligence
  plugins/                # Plugin system (manifest-based)
web/                      # Next.js 16 web UI (TypeScript + Tailwind)
```

## Tools

| Tool | Description |
|------|-------------|
| `analyze` | Analyze project source code, generate summary, dependency graph, and code statistics |
| `read_file` | Read file contents with optional line range |
| `write_file` | Create or overwrite files |
| `edit_file` | Find-and-replace editing |
| `search_files` | Regex content search (ripgrep-style) |
| `list_directory` | List directory contents |
| `get_file_info` | Get file metadata |
| `glob` | Pattern-based file matching |
| `bash` | Execute shell commands |
| `lsp_*` | LSP operations (definition, references, hover, etc.) |
| `web_fetch` | Fetch URL contents |
| `ask_user` | Ask user a question during execution |
| `notebook_edit` | Edit Jupyter notebooks |

## Slash Commands

Type `/` in the REPL for autocomplete. Key commands:

| Command | Description |
|---------|-------------|
| `/help` | Show all commands |
| `/analyze` | Analyze project source code and generate summary/graph |
| `/model [name]` | Show or change AI model |
| `/clear` | Clear conversation |
| `/compact` | Compact conversation history |
| `/cost` | Show token usage and cost |
| `/revert [all]` | Undo file changes (snapshot-based) |
| `/diff` | Show pending git changes |
| `/commit [msg]` | Git commit |
| `/branch [create\|switch\|list\|delete]` | Git branch operations |
| `/session [list\|load\|delete]` | Session management |
| `/permissions [mode]` | Change permission mode |
| `/config [key] [value]` | Read/write configuration |

## Project Analysis

glaw-code includes a built-in project analyzer that can quickly scan your entire codebase and produce a comprehensive report. This is useful for onboarding, understanding unfamiliar code, and providing context to the AI.

### Analyze Tool (AI-facing)

The `analyze` tool is available for the AI model to use:

```json
{
  "mode": "full|summary|graph",
  "format": "text|mermaid|dot|json"
}
```

- **`full`** — Complete analysis: file stats, line counts, Go packages, dependency graph, complexity estimate. Results are cached in `.glaw/analysis.json`.
- **`summary`** — Quick overview from cached analysis (fast). Runs a fresh scan if no cache exists.
- **`graph`** — Dependency graph only, in the requested format (Mermaid, DOT/Graphviz, or JSON adjacency list).

### `/analyze` Command (REPL)

```bash
# Full analysis with summary + dependency graph
/analyze

# Quick summary from cached data
/analyze summary

# Dependency graph in Mermaid format
/analyze graph mermaid

# Dependency graph in DOT/Graphviz format
/analyze graph dot

# Dependency graph as JSON adjacency list
/analyze graph json
```

The analysis output includes:
- **Project Structure** — total files, directories, source/test/doc/config file counts
- **Lines of Code** — code, comments, and blank lines
- **File Type Distribution** — visual bar chart of file types
- **Infrastructure Detection** — go.mod, package.json, Dockerfile, Makefile, CI config
- **Go Package Analysis** — per-package stats: files, lines, functions, types, exported symbols, test coverage
- **Dependency Graph** — internal package dependencies rendered as Mermaid or DOT graph
- **Complexity Estimate** — small/medium/large based on size and structure

Analysis results are saved to `.glaw/analysis.json` for fast retrieval on subsequent requests.

## Permission Modes

| Mode | File Read | File Write | Bash | Outside Workspace |
|------|-----------|------------|------|-------------------|
| `read_only` | Yes | No | No | No |
| `workspace_write` | Yes | Yes (workspace only) | Prompt | Prompt |
| `danger_full_access` | Yes | Yes | Yes | Yes |

## Configuration

Layered config system (later overrides earlier):

1. Built-in defaults
2. `~/.glaw/settings.json` (global)
3. `.glaw/settings.json` (project)
4. `--config path/to/config.json` (explicit)
5. CLI flags (`--model`, `--permissions`)

Example `.glaw/settings.json`:

```json
{
  "model": "claude-sonnet-4-6",
  "permissions": { "mode": "workspace_write" },
  "mcpServers": {
    "context7": {
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@context7/mcp"]
    }
  }
}
```

## Web UI

```bash
glaw serve --addr :8080 --open
```

Features:
- Real-time chat via WebSocket
- Session creation and switching
- Tool call visualization with timing
- Markdown rendering with syntax highlighting
- Token usage display
- Dark theme

## Development

```bash
# Build
bash build.sh

# Run tests
go test ./...

# Run with specific model
go run ./cmd/glaw --model grok-3

# Development web UI
cd web && npm run dev
```

## License

Business Source License 1.1 — free for personal and non-commercial use. Commercial use requires a license from the copyright holder. See [LICENSE](LICENSE) for details.
