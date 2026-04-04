# Slash Commands — Definitions, Parsing, and Dispatch Logic

## Package: `internal/commands/`

---

## Command Parsing

### SlashCommand.Parse

```go
func Parse(input string) (*SlashCommand, string)
```

**Algorithm:**
```
1. Trim whitespace from input
2. IF input does not start with '/': return nil, input (not a slash command)
3. Extract command name: first word after '/'
4. Extract remainder: everything after the command name (trimmed)
5. Look up command name in SLASH_COMMAND_SPECS
   - Try exact match first
   - Try alias match second
   - IF no match: return nil with suggestion
6. Return parsed SlashCommand variant + remainder
```

### Fuzzy Suggestion

```go
func SuggestSlashCommands(input string) []SlashCommandSpec
```

**Algorithm:**
```
1. Extract command name from input (strip leading '/')
2. FOR EACH command spec:
   - Compute Levenshtein distance between input and command name
   - Compute distance for each alias
   - Use minimum distance
   - IF distance <= 2: include in suggestions
3. Sort suggestions by distance (ascending)
4. Return suggested commands
```

---

## Command Dispatch

### handle_slash_command

```go
func HandleSlashCommand(ctx context.Context, cmd SlashCommand, runtime *ConversationRuntime) (*SlashCommandResult, error)
```

**Dispatch table:**

| Command | Handler | Logic |
|---------|---------|-------|
| help | Direct | Print all command specs with aliases and summaries |
| status | Direct | Print runtime state: model, session ID, message count, permission mode |
| compact | Direct | Call runtime.CompactSession(), print summary |
| model | Direct | IF remainder: change model; ELSE: print current model |
| permissions | Direct | IF remainder: change mode; ELSE: print current mode |
| clear | Direct | Clear session messages, print confirmation |
| cost | Direct | Print usage tracker: total tokens, estimated cost |
| resume | Runtime | Load session from .claw/sessions/{id}, restore conversation |
| config | Runtime | Read/write config values (see Config tool) |
| memory | Runtime | Manage .claw/memory/ files |
| init | Direct | Create .claw/ directory structure with defaults |
| diff | Direct | Run git diff, display pending changes |
| version | Direct | Print version string |
| bughunter | Automation | Activate bug hunting agent mode |
| branch | Git | Handle git branch operations (create, switch, list) |
| worktree | Git | Handle git worktree operations (create, remove) |
| commit | Git | Stage files, create git commit with message |
| commit-push-pr | Git | Commit + push + create PR via gh CLI |
| pr | Git | PR operations via gh CLI (create, list, view, checkout) |
| issue | Git | Issue operations via gh CLI (create, list, comment) |
| ultraplan | Automation | Detailed planning mode with task breakdown |
| teleport | Core | Switch to remote session transport |
| debug-tool-call | Core | Enable verbose tool call logging |
| export | Session | Export session as markdown/JSON |
| session | Session | List/load/delete sessions |
| plugin | Plugin | Plugin CRUD (install, enable, disable, list, remove) |
| agents | Core | Scan agent directories, list available agents |
| skills | Core | Scan skill directories, list available skills |

**Handler returns:**
- `nil` → command was handled internally, continue REPL
- `SlashCommandResult{Action: "quit"}` → exit REPL
- `SlashCommandResult{Action: "continue"}` → continue REPL with side effects applied

---

## Git Command Details

### commit

```
handle_commit(command, remainder):
  1. Run git status (capture untracked, modified files)
  2. Run git diff (staged + unstaged)
  3. Analyze changes, draft commit message
  4. Stage relevant files (git add specific files, NOT git add -A)
  5. Run git commit with message
  6. IF pre-commit hook fails: fix issue, retry as NEW commit (never --no-verify)
  7. Print commit result
```

### commit-push-pr

```
handle_commit_push_pr(command, remainder):
  1. Run handle_commit() first
  2. Get current branch name
  3. IF branch doesn't track remote: git push -u origin {branch}
  4. Create PR via: gh pr create --title "..." --body "..."
  5. Print PR URL
```

### pr

```
handle_pr(remainder):
  Parse subcommand:
  - "create": Interactive PR creation
  - "list": gh pr list
  - "view N": gh pr view N
  - "checkout N": gh pr checkout N
  - "merge N": gh pr merge N
  Default: list recent PRs
```

### issue

```
handle_issue(remainder):
  Parse subcommand:
  - "create": Create GitHub issue
  - "list": gh issue list
  - "comment N": Add comment to issue
  Default: list recent issues
```

### branch

```
handle_branch(remainder):
  Parse subcommand:
  - "create NAME": git checkout -b NAME
  - "switch NAME": git checkout NAME
  - "list": git branch -a
  - "delete NAME": git branch -d NAME
  Default: list branches
```

### worktree

```
handle_worktree(remainder):
  Parse subcommand:
  - "create NAME": git worktree add .claw/worktrees/NAME
  - "remove NAME": git worktree remove .claw/worktrees/NAME
  Default: list worktrees
```

---

## Agent and Skill Discovery

### Agent Discovery

```
handle_agents():
  Scan directories in order:
  1. .codex/agents/
  2. .claw/agents/
  3. $CODEX_HOME/agents/ (env var)
  4. ~/.codex/agents/
  5. ~/.claw/agents/

  FOR EACH directory:
    FOR EACH .md file:
      Parse frontmatter (name, description, model, tools)
      Add to agent list with source tag

  Display agent list with source indicator (project vs user)
  Shadowing: later directories override earlier ones
```

### Skill Discovery

```
handle_skills():
  Scan directories (same as agents but for skills/):
  1. .codex/skills/
  2. .claw/skills/
  3. $CODEX_HOME/skills/
  4. ~/.codex/skills/
  5. ~/.claw/skills/

  FOR EACH .md file:
    Parse frontmatter (name, description, trigger conditions)
    Add to skill list

  Display skill list
```

### DefinitionSource (shadowing order)

```go
type DefinitionSource int
// Priority (highest first):
// 1. ProjectCodex   (.codex/ in project)
// 2. ProjectClaw    (.claw/ in project)
// 3. UserCodexHome  ($CODEX_HOME)
// 4. UserCodex      (~/.codex/)
// 5. UserClaw       (~/.claw/)
```

When two definitions have the same name, the higher-priority source wins.
