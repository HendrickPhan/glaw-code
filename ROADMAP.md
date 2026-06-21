# glaw-code — 2-Year Roadmap (2025–2027)

> A living document outlining the evolution of glaw-code from a capable AI coding assistant into a best-in-class, extensible, multi-modal development platform.

---

## Table of Contents

- [Where We Are Today](#where-we-are-today)
- [Vision](#vision)
- [Guiding Principles](#guiding-principles)
- [Phase 1 — Solidify (Q3 2025)](#phase-1--solidify-q3-2025)
- [Phase 2 — Scale (Q4 2025)](#phase-2--scale-q4-2025)
- [Phase 3 — Extend (Q1–Q2 2026)](#phase-3--extend-q1q2-2026)
  - [3.1 Remote Access & Control via Internet](#31-remote-access--control-via-internet)
  - [3.2 Multi-Modal Support (Vision)](#32-multi-modal-support-vision)
  - [3.3 IDE Extension (VS Code)](#33-ide-extension-vs-code)
  - [3.4 Enhanced LSP Integration](#34-enhanced-lsp-integration)
  - [3.5 Git Worktree & Branch Intelligence](#35-git-worktree--branch-intelligence)
  - [3.6 MCP Ecosystem Expansion](#36-mcp-ecosystem-expansion)
- [Phase 4 — Platform (Q3–Q4 2026)](#phase-4--platform-q3q4-2026)
  - [4.1 Autonomous Work Mode](#41-autonomous-work-mode)
  - [4.2 glaw-code SDK / Public API](#42-glaw-code-sdk--public-api)
- [Phase 5 — Intelligence (Q1–Q2 2027)](#phase-5--intelligence-q1q2-2027)
- [Non-Goal Themes](#non-goal-themes)
- [Success Metrics](#success-metrics)
- [How to Contribute to the Roadmap](#how-to-contribute-to-the-roadmap)

---

## Where We Are Today

| Aspect | Status |
|--------|--------|
| **Language** | Go (backend) + TypeScript/Next.js (web UI) |
| **Lines of Code** | ~40K total (32K Go, ~1.5K TypeScript) |
| **Test Coverage** | 27–94% across modules (avg ~55%) |
| **Architecture** | Clean Architecture with all modules migrated to `internal/modules/` |
| **LLM Providers** | Anthropic, OpenAI, Gemini, xAI, OpenRouter, Ollama |
| **Tools** | 23 built-in (bash, file I/O, search, web, analysis, sub-agents) |
| **Cost Tracking** | Per-session usage tracking with `/cost` command; session-scoped (no persistent history) |
| **Error Recovery** | Auto-retry with exponential backoff; session auto-save on SIGINT/SIGTERM |
| **Streaming Infra** | SSE streaming at API + runtime layer; REPL not yet wired up |
| **Delivery** | CLI REPL + Web UI (WebSocket) |
| **Extensions** | MCP servers, Plugin manifest system |
| **CI/CD** | GitHub Actions (build, test, lint, cross-compile, release) |
| **Platforms** | macOS, Linux (amd64, arm64) |

### Known Gaps

- **No streaming in CLI** — only the web UI has real-time streaming; the REPL blocks until the full response arrives. (Streaming infrastructure exists at API + runtime layers but REPL is not wired up.)
- **Low test coverage** on CLI rendering (13.9%), commands (12.8%), MCP (7.8%), and web (18.7%).
- **No conversation memory** — sessions are stored as flat JSON; no vector store or semantic retrieval.
- **No image/multi-modal support** — can't process screenshots, diagrams, or images.
- **No Windows support** — only macOS and Linux binaries.
- **No structured logging** — uses `fmt.Fprintf(os.Stderr, ...)` for logging.
- **No OpenTelemetry/tracing** — no observability into tool execution latency, API call timing, or error rates.
- **Plugin system is manifest-only** — no hot-reload, no sandboxing, no marketplace.
- **Web UI is basic** — no file tree, no diff viewer, no inline code editing.
- **No `glaw` SDK or API** — other tools can't programmatically drive glaw-code.
- **No remote access** — glaw-code is local-only; can't control from phone, another machine, or CI.
- **No autonomous mode** — glaw-code requires interactive human oversight; can't work independently.
- **`internal/runtime/` facade not removed** — 12+ files still import the backward-compatibility facade; module migration is functionally complete but cleanup is pending.

---

## Vision

> **glaw-code becomes the developer's AI pair-programmer that works everywhere — terminal, browser, IDE, CI pipeline — with a thriving ecosystem of extensions, agents, and integrations.**

We aim to be:

1. **The fastest** — sub-second time-to-first-token, streaming everywhere.
2. **The most extensible** — MCP, plugins, custom agents, and a public SDK.
3. **The most transparent** — full observability, cost tracking, and audit logging.
4. **The most reliable** — 80%+ test coverage, chaos testing, graceful degradation.

---

## Guiding Principles

1. **Developer experience first** — every feature must feel instant and intuitive.
2. **Privacy by default** — code stays local; telemetry is opt-in.
3. **Modularity over monolith** — every capability is a composable module.
4. **Convention-compliant** — all new code follows `convention.md` (Clean Architecture, Package by Feature).
5. **Test-driven evolution** — no module ships below 70% coverage.
6. **Open standards** — MCP for tooling, LSP for language intelligence, OpenTelemetry for observability.

---

## Phase 1 — Solidify (Q3 2025)

> *Stabilize the foundation. Fix what matters most.*

### 1.1 Complete Module Migration

**Priority:** P0 · **Effort:** M · **Status:** ✅ Done

Finish migrating remaining flat packages into the `internal/modules/` structure per `convention.md`:

- [x] Move `internal/agent/` → `internal/modules/agent/` with domain/application/infrastructure layers
- [x] Move `internal/commands/` → `internal/modules/commands/` and split the 2,500-line `commands.go` by category
- [x] Move `internal/tools/` → `internal/modules/tools/` and split `tools.go` into individual tool files
- [x] Move `internal/mcp/` → `internal/modules/mcp/` with transport abstraction in infrastructure
- [x] Move `internal/lsp/` → `internal/modules/lsp/` with client in infrastructure
- [x] Move `internal/config/` → `internal/modules/config/`
- [x] Move `internal/tasks/` → `internal/modules/tasks/`
- [x] Move `internal/plugins/` → `internal/modules/plugins/`
- [ ] Remove the `internal/runtime/` backward-compatibility facade once all consumers are migrated (12+ files still import it)
- [x] Update `cmd/glaw/main.go` to use `bootstrap` exclusively

### 1.2 Streaming in the CLI REPL

**Priority:** P0 · **Effort:** L

The REPL currently blocks until the full LLM response arrives. This feels slow even with fast models.

> **Note:** Streaming infrastructure has been built at the API layer (`StreamMessage()` for Anthropic and OpenAI-compatible clients with full SSE parsing) and runtime layer (`StreamTurn()`, `AccumulateStream()`, `RunToolLoopStream()`). However, the REPL does not yet use these methods — `Turn()` is called with `Stream: false`. The remaining work is wiring the REPL to call the streaming path.

- [ ] Wire the REPL to use `StreamTurn()` / `RunToolLoopStream()` instead of `Turn()` with `Stream: false`
- [ ] Stream text content character-by-character (typewriter effect)
- [ ] Show tool calls as they arrive (not after the full response)
- [ ] Add a `--no-stream` flag for users who prefer buffered output
- [ ] Handle stream interruption (Ctrl+C mid-stream)

### 1.3 Structured Logging

**Priority:** P1 · **Effort:** S

Replace all `fmt.Fprintf(os.Stderr, ...)` with a proper structured logger.

- [ ] Add `internal/shared/logger/` with `slog`-based logger
- [ ] Support log levels: `DEBUG`, `INFO`, `WARN`, `ERROR`
- [ ] Add `--log-level` and `--log-format` (text/json) CLI flags
- [ ] Log tool execution with duration, success/failure, and token counts
- [ ] Log API calls with latency, status codes, and retry attempts
- [ ] Redact API keys and sensitive data from log output

### 1.4 Raise Test Coverage to 70%+

**Priority:** P1 · **Effort:** L

Current low-coverage packages need attention:

| Package | Current | Target |
|---------|---------|--------|
| `cli` | 13.9% | 70% |
| `commands` | 12.8% | 70% |
| `mcp` | 7.8% | 70% |
| `web` | 18.7% | 70% |
| `tools` | 47.6% | 75% |
| `agent` | 49.2% | 75% |
| `lsp` | 13.1% | 60% |

- [ ] Add CLI rendering tests (markdown, spinners, tool output)
- [ ] Add command handler tests for all 30+ slash commands
- [ ] Add MCP manager tests with mock transports
- [ ] Add web handler tests with mock WebSocket
- [ ] Add tool execution tests for each of the 23 tools
- [ ] Add agent lifecycle tests (spawn, cancel, timeout)
- [ ] Add integration test suite for full conversation flows

### 1.5 Split `tools.go` into Individual Files

**Priority:** P1 · **Effort:** M · **Status:** ❌ Not Started

The ~1,800-line `tools.go` violates single-responsibility. Split by tool category:

```
internal/modules/tools/infrastructure/registry/
  registry.go          — Registry struct, ExecuteTool dispatcher
  bash_tool.go         — bash, bash_result, bash_stop
  file_tools.go        — read_file, write_file, edit_file
  search_tools.go      — glob_search, grep_search
  web_tools.go         — web_fetch, web_search
  agent_tools.go       — sub_agent, sub_agent_result
  notebook_tool.go     — notebook_edit
  misc_tools.go        — sleep, send_user_message, config, todo_write, tool_search
  analyze_tool.go      — analyze (language-agnostic project analysis)
```

---

## Phase 2 — Scale (Q4 2025)

> *Performance, reliability, and multi-platform support.*

### 2.1 Windows Support

**Priority:** P1 · **Effort:** L

- [ ] Add Windows to CI matrix (`windows-latest`)
- [ ] Handle Windows path separators (`\` vs `/`)
- [ ] Replace `bash` tool with platform-aware shell (`bash` on Unix, `powershell` on Windows)
- [ ] Fix ANSI color codes for Windows Terminal
- [ ] Add Windows installer (`.exe` release, `winget`, `scoop`)
- [ ] Test `read_file`, `write_file`, `edit_file` with Windows paths
- [ ] Handle `exec.Command("bash", ...)` fallback on Windows

### 2.2 Conversation Context Management

**Priority:** P0 · **Effort:** XL

The current system sends the full conversation history to the LLM every turn. This is expensive and hits context window limits.

- [ ] Implement **sliding window** — keep last N messages + system prompt
- [ ] Implement **smart compaction** — summarize older turns into a compact summary
- [ ] Add **RAG-lite** — index project files locally and inject relevant context
- [ ] Support **context window management** — track token usage per turn, auto-compact when approaching limits
- [ ] Add `/context` command to inspect current context window usage
- [ ] Persist conversation summaries alongside sessions

### 2.3 Cost & Usage Dashboard

**Priority:** P2 · **Effort:** M

- [x] Track per-session, per-model cost (`UsageTracker` with `EstimateCost()` per model, displayed after each turn)
- [x] `/cost` command showing tokens + cost summary per session
- [ ] Add `/cost breakdown` command showing cost by turn
- [ ] Web UI cost dashboard with charts (daily, weekly, monthly)
- [ ] Budget alerts — warn when daily/session cost exceeds threshold
- [ ] Export cost data as CSV/JSON
- [ ] Persist usage history in `~/.glaw/usage.json` (currently session-scoped only, resets on new session)

### 2.4 Error Recovery & Resilience

**Priority:** P1 · **Effort:** M

- [x] Auto-retry on transient API failures with exponential backoff (`SendMessage()` and `StreamMessage()` retry with `backoffDuration()`, capped at 30s, `MaxRetries=2`)
- [ ] Graceful degradation when MCP servers crash (currently only prints warnings)
- [x] Session auto-save on SIGINT/SIGTERM (`gracefulShutdown()` in repl.go + one-shot mode handler)
- [ ] Corrupted session file recovery
- [x] Tool execution timeout with configurable limits (bash tool supports `timeout` parameter, default 120s)
- [ ] Disk space check before write operations
- [ ] Network connectivity detection and offline mode

### 2.5 Performance Benchmarks

**Priority:** P2 · **Effort:** S

- [ ] Add Go benchmarks for hot paths: `ContentBlock.MarshalJSON`, `Session.AsAPIMessages`, tool dispatch
- [ ] Benchmark time-to-first-token for each provider
- [ ] Memory profiling for long sessions (10K+ messages)
- [ ] Start-up time benchmark (< 100ms target)
- [ ] Add `make bench` target to CI

---

## Phase 3 — Extend (Q1–Q2 2026)

> *Multi-modal intelligence, richer tooling, IDE integration, remote access.*

### 3.1 Remote Access & Control via Internet

**Priority:** P0 · **Effort:** XL

Enable developers to securely control glaw-code from anywhere — phone, another computer, a colleague's machine — over the internet without exposing the local machine to the public network.

#### 3.1.1 Secure Tunnel Architecture

The core challenge: glaw-code runs on the developer's local machine, but the user wants to interact from a remote device. We avoid opening ports or running a public server.

- [ ] **glaw tunnel** — establish an encrypted WebSocket tunnel between the local glaw-code instance and a lightweight relay service
- [ ] **Relay server** (`glaw-relay`) — minimal, stateless relay that forwards encrypted frames between two authenticated peers (can be self-hosted or community-hosted)
- [ ] **End-to-end encryption** — all traffic encrypted with per-session keys using NaCl (X25519 + XSalsa20-Poly1305); relay sees only ciphertext
- [ ] **Authentication** — connect using a short-lived pairing code (QR code or 6-digit PIN, similar to `ssh` or VS Code tunnels)
- [ ] **No port forwarding required** — outbound-only connections from both sides to the relay
- [ ] **Multi-connection** — allow multiple remote clients simultaneously (phone + laptop + teammate)
- [ ] **Self-hosted relay option** — run `glaw-relay` on your own infrastructure for full control

```
┌──────────┐          ┌──────────────┐          ┌──────────────┐
│ Developer │◄─E2E E2E─►│  glaw-relay  │◄─E2E E2E─►│ glaw-code    │
│ (phone /  │          │ (stateless   │          │ (developer's │
│  browser) │          │  forwarder)  │          │  workstation)│
└──────────┘          └──────────────┘          └──────────────┘
```

#### 3.1.2 Remote Client Interfaces

- [ ] **glaw remote** — CLI client that connects to a remote glaw-code instance via tunnel
  ```bash
  glaw remote connect --code ABC123    # connect via pairing code
  glaw remote send "fix the tests"    # send a one-shot prompt
  glaw remote status                   # check remote instance status
  glaw remote sessions                 # list remote sessions
  ```
- [ ] **Mobile web interface** — responsive PWA at `https://remote.glaw.dev/{code}` for quick access from any phone browser
- [ ] **Web UI remote mode** — toggle the existing web UI to connect to a remote instance instead of local
- [ ] **Chat API over tunnel** — send/receive messages, tool results, and status updates through the relay
- [ ] **Remote slash commands** — execute `/status`, `/model`, `/agents`, `/stop` from the remote client

#### 3.1.3 Security Model

- [ ] **Pairing ceremony** — local instance displays a QR code + PIN; remote client scans or enters it; connection is authorized for that session only
- [ ] **Permission scoping for remote** — remote clients can be restricted:
  - `--remote-perm readonly` — can only view output, cannot send prompts
  - `--remote-perm chat` — can send messages but tools require local approval
  - `--remote-perm full` — full control (default when explicitly set)
- [ ] **Rate limiting** — prevent abuse from remote clients
- [ ] **Idle disconnect** — auto-disconnect after configurable timeout
- [ ] **Audit log** — all remote actions logged with timestamp, client ID, and action
- [ ] **Kill switch** — `/remote kick <client>` or `/remote kill` to disconnect all remotes instantly
- [ ] **IP allowlist** — optional restriction to specific source networks

#### 3.1.4 Use Cases

| Use Case | Scenario |
|----------|----------|
| **Commuter review** | Review AI-generated code on your phone during commute |
| **Pair programming** | Teammate watches and sends suggestions via their browser |
| **CI trigger** | Trigger glaw-code tasks from a CI/CD pipeline over the internet |
| **Monitoring** | Keep an eye on a long-running autonomous task from another machine |
| **Remote server** | Run glaw-code on a beefy cloud VM, control from laptop |
| **IoT / headless** | Run glaw-code on a Raspberry Pi, interact from phone |

#### 3.1.5 Implementation Checklist

- [ ] `internal/modules/remote/domain/entity/` — tunnel session, pairing protocol, permission scope
- [ ] `internal/modules/remote/domain/service/` — encryption, relay protocol, authentication
- [ ] `internal/modules/remote/infrastructure/tunnel/` — WebSocket tunnel client, NaCl encryption
- [ ] `internal/modules/remote/infrastructure/relay/` — relay server implementation
- [ ] `internal/modules/remote/delivery/http/` — remote web UI endpoint, pairing page
- [ ] `cmd/glaw-relay/` — standalone relay binary
- [ ] `glaw tunnel start` / `glaw tunnel stop` CLI commands
- [ ] Integration tests: local ↔ relay ↔ remote round-trip
- [ ] Documentation: security model, self-hosting guide, architecture diagram

---

### 3.2 Multi-Modal Support (Vision)

**Priority:** P1 · **Effort:** XL

Enable glaw-code to understand images, screenshots, and diagrams.

- [ ] Add `image` content block type to `shared/api` types
- [ ] Support image input in CLI (paste from clipboard, file path argument)
- [ ] Support image input in Web UI (drag & drop, paste, file upload)
- [ ] Support image input via API (base64, URL)
- [ ] Convert images to provider-specific formats (Anthropic `image/*`, OpenAI `image_url`)
- [ ] Add `screenshot` tool — capture screen region and send to model
- [ ] Add `diagram` tool — render Mermaid/PlantUML and send as image

### 3.3 IDE Extension (VS Code)

**Priority:** P1 · **Effort:** XL

Bring glaw-code into the editor where developers spend most of their time.

- [ ] VS Code extension using the Language Server Protocol
- [ ] Inline chat panel (similar to GitHub Copilot Chat)
- [ ] Code actions: "Explain this", "Refactor this", "Fix this error"
- [ ] Inline diff view for suggested edits
- [ ] Terminal integration (run glaw-code in VS Code terminal)
- [ ] File context awareness (send open file + selection to model)
- [ ] Workspace symbol awareness via LSP integration

### 3.4 Enhanced LSP Integration

**Priority:** P1 · **Effort:** L

The current LSP integration is basic (connect, diagnostics, goto definition). Expand it significantly.

- [ ] Auto-detect and manage language servers (install if missing)
- [ ] Support `textDocument/rename` — AI-assisted refactoring
- [ ] Support `textDocument/codeAction` — quick fixes
- [ ] Support `textDocument/formatting` — AI formatting suggestions
- [ ] Support `textDocument/hover` — enrich hover with AI explanations
- [ ] Multi-root workspace support
- [ ] LSP health monitoring and auto-restart

### 3.5 Git Worktree & Branch Intelligence

**Priority:** P2 · **Effort:** M

- [ ] `/worktree create <name>` — create git worktree for isolated AI work
- [ ] `/worktree list` — show active worktrees with AI agents
- [ ] Auto-commit after each tool execution turn (optional)
- [ ] `/branch pr` — create PR from current branch with AI-generated description
- [ ] `/issue create` — create GitHub issue from conversation
- [ ] `/review` — AI code review of current diff
- [ ] Smart commit messages based on tool execution history

### 3.6 MCP Ecosystem Expansion

**Priority:** P1 · **Effort:** M

- [ ] Built-in MCP server registry (discover community servers)
- [ ] `mcp install <name>` — one-command MCP server installation
- [ ] MCP server health dashboard in Web UI
- [ ] MCP server sandboxing (run in container/namespace)
- [ ] MCP server configuration validation
- [ ] Support MCP `resources` protocol (read files via MCP)
- [ ] Support MCP `prompts` protocol (server-provided prompt templates)

---

## Phase 4 — Platform (Q3–Q4 2026)

> *Programmable, observable, and production-grade.*

### 4.1 Autonomous Work Mode

**Priority:** P0 · **Effort:** XL

Enable glaw-code to work independently on tasks for a configurable duration — the developer assigns work, walks away, and returns to completed, reviewed results.

#### 4.1.1 Core Autonomous Engine

- [ ] **`glaw work <duration>`** — start an autonomous work session
  ```bash
  glaw work 30m "add unit tests for all handlers"
  glaw work 2h "refactor the authentication module"
  glaw work --until 17:00 "implement the payment integration"
  glaw work --confirm    # review and approve the plan before starting
  ```
- [ ] **Time budgeting** — strictly enforce the time limit; gracefully wind down as deadline approaches
  - Last 20% of time: wrap up, write summary, save progress
  - Last 5%: stop tool execution, force summary generation
  - At deadline: save session, generate report, send notification
- [ ] **Scope boundaries** — define what the autonomous session can and cannot touch
  ```json
  {
    "scope": {
      "allow_paths": ["src/", "test/"],
      "deny_paths": [".env", "prod/", "secrets/"],
      "allow_tools": ["read_file", "write_file", "edit_file", "bash", "glob_search", "grep_search"],
      "deny_tools": ["web_fetch", "web_search"],
      "max_file_changes": 50,
      "max_cost_usd": 1.00
    }
  }
  ```
- [ ] **Checkpoint system** — auto-create git checkpoints at configurable intervals
  - `/autonomous checkpoint-interval 5m` — checkpoint every 5 minutes
  - Each checkpoint is a `git stash` or `git commit` with a descriptive message
  - Rolling checkpoints — keep last N, discard older ones
- [ ] **Progress tracking** — real-time progress dashboard accessible via remote tunnel
  - Current task status
  - Files modified count, tools called count, tokens consumed
  - Estimated time remaining
  - Live log of actions taken

#### 4.1.2 Safety & Guardrails

- [ ] **Hard boundaries** — autonomous mode MUST NOT exceed:
  - Time limit (strict enforcement)
  - Cost limit (max USD per session)
  - File change limit (max files modified)
  - Scope limit (only allowed paths/tools)
- [ ] **Pre-flight planning** — before autonomous work begins:
  - Generate a step-by-step plan
  - Estimate cost and time
  - Require explicit approval (`--confirm` flag) or auto-proceed (`--auto`)
- [ ] **Safety checks** between every tool call:
  - File scope validation (not modifying denied paths)
  - Cost check (haven't exceeded budget)
  - Test suite runner (optional: run tests after every N file changes)
  - Lint check (optional: reject changes that introduce lint errors)
- [ ] **Rollback plan** — if the autonomous session goes wrong:
  - All changes are in a git branch (not main)
  - One-command revert: `glaw work revert <session-id>`
  - Rollback to any checkpoint: `glaw work checkpoint <id>`
- [ ] **Notification system** — alert the developer when:
  - Session completes (success or failure)
  - Cost exceeds threshold (50%, 80%, 100% of budget)
  - Error rate exceeds threshold
  - Session is blocked (waiting for input that won't come)
  - Significant milestone reached (e.g., all tests passing)

#### 4.1.3 Notification Channels

- [ ] **Terminal notification** — macOS Notification Center, Linux `notify-send`
- [ ] **Webhook** — POST to a configurable URL on session events
  ```json
  {
    "event": "work.completed",
    "session_id": "sess_xxx",
    "duration": "28m",
    "files_changed": 12,
    "cost_usd": 0.34,
    "summary": "Added 23 unit tests across 5 files. All tests passing."
  }
  ```
- [ ] **Email** — SMTP integration for email notifications
- [ ] **Slack/Discord** — webhook integration for team chat notifications
- [ ] **Push notification** — via glaw remote tunnel to mobile device
- [ ] **Sound alert** — configurable sound when session completes

#### 4.1.4 Autonomous Work Patterns

| Pattern | Description | Example |
|---------|-------------|---------|
| **Fixer** | Fix a specific bug or test failure | `glaw work 15m "fix the flaky test in auth_test.go"` |
| **Implementer** | Implement a feature from a description | `glaw work 1h "add rate limiting to the API"` |
| **Reviewer** | Review and improve existing code | `glaw work 30m "review and improve error handling in handlers"` |
| **Tester** | Generate comprehensive tests | `glaw work 45m "achieve 80% coverage on the payment module"` |
| **Documenter** | Write documentation | `glaw work 20m "add godoc comments to all exported functions"` |
| **Refactorer** | Clean up code without changing behavior | `glaw work 30m "extract shared logic from duplicate handlers"` |
| **Upgrader** | Upgrade dependencies and fix breaking changes | `glaw work 1h "upgrade to React 19 and fix all deprecation warnings"` |
| **Overnighter** | Long autonomous session (6–10 hours) | `glaw work 8h "migrate the entire project from REST to gRPC"` |

#### 4.1.5 Work Session Report

When an autonomous session ends, glaw-code generates a comprehensive report:

```markdown
# Autonomous Work Report

**Session:** sess_1748765432
**Task:** "add unit tests for all handlers"
**Duration:** 28m 14s (of 30m budget)
**Status:** ✅ Completed

## Summary
Added 47 unit tests across 8 files. Achieved 83% coverage (up from 34%).

## Changes
| File | Action | Lines Changed |
|------|--------|---------------|
| handler_test.go | Created | +312 |
| middleware_test.go | Created | +89 |
| routes_test.go | Modified | +45 |

## Cost
- Tokens: 124K input / 38K output
- Estimated cost: $0.34

## Checkpoints
- `checkpoint-001` (5m) — initial test structure
- `checkpoint-002` (10m) — handler tests complete
- `checkpoint-003` (15m) — middleware tests added
- `checkpoint-004` (20m) — all tests passing
- `final` (28m) — documentation and cleanup

## Quality
- ✅ All 47 new tests passing
- ✅ No lint errors introduced
- ✅ No existing tests broken
```

- [ ] Auto-generate report in `.glaw/work-reports/<session-id>.md`
- [ ] Display report in terminal on completion
- [ ] Push report to webhook/notification channel
- [ ] `glaw work report <session-id>` — view any past report
- [ ] `glaw work history` — list all autonomous work sessions

#### 4.1.6 Implementation Checklist

- [ ] `internal/modules/work/domain/entity/` — WorkSession, WorkScope, WorkBudget, WorkCheckpoint, WorkReport
- [ ] `internal/modules/work/domain/service/` — planner, safety checker, progress tracker, time manager
- [ ] `internal/modules/work/application/usecase/` — StartWork, StopWork, GeneratePlan, Checkpoint, GenerateReport
- [ ] `internal/modules/work/application/dto/` — WorkInput, WorkOutput, WorkProgress, WorkStatus
- [ ] `internal/modules/work/infrastructure/persistence/` — session state persistence, checkpoint storage
- [ ] `internal/modules/work/infrastructure/notification/` — webhook, email, Slack, push notifications
- [ ] `internal/modules/work/delivery/cli/` — `glaw work` command, sub-commands, flags
- [ ] `internal/modules/work/tests/` — comprehensive tests for safety boundaries, time enforcement, cost limits
- [ ] Integration test: full autonomous session with mock LLM
- [ ] Documentation: configuration guide, safety model, notification setup

---

### 4.2 glaw-code SDK / Public API

**Priority:** P0 · **Effort:** XL

Enable other tools to programmatically drive glaw-code.

- [ ] gRPC/REST API server mode (`glaw serve --mode api`)
- [ ] Go SDK client library (`github.com/hieu-glaw/glaw-code/sdk`)
- [ ] TypeScript SDK client library (`@glaw-code/sdk`)
- [ ] Python SDK client library (`glaw-code-sdk`)
- [ ] API authentication (API key, JWT)
- [ ] Rate limiting and quota management
- [ ] OpenAPI 3.1 spec auto-generated from code
- [ ] API versioning strategy

### 4.3 Observability (OpenTelemetry)

**Priority:** P1 · **Effort:** L

- [ ] Add `internal/shared/telemetry/` with OpenTelemetry integration
- [ ] Trace every tool execution with span attributes (tool name, duration, success)
- [ ] Trace every API call (provider, model, tokens, latency)
- [ ] Trace every conversation turn (input/output token counts)
- [ ] Export to Jaeger, Zipkin, or OTLP-compatible backends
- [ ] Add `--telemetry-endpoint` and `--telemetry-enable` flags
- [ ] Metrics: request count, error rate, p50/p95/p99 latency, token throughput
- [ ] Health check endpoint (`/healthz`)

### 4.4 Agent Framework 2.0

**Priority:** P1 · **Effort:** XL

Evolve the sub-agent system from a simple task delegator into a full agent framework.

- [ ] **Agent DAG** — define agent workflows as directed acyclic graphs
- [ ] **Agent memory** — persistent memory store per agent (not just conversation history)
- [ ] **Agent skills** — reusable, composable skill modules that agents can load
- [ ] **Agent evaluation** — automated quality scoring for agent outputs
- [ ] **Agent marketplace** — share and discover community agents
- [ ] **Multi-agent collaboration** — agents can call other agents
- [ ] **Agent templates** — pre-built agent patterns (code-review, test-gen, refactor)
- [ ] **Human-in-the-loop agents** — agents that pause for human approval at key steps

### 4.5 Web UI 2.0

**Priority:** P1 · **Effort:** XL

A major upgrade to the web interface.

- [ ] **File explorer** — browse workspace files in sidebar
- [ ] **Inline diff viewer** — see proposed changes before applying
- [ ] **Split-pane editor** — edit code alongside chat
- [ ] **Terminal emulator** — embedded terminal for bash tool output
- [ ] **Multi-session tabs** — switch between conversations
- [ ] **Agent dashboard** — monitor running sub-agents
- [ ] **Cost & usage analytics** — visual spending tracker
- [ ] **Settings UI** — configure model, permissions, MCP servers from browser
- [ ] **Markdown preview** — render markdown files in workspace
- [ ] **Mobile-responsive** — use on tablet/phone
- [ ] **Dark/light theme toggle**

### 4.6 Plugin System 2.0

**Priority:** P2 · **Effort:** L

- [ ] WASM-based plugin sandboxing (run untrusted plugins safely)
- [ ] Plugin hot-reload (install/update without restart)
- [ ] Plugin marketplace with versioning
- [ ] Plugin configuration UI in web dashboard
- [ ] Plugin lifecycle hooks: `onSessionStart`, `onToolCall`, `onResponse`, `onFileChange`
- [ ] TypeScript plugin support (compile to WASM)
- [ ] Plugin signing and verification

---

## Phase 5 — Intelligence (Q1–Q2 2027)

> *Self-improving, context-aware, and proactive.*

### 5.1 Persistent Knowledge Base

**Priority:** P0 · **Effort:** XL

- [ ] **Local vector store** — index project codebase for semantic search
- [ ] **Code embeddings** — generate embeddings for functions, types, and files
- [ ] **Automatic context injection** — RAG system that fetches relevant code before each turn
- [ ] **Project knowledge graph** — understand code relationships (callers, callees, dependencies)
- [ ] **Cross-project learning** — optional shared knowledge across projects
- [ ] **Documentation indexing** — index README, docs, comments for context
- [ ] **`.glaw/knowledge/`** — persisted knowledge artifacts

### 5.2 Proactive Intelligence

**Priority:** P1 · **Effort:** XL

- [ ] **Background analysis** — run static analysis on file changes, surface issues proactively
- [ ] **Smart suggestions** — suggest fixes for lint errors, test failures
- [ ] **Test generation** — auto-generate tests for newly written functions
- [ ] **Dependency monitoring** — alert on vulnerable dependencies
- [ ] **Performance regression detection** — benchmark and alert on perf degradation
- [ ] **Watch mode** — `glaw watch` monitors file changes and offers suggestions

### 5.3 Multi-Agent Orchestration Engine

**Priority:** P1 · **Effort:** XL

- [ ] **Workflow editor** — visual DAG editor for agent workflows (Web UI)
- [ ] **Parallel agents** — run multiple agents concurrently on different tasks
- [ ] **Agent consensus** — multiple agents review and agree on changes before applying
- [ ] **Rollback-aware agents** — agents that can undo their changes
- [ ] **Agent evaluation pipeline** — CI-like testing for agent quality
- [ ] **Agent versioning** — pin agents to specific versions

### 5.4 Natural Language CI/CD

**Priority:** P2 · **Effort:** L

- [ ] `glaw ci` — run glaw-code as a CI check (code review bot)
- [ ] GitHub App integration — comment `@glaw review` on PRs
- [ ] GitLab, Bitbucket integrations
- [ ] Generate CI pipeline configs from natural language
- [ ] AI-powered test failure triage

### 5.5 Privacy & Security Hardening

**Priority:** P0 · **Effort:** L

- [ ] **Local-only mode** — ensure all processing stays on-device (Ollama + local embeddings)
- [ ] **Data classification** — tag files as sensitive, exclude from API calls
- [ ] **Secret scanning** — detect and redact API keys/passwords before sending to LLM
- [ ] **Audit log** — tamper-proof log of all tool executions and API calls
- [ ] **SOC 2 compliance** preparation (if enterprise offering)
- [ ] **Content Security Policy** for Web UI
- [ ] **Sandbox hardening** — improve Linux namespace isolation, add macOS sandbox

---

## Non-Goal Themes

These are explicitly **out of scope** for the 2-year roadmap:

| Non-Goal | Reason |
|----------|--------|
| **Building our own LLM** | Focus on being the best client/interface, not training models |
| **IDE from scratch** | Integrate with existing editors (VS Code, JetBrains) rather than building one |
| **Cloud hosting** | glaw-code runs locally; no SaaS offering planned |
| **Code completion** | Leave autocomplete to specialized tools (Copilot, Codeium, Tabnine) |
| **Supporting Python/Java/Rust rewrites** | Go is the core language; no rewrite planned |
| **Real-time collaboration** | Single-user tool; multi-user collab is a different product |
| **Mobile app** | Web UI is mobile-responsive; no native app planned |

---

## Success Metrics

| Metric | Current (2025) | Target (2027) |
|--------|---------------|---------------|
| **Test coverage** | ~55% avg | 80%+ across all packages |
| **Time to first token** | ~2–5s (blocking) | <500ms (streaming) |
| **Supported platforms** | 2 (macOS, Linux) | 3 (macOS, Linux, Windows) |
| **LLM providers** | 6 | 10+ (add Mistral, DeepSeek, Cohere, Amazon Bedrock, Azure OpenAI) |
| **Built-in tools** | 23 | 35+ |
| **MCP ecosystem** | Manual config | 50+ community servers in registry |
| **Web UI features** | Basic chat | Full IDE-like experience |
| **Startup time** | ~200ms | <100ms |
| **Memory (10K msgs)** | Unmeasured | <50MB RSS |
| **Remote access** | None | Encrypted tunnel with E2E encryption, multi-client |
| **Autonomous work** | None | Time/cost/scope-bounded with git checkpoints |
| **Notification channels** | None | Webhook, email, Slack/Discord, push, sound |
| **Contributors** | Core team | 20+ active contributors |
| **GitHub stars** | — | 5,000+ |

---

## How to Contribute to the Roadmap

This roadmap is a living document. To propose changes:

1. **Open a Discussion** on GitHub with the `roadmap` label
2. **File an Issue** for specific features you want to champion
3. **Submit a PR** against this document with your proposed changes
4. **Vote** on existing roadmap items using 👍 reactions on issues

### Priority Labels

| Label | Meaning |
|-------|---------|
| `P0` | Critical — must have in this phase |
| `P1` | Important — should have in this phase |
| `P2` | Nice to have — can slip to next phase |
| `effort:S` | < 1 week |
| `effort:M` | 1–4 weeks |
| `effort:L` | 1–3 months |
| `effort:XL` | 3–6 months |

---

## Timeline Overview

```
2025
  Q3 ████████ Phase 1: Solidify (module migration, streaming, logging, tests)
  Q4 ████████ Phase 2: Scale (Windows, context mgmt, resilience, perf)

2026
  Q1 ████████ Phase 3: Extend (remote access, multi-modal, VS Code, LSP, git, MCP)
  Q2 ████████
  Q3 ████████ Phase 4: Platform (autonomous work, SDK, observability, agents, web UI)
  Q4 ████████

2027
  Q1 ████████ Phase 5: Intelligence (knowledge base, proactive, orchestration)
  Q2 ████████
```

---

*Last updated: July 2025 · Completed 1.1 Module Migration, 2.3 basic cost tracking, 2.4 auto-retry & session auto-save*
*Next review: September 2025*
