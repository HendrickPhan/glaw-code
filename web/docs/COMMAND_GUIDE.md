# Slash Commands Visual Guide

## Quick Start

1. **Type `/`** in the input box to open the command palette
2. **Type to filter** commands (e.g., `/git` to see git commands)
3. **Use arrow keys** to navigate
4. **Press Enter** to execute
5. **Press Esc** to close

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `/` | Open command palette |
| `↑` `↓` | Navigate commands |
| `Tab` | Autocomplete command |
| `Enter` | Execute command |
| `Esc` | Close palette |

## Command Categories

### 🔧 Core Commands

Basic system commands for managing your session and configuration.

```
/help              Show all commands
/status            Show session status
/model [name]      Change AI model
/permissions [mode] Change permissions
/clear             Clear conversation
/cost              Show cost summary
/compact           Compact history
/version           Show version
/yolo              Toggle auto-approve
```

### 📁 Session Commands

Manage your conversation sessions.

```
/session list      List all sessions
/session new       Create new session
/session switch <id> Switch session
/resume <id>       Resume previous session
```

### 💼 Workspace Commands

Workspace and project management.

```
/init              Initialize .glaw directory
/diff              Show pending changes
/revert [all]      Revert changes
/analyze [mode]    Analyze project
```

### 🔀 Git Commands

Git version control operations.

```
/branch            Branch operations
/commit [msg]      Create commit
/pr                Pull requests
/issue             Issues
/commit-push-pr    Commit, push, PR
```

### 🤖 Automation Commands

Automation and planning tools.

```
/bughunter         Bug hunting mode
/ultraplan         Detailed planning
```

### 🧩 Agents & Skills

Agent and skill management.

```
/agents list       List agents
/agents show <name> Show agent details
/skills            List skills
/tasks             Manage tasks
```

## Examples

### Change Model
```
/model
→ Shows current model

/model claude-haiku-4
→ Changes to Haiku model
```

### Session Management
```
/session new
→ Creates: session-456

/session switch session-123
→ Switches to: session-123
```

### Git Operations
```
/diff
→ Shows: changes in working directory

/commit "Fix bug in parser"
→ Creates: commit with message
```

### Project Analysis
```
/analyze
→ Full project analysis

/analyze summary
→ Quick overview

/analyze graph
→ Dependency graph
```

## Command Palette UI

The command palette shows:

```
┌─────────────────────────────────────────┐
│  CORE COMMANDS                          │
├─────────────────────────────────────────┤
│  /help          Show available commands │ ← Selected
│  /status        Show runtime status     │
│  /model         Show or change model    │
│  /clear         Clear conversation      │
├─────────────────────────────────────────┤
│  SESSION COMMANDS                       │
├─────────────────────────────────────────┤
│  /session new   Create new session      │
│  /session list  List all sessions       │
└─────────────────────────────────────────┘
   ↑↓ Navigate  Tab Autocomplete  Enter Select
```

## Command Result Display

Command results are displayed with:

- **Icon** - Based on command type
- **Color coding** - Success (green), Error (red), Info (blue), Warning (yellow)
- **Formatted output** - Clean, readable text
- **Context** - Shows which command was executed

### Example Outputs

**Success:**
```
✓ Session created: session-456
  Previous session saved automatically.
```

**Error:**
```
✗ Error: Session not found
  Use /session list to see available sessions.
```

**Info:**
```
ℹ Session: session-123
   Messages: 42
   Model: claude-sonnet-4
```

## Tips & Tricks

1. **Quick Help** - Type `/help` to see all commands
2. **Fuzzy Search** - Type partial command names (e.g., `/git` for `/git commit`)
3. **Keyboard Power User** - Use arrow keys + enter for fast command execution
4. **Command History** - Commands are saved in session history
5. **Session Persistence** - Commands and results persist across sessions

## Limitations

Some commands have web-specific behavior:

- **File operations** - Limited in web mode for security
- **Git operations** - Use CLI for full git functionality
- **Memory/config** - Web mode shows instructions, use CLI for persistence

## Need Help?

- Type `/help` for command reference
- Check the [Documentation](./) for detailed guides
- Use `/status` to see current session info
- Try `/agents list` to see available agents

---

*For developers: See [`../SLASH_COMMANDS.md`](../SLASH_COMMANDS.md) for implementation details.*
