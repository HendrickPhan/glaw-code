package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/tasks"
)

// Category groups commands.
type Category string

const (
	CategoryCore       Category = "core"
	CategoryWorkspace  Category = "workspace"
	CategorySession    Category = "session"
	CategoryGit        Category = "git"
	CategoryAutomation Category = "automation"
)

// Spec defines a slash command.
type Spec struct {
	Name            string
	Aliases         []string
	Summary         string
	ArgumentHint    string
	ResumeSupported bool
	Category        Category
}

// Result is returned after handling a command.
type Result struct {
	Action  string // "continue" | "quit"
	Message string
}

// Runtime provides the interface commands need.
type Runtime interface {
	GetModel() string
	SetModel(model string)
	GetPermissionMode() string
	SetPermissionMode(mode string)
	GetMessageCount() int
	GetSessionID() string
	GetUsageInfo() UsageInfo
	CompactSession() error
	ClearSession()
	RunGitCommand(args ...string) (string, error)
	GetWorkspaceRoot() string
	GetAllSettings() map[string]interface{}
	SetConfigValue(key, value string) error
	GetLSPStatus() []LSPServerStatus
	RevertLastTurn() (int, error)
	RevertAll() (int, error)
}

// LSPServerStatus represents the status of an LSP server for display.
type LSPServerStatus struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Running bool   `json:"running"`
}

// UsageInfo holds usage statistics.
type UsageInfo struct {
	InputTokens  int
	OutputTokens int
	TotalCostUSD float64
}

// Specs lists all available slash commands.
var Specs = []Spec{
	{Name: "help", Aliases: []string{"h", "?"}, Summary: "Show available commands", Category: CategoryCore},
	{Name: "status", Aliases: []string{"st"}, Summary: "Show runtime status", Category: CategoryCore},
	{Name: "compact", Summary: "Compact conversation history", Category: CategoryCore},
	{Name: "model", Summary: "Show or change model", ArgumentHint: "[model]", Category: CategoryCore},
	{Name: "permissions", Aliases: []string{"perm"}, Summary: "Show or change permission mode", ArgumentHint: "[mode]", Category: CategoryCore},
	{Name: "clear", Summary: "Clear conversation", Category: CategoryCore},
	{Name: "cost", Summary: "Show cost summary", Category: CategoryCore},
	{Name: "resume", Summary: "Resume previous session", ArgumentHint: "<session-id>", ResumeSupported: true, Category: CategorySession},
	{Name: "config", Summary: "Read/write configuration", ArgumentHint: "[key] [value]", Category: CategoryCore},
	{Name: "memory", Summary: "Manage memory/context", ArgumentHint: "[add|read|delete] [name] [content]", Category: CategoryCore},
	{Name: "init", Summary: "Initialize .glaw directory", Category: CategoryWorkspace},
	{Name: "diff", Summary: "Show pending changes", Category: CategoryWorkspace},
	{Name: "version", Aliases: []string{"v"}, Summary: "Show version", Category: CategoryCore},
	{Name: "bughunter", Summary: "Bug hunting mode", ArgumentHint: "[target]", Category: CategoryAutomation},
	{Name: "branch", Summary: "Git branch operations", ArgumentHint: "[create|switch|list|delete] [name]", Category: CategoryGit},
	{Name: "worktree", Summary: "Git worktree operations", ArgumentHint: "[create|remove|list] [name]", Category: CategoryGit},
	{Name: "commit", Summary: "Create a git commit", ArgumentHint: "[message]", Category: CategoryGit},
	{Name: "commit-push-pr", Aliases: []string{"cpp"}, Summary: "Commit, push, and create PR", ArgumentHint: "[message]", Category: CategoryGit},
	{Name: "pr", Summary: "Pull request operations", ArgumentHint: "[create|list|view|checkout] [args]", Category: CategoryGit},
	{Name: "issue", Summary: "GitHub issue operations", ArgumentHint: "[create|list|view] [args]", Category: CategoryGit},
	{Name: "ultraplan", Summary: "Detailed planning mode", ArgumentHint: "[task description]", Category: CategoryAutomation},
	{Name: "teleport", Summary: "Remote session teleport", ArgumentHint: "<host:port>", Category: CategoryCore},
	{Name: "debug-tool-call", Summary: "Debug tool call tracing", ArgumentHint: "[on|off]", Category: CategoryCore},
	{Name: "export", Summary: "Export session data", ArgumentHint: "[filename]", Category: CategorySession},
	{Name: "session", Summary: "Session management", ArgumentHint: "[list|load|delete]", Category: CategorySession},
	{Name: "plugin", Summary: "Plugin management", ArgumentHint: "[install|enable|disable|list]", Category: CategoryCore},
	{Name: "agents", Summary: "List available agents", Category: CategoryCore},
	{Name: "skills", Summary: "List available skills", Category: CategoryCore},
	{Name: "btw", Summary: "Ask a side question during execution", ArgumentHint: "<question>", Category: CategoryCore},
	{Name: "tasks", Summary: "Manage agent tasks", ArgumentHint: "[create|list|update|delete] [args]", Category: CategoryCore},
	{Name: "lsp", Summary: "LSP server management and status", ArgumentHint: "[status|restart|detect]", Category: CategoryWorkspace},
{Name: "revert", Aliases: []string{"undo"}, Summary: "Revert file changes from the last turn", ArgumentHint: "[all]", Category: CategoryWorkspace},
}

// ParsedCommand is a parsed slash command.
type ParsedCommand struct {
	Spec      Spec
	Remainder string
}

// Parse parses a slash command from input.
func Parse(input string) (*ParsedCommand, string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil, input
	}

	input = input[1:] // strip leading /
	parts := strings.SplitN(input, " ", 2)
	name := parts[0]
	remainder := ""
	if len(parts) > 1 {
		remainder = strings.TrimSpace(parts[1])
	}

	// Find matching spec
	for _, spec := range Specs {
		if spec.Name == name {
			return &ParsedCommand{Spec: spec, Remainder: remainder}, remainder
		}
		for _, alias := range spec.Aliases {
			if alias == name {
				return &ParsedCommand{Spec: spec, Remainder: remainder}, remainder
			}
		}
	}

	return nil, name
}

// SuggestCommands returns fuzzy-matched command specs.
func SuggestCommands(input string) []Spec {
	input = strings.TrimPrefix(input, "/")
	var suggestions []Spec
	for _, spec := range Specs {
		dist := levenshtein(input, spec.Name)
		if dist <= 2 {
			suggestions = append(suggestions, spec)
			continue
		}
		for _, alias := range spec.Aliases {
			if levenshtein(input, alias) <= 2 {
				suggestions = append(suggestions, spec)
				break
			}
		}
	}
	return suggestions
}

// Dispatcher handles slash command dispatch.
type Dispatcher struct {
	runtime   Runtime
	taskStore *tasks.Store
}

// NewDispatcher creates a new command dispatcher.
func NewDispatcher(rt Runtime) *Dispatcher {
	var taskPath string
	if root := rt.GetWorkspaceRoot(); root != "" {
		taskPath = filepath.Join(root, ".glaw", "tasks.json")
	}
	return &Dispatcher{
		runtime:   rt,
		taskStore: tasks.NewStore(taskPath),
	}
}

// Handle parses and dispatches a slash command.
func (d *Dispatcher) Handle(ctx context.Context, input string) (*Result, error) {
	parsed, _ := Parse(input)
	if parsed == nil {
		// Unknown command, suggest alternatives
		suggestions := SuggestCommands(input)
		if len(suggestions) > 0 {
			names := make([]string, len(suggestions))
			for i, s := range suggestions {
				names[i] = "/" + s.Name
			}
			return &Result{
				Action:  "continue",
				Message: fmt.Sprintf("Unknown command. Did you mean: %s?", strings.Join(names, ", ")),
			}, nil
		}
		return &Result{Action: "continue", Message: "Unknown command. Type /help for available commands."}, nil
	}

	return d.dispatch(ctx, parsed)
}

func (d *Dispatcher) dispatch(ctx context.Context, cmd *ParsedCommand) (*Result, error) {
	switch cmd.Spec.Name {
	case "help":
		return d.handleHelp()
	case "status":
		return d.handleStatus()
	case "version":
		return &Result{Action: "continue", Message: "glaw-code v1.0.0"}, nil
	case "model":
		return d.handleModel(cmd.Remainder)
	case "permissions":
		return d.handlePermissions(cmd.Remainder)
	case "clear":
		d.runtime.ClearSession()
		return &Result{Action: "continue", Message: "Conversation cleared."}, nil
	case "cost":
		return d.handleCost()
	case "compact":
		if err := d.runtime.CompactSession(); err != nil {
			return &Result{Action: "continue", Message: "Compact failed: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Session compacted."}, nil
	case "diff":
		return d.handleDiff(ctx)
	case "commit":
		return d.handleCommit(ctx, cmd.Remainder)
	case "branch":
		return d.handleBranch(ctx, cmd.Remainder)
	case "worktree":
		return d.handleWorktree(ctx, cmd.Remainder)
	case "init":
		return d.handleInit()
	case "config":
		return d.handleConfig(cmd.Remainder)
	case "memory":
		return d.handleMemory(cmd.Remainder)
	case "resume":
		return d.handleResume(cmd.Remainder)
	case "session":
		return d.handleSession(cmd.Remainder)
	case "export":
		return d.handleExport(cmd.Remainder)
	case "plugin":
		return d.handlePlugin(cmd.Remainder)
	case "agents":
		return d.handleAgents()
	case "skills":
		return d.handleSkills()
	case "bughunter":
		return d.handleBughunter(cmd.Remainder)
	case "commit-push-pr":
		return d.handleCommitPushPR(cmd.Remainder)
	case "pr":
		return d.handlePR(cmd.Remainder)
	case "issue":
		return d.handleIssue(cmd.Remainder)
	case "ultraplan":
		return d.handleUltraplan(cmd.Remainder)
	case "teleport":
		return d.handleTeleport(cmd.Remainder)
	case "debug-tool-call":
		return d.handleDebugToolCall(cmd.Remainder)
	case "btw":
		return d.handleBTW(cmd.Remainder)
	case "tasks":
		return d.handleTasks(cmd.Remainder)
	case "lsp":
		return d.handleLSP(cmd.Remainder)
	case "revert":
		return d.handleRevert(cmd.Remainder)
	case "quit", "exit":
		return &Result{Action: "quit", Message: "Goodbye!"}, nil
	default:
		return &Result{Action: "continue", Message: fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmd.Spec.Name)}, nil
	}
}

func (d *Dispatcher) handleHelp() (*Result, error) {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	categories := []Category{CategoryCore, CategoryWorkspace, CategorySession, CategoryGit, CategoryAutomation}
	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("  %s:\n", strings.ToUpper(string(cat[:1])) + string(cat[1:])))
		for _, spec := range Specs {
			if spec.Category == cat {
				aliases := ""
				if len(spec.Aliases) > 0 {
					aliases = fmt.Sprintf(" (%s)", strings.Join(spec.Aliases, ", "))
				}
				hint := ""
				if spec.ArgumentHint != "" {
					hint = " " + spec.ArgumentHint
				}
				sb.WriteString(fmt.Sprintf("    /%s%s%s - %s\n", spec.Name, hint, aliases, spec.Summary))
			}
		}
	}

	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleStatus() (*Result, error) {
	msg := fmt.Sprintf("Model: %s\nSession: %s\nMessages: %d\nPermissions: %s",
		d.runtime.GetModel(),
		d.runtime.GetSessionID(),
		d.runtime.GetMessageCount(),
		d.runtime.GetPermissionMode(),
	)
	return &Result{Action: "continue", Message: msg}, nil
}

func (d *Dispatcher) handleModel(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Current model: %s", d.runtime.GetModel())}, nil
	}
	d.runtime.SetModel(remainder)
	return &Result{Action: "continue", Message: fmt.Sprintf("Model set to: %s", remainder)}, nil
}

func (d *Dispatcher) handlePermissions(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Current permissions: %s", d.runtime.GetPermissionMode())}, nil
	}
	d.runtime.SetPermissionMode(remainder)
	return &Result{Action: "continue", Message: fmt.Sprintf("Permissions set to: %s", remainder)}, nil
}

func (d *Dispatcher) handleCost() (*Result, error) {
	info := d.runtime.GetUsageInfo()
	msg := fmt.Sprintf("Tokens: %d in + %d out\nCost: $%.4f", info.InputTokens, info.OutputTokens, info.TotalCostUSD)
	return &Result{Action: "continue", Message: msg}, nil
}

func (d *Dispatcher) handleDiff(ctx context.Context) (*Result, error) {
	output, err := d.runtime.RunGitCommand("diff")
	if err != nil {
		return &Result{Action: "continue", Message: "Not in a git repository or git not available."}, nil
	}
	if output == "" {
		return &Result{Action: "continue", Message: "No pending changes."}, nil
	}
	return &Result{Action: "continue", Message: output}, nil
}

func (d *Dispatcher) handleCommit(ctx context.Context, message string) (*Result, error) {
	if message == "" {
		message = "Update files"
	}

	// Stage all changes
	if _, err := d.runtime.RunGitCommand("add", "-A"); err != nil {
		return &Result{Action: "continue", Message: "Failed to stage files: " + err.Error()}, nil
	}

	// Commit
	output, err := d.runtime.RunGitCommand("commit", "-m", message)
	if err != nil {
		return &Result{Action: "continue", Message: "Commit failed: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: "Committed: " + output}, nil
}

func (d *Dispatcher) handleBranch(ctx context.Context, remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		output, err := d.runtime.RunGitCommand("branch", "-a")
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list branches."}, nil
		}
		return &Result{Action: "continue", Message: output}, nil
	}

	switch parts[0] {
	case "create":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /branch create <name>"}, nil
		}
		output, err := d.runtime.RunGitCommand("checkout", "-b", parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Created branch: " + parts[1] + "\n" + output}, nil
	case "switch":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /branch switch <name>"}, nil
		}
		_, err := d.runtime.RunGitCommand("checkout", parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Switched to: " + parts[1]}, nil
	case "delete":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /branch delete <name>"}, nil
		}
		output, err := d.runtime.RunGitCommand("branch", "-d", parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Deleted branch: " + parts[1] + "\n" + output}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /branch [create|switch|list|delete] [name]"}, nil
	}
}

func (d *Dispatcher) handleWorktree(ctx context.Context, remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		output, err := d.runtime.RunGitCommand("worktree", "list")
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list worktrees."}, nil
		}
		return &Result{Action: "continue", Message: output}, nil
	}

	switch parts[0] {
	case "create":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /worktree create <name>"}, nil
		}
		output, err := d.runtime.RunGitCommand("worktree", "add", ".glaw/worktrees/"+parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Created worktree: " + parts[1] + "\n" + output}, nil
	case "remove":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /worktree remove <name>"}, nil
		}
		_, err := d.runtime.RunGitCommand("worktree", "remove", ".glaw/worktrees/"+parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: err.Error()}, nil
		}
		return &Result{Action: "continue", Message: "Removed worktree: " + parts[1]}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /worktree [create|remove|list] [name]"}, nil
	}
}

func (d *Dispatcher) handleInit() (*Result, error) {
	dirs := []string{".glaw", ".glaw/sessions", ".glaw/agents", ".glaw/skills", ".glaw/plugins", ".glaw/memory", ".glaw/exports"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return &Result{Action: "continue", Message: "Failed to create " + dir + ": " + err.Error()}, nil
		}
	}

	// Create default settings.json
	settings := []byte(`{"permissions": {"mode": "workspace_write"}, "model": "claude-sonnet-4-6"}`)
	if err := os.WriteFile(".glaw/settings.json", settings, 0o644); err != nil {
		return &Result{Action: "continue", Message: "Failed to write settings: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: "Initialized .glaw/ directory structure."}, nil
}

func (d *Dispatcher) handleConfig(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)

	if len(parts) == 0 {
		// Show all config
		settings := d.runtime.GetAllSettings()
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return &Result{Action: "continue", Message: "Error serializing config"}, nil
		}
		return &Result{Action: "continue", Message: string(data)}, nil
	}

	if len(parts) == 1 {
		// Show single key
		settings := d.runtime.GetAllSettings()
		val, ok := settings[parts[0]]
		if !ok {
			return &Result{Action: "continue", Message: fmt.Sprintf("Key %q not found in config.", parts[0])}, nil
		}
		data, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return &Result{Action: "continue", Message: fmt.Sprintf("%v", val)}, nil
		}
		return &Result{Action: "continue", Message: string(data)}, nil
	}

	// Set a value: /config <key> <value>
	key := parts[0]
	value := strings.Join(parts[1:], " ")
	if err := d.runtime.SetConfigValue(key, value); err != nil {
		return &Result{Action: "continue", Message: "Error: " + err.Error()}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Set %s = %s", key, value)}, nil
}

// --- Memory management ---

func (d *Dispatcher) handleMemory(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		memDir := filepath.Join(".glaw", "memory")
		entries, err := os.ReadDir(memDir)
		if err != nil {
			return &Result{Action: "continue", Message: "No memory files found. Use /memory add <name> <content> to create one."}, nil
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return &Result{Action: "continue", Message: "No memory files found."}, nil
		}
		return &Result{Action: "continue", Message: "Memory files:\n  " + strings.Join(names, "\n  ")}, nil
	}

	switch parts[0] {
	case "add", "write":
		if len(parts) < 3 {
			return &Result{Action: "continue", Message: "Usage: /memory add <name> <content>"}, nil
		}
		name := parts[1]
		content := strings.Join(parts[2:], " ")
		memDir := filepath.Join(".glaw", "memory")
		if err := os.MkdirAll(memDir, 0o755); err != nil {
			return &Result{Action: "continue", Message: "Error creating memory directory: " + err.Error()}, nil
		}
		path := filepath.Join(memDir, name+".md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return &Result{Action: "continue", Message: "Error writing memory: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Memory saved: %s", name)}, nil
	case "read":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /memory read <name>"}, nil
		}
		path := filepath.Join(".glaw", "memory", parts[1]+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			return &Result{Action: "continue", Message: "Memory not found: " + parts[1]}, nil
		}
		return &Result{Action: "continue", Message: string(data)}, nil
	case "delete", "remove":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /memory delete <name>"}, nil
		}
		path := filepath.Join(".glaw", "memory", parts[1]+".md")
		if err := os.Remove(path); err != nil {
			return &Result{Action: "continue", Message: "Error deleting memory: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Memory deleted: %s", parts[1])}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /memory [add|read|delete] [name] [content]"}, nil
	}
}

// --- Resume ---

func (d *Dispatcher) handleResume(remainder string) (*Result, error) {
	if remainder == "" {
		sessionsDir := filepath.Join(".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return &Result{Action: "continue", Message: "No sessions found. Usage: /resume <session-id>"}, nil
		}
		var sessions []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				name := strings.TrimSuffix(e.Name(), ".json")
				sessions = append(sessions, "  "+name)
			}
		}
		if len(sessions) == 0 {
			return &Result{Action: "continue", Message: "No saved sessions found."}, nil
		}
		return &Result{Action: "continue", Message: "Available sessions:\n" + strings.Join(sessions, "\n") + "\n\nUsage: /resume <session-id>"}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("To resume session %s, restart with: glaw-code --session %s", remainder, remainder)}, nil
}

// --- Session management ---

func (d *Dispatcher) handleSession(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		return &Result{Action: "continue", Message: fmt.Sprintf("Current session: %s\nMessages: %d", d.runtime.GetSessionID(), d.runtime.GetMessageCount())}, nil
	}

	switch parts[0] {
	case "list":
		sessionsDir := filepath.Join(".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return &Result{Action: "continue", Message: "No sessions directory found."}, nil
		}
		var sessions []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				info, err := e.Info()
				if err != nil {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".json")
				sessions = append(sessions, fmt.Sprintf("  %s  (%s)", name, info.ModTime().Format("2006-01-02 15:04")))
			}
		}
		if len(sessions) == 0 {
			return &Result{Action: "continue", Message: "No saved sessions found."}, nil
		}
		return &Result{Action: "continue", Message: "Saved sessions:\n" + strings.Join(sessions, "\n")}, nil
	case "load":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /session load <session-id>"}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("To resume session %s, restart with: glaw-code --session %s", parts[1], parts[1])}, nil
	case "delete":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /session delete <session-id>"}, nil
		}
		path := filepath.Join(".glaw", "sessions", parts[1]+".json")
		if err := os.Remove(path); err != nil {
			return &Result{Action: "continue", Message: "Failed to delete session: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Session %s deleted.", parts[1])}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /session [list|load|delete] [session-id]"}, nil
	}
}

// --- Export ---

func (d *Dispatcher) handleExport(remainder string) (*Result, error) {
	sessionID := d.runtime.GetSessionID()
	if remainder == "" {
		remainder = fmt.Sprintf("session-export-%s.json", sessionID)
	}

	exportDir := filepath.Join(".glaw", "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return &Result{Action: "continue", Message: "Failed to create export directory: " + err.Error()}, nil
	}

	exportPath := remainder
	if !filepath.IsAbs(exportPath) {
		exportPath = filepath.Join(exportDir, exportPath)
	}

	settings := d.runtime.GetAllSettings()
	exportData := map[string]interface{}{
		"session_id":    sessionID,
		"exported_at":   time.Now().Format(time.RFC3339),
		"model":         d.runtime.GetModel(),
		"permissions":   d.runtime.GetPermissionMode(),
		"message_count": d.runtime.GetMessageCount(),
		"usage":         d.runtime.GetUsageInfo(),
		"settings":      settings,
	}

	data, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return &Result{Action: "continue", Message: "Error serializing export: " + err.Error()}, nil
	}

	if err := os.WriteFile(exportPath, data, 0o644); err != nil {
		return &Result{Action: "continue", Message: "Error writing export: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Session exported to: %s", exportPath)}, nil
}

// --- Plugin management ---

func (d *Dispatcher) handlePlugin(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		return &Result{Action: "continue", Message: "Plugin management.\nUsage: /plugin [install|enable|disable|list] [name]"}, nil
	}

	switch parts[0] {
	case "list":
		pluginDir := filepath.Join(".glaw", "plugins")
		entries, err := os.ReadDir(pluginDir)
		if err != nil {
			return &Result{Action: "continue", Message: "No plugins directory found. Use /init to create project structure."}, nil
		}
		var plugins []string
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifestPath := filepath.Join(pluginDir, e.Name(), "manifest.json")
			pluginData, err := os.ReadFile(manifestPath)
			if err != nil {
				plugins = append(plugins, fmt.Sprintf("  %s (no manifest)", e.Name()))
				continue
			}
			var manifest struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}
			if json.Unmarshal(pluginData, &manifest) == nil {
				plugins = append(plugins, fmt.Sprintf("  %s v%s", manifest.Name, manifest.Version))
			} else {
				plugins = append(plugins, fmt.Sprintf("  %s", e.Name()))
			}
		}
		if len(plugins) == 0 {
			return &Result{Action: "continue", Message: "No plugins installed."}, nil
		}
		return &Result{Action: "continue", Message: "Plugins:\n" + strings.Join(plugins, "\n")}, nil
	case "install":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /plugin install <path-to-manifest>"}, nil
		}
		srcData, err := os.ReadFile(parts[1])
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to read manifest: " + err.Error()}, nil
		}
		var manifest struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(srcData, &manifest); err != nil || manifest.Name == "" {
			return &Result{Action: "continue", Message: "Invalid plugin manifest: missing name field"}, nil
		}
		pluginDir := filepath.Join(".glaw", "plugins", manifest.Name)
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
			return &Result{Action: "continue", Message: "Failed to create plugin directory: " + err.Error()}, nil
		}
		dstPath := filepath.Join(pluginDir, "manifest.json")
		if err := os.WriteFile(dstPath, srcData, 0o644); err != nil {
			return &Result{Action: "continue", Message: "Failed to install plugin: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Plugin %s installed. Restart to activate.", manifest.Name)}, nil
	case "enable":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /plugin enable <name>"}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Plugin %s will be enabled on next session restart.", parts[1])}, nil
	case "disable":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /plugin disable <name>"}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Plugin %s will be disabled on next session restart.", parts[1])}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /plugin [install|enable|disable|list] [name]"}, nil
	}
}

// --- Agents ---

func (d *Dispatcher) handleAgents() (*Result, error) {
	agents := []struct {
		name, description string
	}{
		{"general-purpose", "General-purpose agent for research, search, and multi-step tasks"},
		{"Explore", "Fast agent for exploring codebases - file search, content search"},
		{"Plan", "Software architect agent for designing implementation plans"},
	}
	var sb strings.Builder
	sb.WriteString("Available Agents:\n\n")
	for _, a := range agents {
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", a.name, a.description))
	}
	return &Result{Action: "continue", Message: sb.String()}, nil
}

// --- Skills ---

func (d *Dispatcher) handleSkills() (*Result, error) {
	skills := []struct {
		name, description string
	}{
		{"commit", "Create a git commit with staged changes"},
		{"review-pr", "Review a pull request"},
		{"pdf", "Read and analyze PDF files"},
		{"context7-mcp", "Fetch current library/framework documentation"},
		{"claude-api", "Build apps with the Claude API or Anthropic SDK"},
		{"simplify", "Review and simplify changed code"},
	}
	var sb strings.Builder
	sb.WriteString("Available Skills:\n\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", s.name, s.description))
	}
	return &Result{Action: "continue", Message: sb.String()}, nil
}

// --- Bughunter ---

func (d *Dispatcher) handleBughunter(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: "Bughunter mode: systematically finds and fixes bugs.\nUsage: /bughunter [target]\nProvide a file, directory, or description of the area to hunt bugs in."}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Bughunter mode activated. Target: %s\nThe next message you send will be analyzed for bugs, security issues, and edge cases.", remainder)}, nil
}

// --- Commit-Push-PR ---

func (d *Dispatcher) handleCommitPushPR(remainder string) (*Result, error) {
	message := remainder
	if message == "" {
		message = "Update files"
	}

	// Stage all changes
	if _, err := d.runtime.RunGitCommand("add", "-A"); err != nil {
		return &Result{Action: "continue", Message: "Failed to stage files: " + err.Error()}, nil
	}

	// Commit
	output, err := d.runtime.RunGitCommand("commit", "-m", message)
	if err != nil {
		return &Result{Action: "continue", Message: "Commit failed: " + err.Error()}, nil
	}

	// Push
	pushOutput, err := d.runtime.RunGitCommand("push")
	if err != nil {
		return &Result{Action: "continue", Message: "Committed but push failed: " + err.Error()}, nil
	}

	// Create PR via gh CLI
	prOutput, err := exec.Command("gh", "pr", "create", "--fill").CombinedOutput()
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Committed and pushed.\nCommit: %s\nPush: %s\nPR creation failed (install gh CLI): %v",
			strings.TrimSpace(output), strings.TrimSpace(pushOutput), err)}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Committed, pushed, and PR created!\nCommit: %s\nPR: %s",
		strings.TrimSpace(output), strings.TrimSpace(string(prOutput)))}, nil
}

// --- PR operations ---

func (d *Dispatcher) handlePR(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		output, err := exec.Command("gh", "pr", "list", "--limit", "10").CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list PRs. Make sure gh CLI is installed and you're in a GitHub repo."}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	}

	switch parts[0] {
	case "create":
		args := []string{"gh", "pr", "create"}
		if len(parts) > 1 && parts[1] == "--fill" {
			args = append(args, "--fill")
		} else if len(parts) >= 3 {
			args = append(args, "--title", parts[1], "--body", strings.Join(parts[2:], " "))
		} else {
			args = append(args, "--fill")
		}
		output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to create PR: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: "PR created: " + strings.TrimSpace(string(output))}, nil
	case "list":
		limit := "10"
		if len(parts) > 1 {
			limit = parts[1]
		}
		output, err := exec.Command("gh", "pr", "list", "--limit", limit).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list PRs: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	case "view":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /pr view <number>"}, nil
		}
		output, err := exec.Command("gh", "pr", "view", parts[1]).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to view PR: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	case "checkout":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /pr checkout <number>"}, nil
		}
		output, err := exec.Command("gh", "pr", "checkout", parts[1]).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to checkout PR: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /pr [create|list|view|checkout] [args]"}, nil
	}
}

// --- Issue operations ---

func (d *Dispatcher) handleIssue(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		output, err := exec.Command("gh", "issue", "list", "--limit", "10").CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list issues. Make sure gh CLI is installed and you're in a GitHub repo."}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	}

	switch parts[0] {
	case "create":
		args := []string{"gh", "issue", "create"}
		if len(parts) >= 3 {
			args = append(args, "--title", parts[1], "--body", strings.Join(parts[2:], " "))
		} else if len(parts) >= 2 {
			args = append(args, "--title", parts[1], "--body", "")
		} else {
			args = append(args, "--web")
		}
		output, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to create issue: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: "Issue created: " + strings.TrimSpace(string(output))}, nil
	case "list":
		limit := "10"
		if len(parts) > 1 {
			limit = parts[1]
		}
		output, err := exec.Command("gh", "issue", "list", "--limit", limit).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list issues: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	case "view":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /issue view <number>"}, nil
		}
		output, err := exec.Command("gh", "issue", "view", parts[1]).CombinedOutput()
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to view issue: " + string(output)}, nil
		}
		return &Result{Action: "continue", Message: string(output)}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /issue [create|list|view] [args]"}, nil
	}
}

// --- Ultraplan ---

func (d *Dispatcher) handleUltraplan(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: "Ultraplan mode: generates a detailed implementation plan.\nUsage: /ultraplan <task description>\nDescribe what you want built and it will produce a comprehensive plan."}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Ultraplan mode activated for: %s\n\nThe next conversation will focus on creating a detailed implementation plan with:\n- Architecture decisions\n- Step-by-step implementation order\n- Dependencies between components\n- Testing strategy\n- Risk assessment", remainder)}, nil
}

// --- Teleport ---

func (d *Dispatcher) handleTeleport(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: "Teleport: Connect to a remote glaw-code session.\nUsage: /teleport <host:port>\nThis connects to a running glaw-code server instance."}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Teleporting to %s...\nNote: Remote sessions require a running glaw-code server. Use 'glaw-code serve' to start one.", remainder)}, nil
}

// --- Debug tool call ---

func (d *Dispatcher) handleDebugToolCall(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: "Debug tool call: trace tool call execution.\nUsage: /debug-tool-call [on|off]\nToggle or check the current state of tool call debugging."}, nil
	}
	switch strings.ToLower(remainder) {
	case "on", "true", "1", "enable":
		return &Result{Action: "continue", Message: "Tool call debugging enabled. Tool calls will show full input/output details."}, nil
	case "off", "false", "0", "disable":
		return &Result{Action: "continue", Message: "Tool call debugging disabled."}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /debug-tool-call [on|off]"}, nil
	}
}

// levenshtein computes the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// --- /btw: Ask a side question during execution ---

func (d *Dispatcher) handleBTW(remainder string) (*Result, error) {
	if remainder == "" {
		return &Result{Action: "continue", Message: "Usage: /btw <question>\nAsk a side question while working on something else."}, nil
	}
	// The /btw command stores the question and returns it as a message
	// The REPL will pick this up and process it as an interrupt question
	return &Result{
		Action:  "continue",
		Message: fmt.Sprintf("BTW noted: %s\n(Your question will be addressed alongside the current task.)", remainder),
	}, nil
}

// --- /tasks: Manage agent tasks ---

func (d *Dispatcher) handleTasks(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		return &Result{Action: "continue", Message: "Task Management:\n  /tasks create <subject> - Create a new task\n  /tasks list - List all tasks\n  /tasks update <id> <status> - Update task status\n  /tasks delete <id> - Delete a task"}, nil
	}

	store := d.taskStore

	switch parts[0] {
	case "create":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /tasks create <subject>"}, nil
		}
		subject := strings.Join(parts[1:], " ")
		t := store.Create(subject)
		return &Result{Action: "continue", Message: fmt.Sprintf("Task #%s created: %s", t.ID, t.Subject)}, nil

	case "list":
		list := store.List()
		if len(list) == 0 {
			return &Result{Action: "continue", Message: "No active tasks. Use /tasks create <subject> to create one."}, nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tasks (%d):\n", len(list)))
		for _, t := range list {
			sb.WriteString(fmt.Sprintf("  #%s [%s] %s", t.ID, t.Status, t.Subject))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf(" - %s", t.Description))
			}
			if len(t.BlockedBy) > 0 {
				sb.WriteString(fmt.Sprintf(" (blocked by: %s)", strings.Join(t.BlockedBy, ", ")))
			}
			sb.WriteString("\n")
		}
		return &Result{Action: "continue", Message: sb.String()}, nil

	case "update":
		if len(parts) < 3 {
			return &Result{Action: "continue", Message: "Usage: /tasks update <id> <status>\nStatuses: pending, in_progress, completed, deleted"}, nil
		}
		t, err := store.Update(parts[1], parts[2], "")
		if err != nil {
			return &Result{Action: "continue", Message: "Error: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Task #%s updated to %s: %s", t.ID, t.Status, t.Subject)}, nil

	case "delete":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /tasks delete <id>"}, nil
		}
		if err := store.Delete(parts[1]); err != nil {
			return &Result{Action: "continue", Message: "Error: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Task #%s deleted.", parts[1])}, nil

	default:
		return &Result{Action: "continue", Message: "Usage: /tasks [create|list|update|delete] [args]"}, nil
	}
}

// --- /lsp: LSP server management ---

func (d *Dispatcher) handleLSP(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		return d.handleLSPStatus()
	}

	switch parts[0] {
	case "status":
		return d.handleLSPStatus()
	case "detect":
		return d.handleLSPDetect()
	case "restart":
		return &Result{Action: "continue", Message: "LSP restart requested. Language servers will reconnect on next tool use."}, nil
	default:
		return &Result{Action: "continue", Message: "Usage: /lsp [status|detect|restart]"}, nil
	}
}

func (d *Dispatcher) handleLSPStatus() (*Result, error) {
	statuses := d.runtime.GetLSPStatus()
	if len(statuses) == 0 {
		return &Result{Action: "continue", Message: "No LSP servers configured.\n\nTo auto-detect language servers, use: /lsp detect\nTo configure manually, create .glaw/lsp.json"}, nil
	}

	var sb strings.Builder
	sb.WriteString("LSP Server Status:\n\n")
	for _, s := range statuses {
		state := "stopped"
		if s.Running {
			state = "running"
		}
		sb.WriteString(fmt.Sprintf("  %-30s %-10s %s\n", s.Name, state, s.Command))
	}
	sb.WriteString("\nLSP tools available: lsp_go_to_definition, lsp_find_references, lsp_hover, lsp_document_symbol, lsp_workspace_symbol, lsp_go_to_implementation, lsp_incoming_calls, lsp_outgoing_calls")

	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleLSPDetect() (*Result, error) {
	return &Result{Action: "continue", Message: "LSP auto-detection scans for known language servers on your PATH.\n\nDetected servers are configured for lazy initialization - they connect on first use.\nTo see current status, use: /lsp status\nTo manually configure, create .glaw/lsp.json with:\n  {\"servers\": [{\"name\": \"gopls\", \"command\": \"gopls\", \"args\": [\"serve\"], \"extensionToLanguage\": {\".go\": \"go\"}}]}"}, nil
}

// --- /revert: Undo file changes ---

func (d *Dispatcher) handleRevert(remainder string) (*Result, error) {
	if remainder == "all" {
		count, err := d.runtime.RevertAll()
		if err != nil {
			return &Result{Action: "continue", Message: "Revert failed: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Reverted all changes (%d file(s) restored).", count)}, nil
	}

	count, err := d.runtime.RevertLastTurn()
	if err != nil {
		return &Result{Action: "continue", Message: "Revert failed: " + err.Error()}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Reverted last turn (%d file(s) restored).\nUse /revert all to revert all changes.", count)}, nil
}
