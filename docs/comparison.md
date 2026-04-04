# glaw-code vs Claude Code vs Open Code

A feature comparison of three AI coding assistants: **glaw-code** (Go), **Claude Code** (TypeScript/CLI), and **Open Code** (Go).

## Overview

| | glaw-code | Claude Code | Open Code |
|---|---|---|---|
| **Language** | Go | TypeScript (Node.js) | Go |
| **License** | BSL 1.1 (non-commercial) | Proprietary | MIT |
| **AI Providers** | Anthropic, xAI, OpenAI-compat | Anthropic only | Multiple (OpenAI, Anthropic, Google, etc.) |
| **Interface** | REPL + Web UI | CLI + IDE extensions | TUI (Terminal UI) |
| **Open Source** | Yes | No | Yes |

## Feature Matrix

### Core

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Interactive REPL | Yes | Yes | Yes (TUI) |
| Web UI | Yes (Next.js) | No (IDE only) | No |
| One-shot mode | Yes | Yes | Yes |
| Multi-turn conversation | Yes | Yes | Yes |
| Streaming responses | Yes | Yes | Yes |
| Session save/resume | Yes | Yes | Yes |
| System prompt customization | Yes (GLAW.md) | Yes (CLAUDE.md) | Yes |

### Tools

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| File read/write/edit | Yes | Yes | Yes |
| Regex search | Yes | Yes | Yes |
| Glob matching | Yes | Yes | Yes |
| Bash execution | Yes | Yes | Yes |
| LSP integration | Yes | Yes | No |
| MCP support | Yes | Yes | Yes |
| Web fetch/search | Yes | Yes | No |
| Notebook editing | Yes | Yes | No |
| Sub-agent spawning | Planned | Yes | No |
| Total built-in tools | 22 | ~15 | ~12 |

### Developer Experience

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Slash command autocomplete | Yes (Tab) | Yes | Yes |
| Markdown rendering | Yes (ANSI) | Yes | Yes |
| Syntax highlighting | Yes (Chroma) | Yes | Yes (Chroma) |
| Collapsible thinking blocks | Yes | Yes | No |
| Tool execution animation | Yes (spinner) | Yes (spinner) | Basic |
| Token usage display | Yes | Yes | Yes |
| Cost estimation | Yes | Yes | No |
| Undo/revert (no git) | Yes (snapshot) | Yes (git-based) | No |

### Permissions & Security

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Permission modes | 4 (read_only, workspace_write, danger_full_access, prompt) | 3 (read, write, full) | Basic allow/deny |
| Path validation | Yes (workspace boundary) | Yes | No |
| Bash command prompting | Yes (per-command) | Yes | Basic |
| Session-level caching | Yes | Yes | No |
| Sandbox support | Linux (namespaces) | Yes (containers) | No |

### Git & GitHub

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Git diff | Yes | Yes | No |
| Git commit | Yes | Yes | No |
| Branch management | Yes | Yes | No |
| PR creation | Yes (via gh) | Yes (via gh) | No |
| Issue operations | Yes (via gh) | Yes (via gh) | No |
| Worktree support | Yes | Yes | No |

### Configuration

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Layered config | Yes (5 levels) | Yes | Yes |
| Project-level settings | Yes (.glaw/) | Yes (.claude/) | Yes |
| MCP server config | Yes | Yes | Yes |
| Custom instructions file | GLAW.md | CLAUDE.md | Yes |
| Hook system (pre/post tool) | Planned | Yes | No |

### Architecture

| Feature | glaw-code | Claude Code | Open Code |
|---------|-----------|-------------|-----------|
| Binary size | ~12MB | ~100MB+ (Node.js) | ~15MB |
| Startup time | Instant | ~2-5s (Node.js) | Instant |
| Memory usage | Low (~20-50MB) | High (~200-500MB) | Low (~30-60MB) |
| Static embedding | Yes (Go embed) | N/A | No |
| Cross-platform | Mac, Linux, Windows | Mac, Linux, Windows | Mac, Linux, Windows |
| Concurrent tool execution | Sequential | Sequential | Sequential |

## Key Differentiators

### glaw-code strengths
- **Multi-provider**: Not locked to a single AI provider (Anthropic + xAI + OpenAI-compat)
- **Web UI**: Full browser-based interface alongside terminal REPL
- **Lightweight**: Go binary with low memory footprint and instant startup
- **Snapshot undo**: File revert without requiring git
- **Extensible**: MCP + LSP + plugin architecture

### Claude Code strengths
- **Mature ecosystem**: IDE extensions (VS Code, JetBrains), desktop app, web app
- **Deep Anthropic integration**: Optimized for Claude models
- **Large user base**: Well-tested, frequent updates
- **Advanced features**: Sub-agents, computer use, image support
- **Enterprise ready**: Team plans, SSO, audit logs

### Open Code strengths
- **MIT license**: Fully open, no commercial restrictions
- **Multi-provider**: Broad LLM support out of the box
- **TUI interface**: Beautiful terminal UI with bubbletea
- **Lightweight**: Go binary, fast startup
- **Active community**: Open-source contributions welcome

## When to Choose

| Use Case | Recommended |
|----------|-------------|
| Personal coding with AI | glaw-code or Open Code |
| Team/enterprise | Claude Code |
| Self-hosted, private | glaw-code or Open Code |
| Multi-model flexibility | glaw-code or Open Code |
| IDE integration | Claude Code |
| Web-based interface | glaw-code |
| Learning/hacking on AI tools | glaw-code or Open Code |
| Production coding at scale | Claude Code |
