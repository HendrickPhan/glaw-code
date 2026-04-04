# Architecture Overview

## Project Purpose

Claw Code is a local coding-agent CLI (inspired by Claude Code) that provides:
- Interactive REPL and one-shot prompt execution
- Workspace-aware tools (file ops, bash, search, web)
- Slash commands for task management
- Plugin system and agent/skill discovery
- Session management and history persistence
- Permission-based tool access control
- MCP (Model Context Protocol) integration
- LSP (Language Server Protocol) support

The project has two implementations:
- **Python (`src/`):** Scaffolding/porting workspace that mirrors the original TypeScript archive. Not production code -- it loads JSON snapshots and simulates behavior.
- **Rust (`rust/`):** Production-ready implementation with full runtime, API client, tools, commands, plugins, and CLI.

This document focuses on the Rust implementation as the authoritative source for the Golang rewrite.

---

## Crate/Module Map (Rust) → Suggested Go Package Map

| Rust Crate | Responsibility | Suggested Go Package |
|---|---|---|
| `claw-cli` | CLI entrypoint, REPL loop, terminal UI | `cmd/claw/` + `internal/cli/` |
| `api` | HTTP API client for LLM providers (Anthropic, xAI/Grok, OpenAI-compat) | `internal/api/` |
| `runtime` | Conversation runtime, session, config, permissions, bash, file ops, hooks, MCP, OAuth, sandbox | `internal/runtime/` (with sub-packages) |
| `tools` | 19 built-in tools, tool registry, agent spawning | `internal/tools/` |
| `commands` | 28 slash commands, parsing, dispatch | `internal/commands/` |
| `plugins` | Plugin lifecycle, registry, installation, hook execution | `internal/plugins/` |
| `lsp` | LSP client integration | `internal/lsp/` |
| `server` | HTTP/REST server with SSE streaming | `internal/server/` |

---

## High-Level Dependency Graph

```
cmd/claw (main)
  ├── internal/cli (REPL loop, terminal input, rendering)
  │     ├── internal/runtime (ConversationRuntime)
  │     │     ├── internal/api (ProviderClient → Anthropic/xAI/OpenAI)
  │     │     ├── internal/tools (ToolRegistry, 19 tools)
  │     │     ├── internal/commands (SlashCommand dispatch)
  │     │     ├── internal/plugins (PluginManager, hooks)
  │     │     ├── internal/lsp (LspManager)
  │     │     └── internal/server (HTTP API, optional)
  │     └── internal/runtime/config (Config loading/merging)
  └── internal/compat-harness (upstream manifest extraction, optional)
```

---

## Key Architectural Decisions

1. **Async I/O throughout:** Rust uses tokio. In Go, use goroutines + channels naturally.
2. **Trait-based abstraction:** Rust traits `ApiClient` and `ToolExecutor` allow mocking and provider swapping. In Go, use interfaces.
3. **Custom JSON parser for sessions:** Rust avoids serde for session persistence. In Go, use `encoding/json` -- no need for a custom parser.
4. **SSE streaming:** Used for both API responses and server-side event streaming. Go can use `bufio.Scanner` for SSE parsing.
5. **Permission-gated tool execution:** Every tool call goes through permission checks and hook execution.
6. **Plugin isolation:** External plugins run as subprocesses with JSON on stdin/stdout.
7. **LSP and MCP use JSON-RPC over stdio:** Content-Length header framing, same pattern as LSP protocol.

---

## Entry Points

### CLI Entry (`claw-cli/src/main.rs`)

```
main()
  → parse CLI args (clap)
  → load config (layered: user → project → local)
  → init runtime (ConversationRuntime)
  → IF prompt arg provided: run single turn, print result, exit
  → ELSE: enter REPL loop
```

### REPL Loop

```
run_repl_loop()
  LOOP:
    read user input (modal line editor)
    IF input starts with '/': dispatch slash command
    ELSE:
      build system prompt (project context + CLAW.md + tool descriptions)
      send to API with streaming
      process SSE response stream
      FOR EACH tool_use block:
        check permissions
        run pre-tool hooks
        execute tool
        run post-tool hooks
        append tool result to conversation
        CONTINUE loop (send tool results back to API)
      display response text to user
      display usage/cost summary
```
