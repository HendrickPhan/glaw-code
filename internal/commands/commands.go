package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/tasks"
)

// AgentJobStatus holds the status of a background agent job.
type AgentJobStatus struct {
	ID        string     `json:"id"`
	AgentType string     `json:"agent_type"`
	Status    string     `json:"status"`
	Prompt    string     `json:"prompt"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
}

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

// AgentInfo holds descriptive information about an available agent.
type AgentInfo struct {
	Name        string
	Description string
	Source      string // "builtin", "project", "user"
	Tools       []string
	Model       string
	Prompt      string
}

// SubAgentSessionInfo holds metadata about a sub-agent's session for listing.
type SubAgentSessionInfo struct {
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

// AgentsProvider is the interface for resolving and managing agents without
// importing the agent package (which would create an import cycle via runtime).
type AgentsProvider interface {
	ListAgents(workspaceRoot string) ([]AgentInfo, error)
	GetAgent(workspaceRoot, name string) (*AgentInfo, error)
	CreateAgent(workspaceRoot, name, description, scope string, tools []string, model, prompt string) error
	DeleteAgent(workspaceRoot, name, scope string) error
	CallAgent(ctx context.Context, name, prompt string) (string, error)
	// Background agent job management
	CallAgentBackground(ctx context.Context, name, prompt string) (string, error) // returns job ID
	GetAgentJobStatus(jobID string) (*AgentJobStatus, error)
	ListAgentJobs() []*AgentJobStatus
	WaitAgentJob(jobID string) (string, error)
	CancelAgentJob(jobID string) error
}

// Runtime provides the interface commands need.
type Runtime interface {
	GetModel() string
	SetModel(model string)
	GetPermissionMode() string
	SetPermissionMode(mode string)
	IsYoloMode() bool
	ToggleYoloMode() bool
	GetMessageCount() int
	GetSessionID() string
	GetUsageInfo() UsageInfo
	CompactSession() error
	ClearSession()
	LoadSession(sessionID string) error
	NewSession()
	RunGitCommand(args ...string) (string, error)
	GetWorkspaceRoot() string
	GetAllSettings() map[string]interface{}
	SetConfigValue(key, value string) error
	RevertLastTurn() (int, error)
	RevertAll() (int, error)
	// Sub-agent context support
	GetSubAgentSessions() []SubAgentSessionInfo
	ResumeSubAgentSession(agentID string) error
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
	{Name: "session", Summary: "Session management", ArgumentHint: "[list|load|new|delete|resume]", Category: CategorySession},
	{Name: "plugin", Summary: "Plugin management", ArgumentHint: "[install|enable|disable|list]", Category: CategoryCore},
	{Name: "agents", Summary: "Agent management", ArgumentHint: "[list|show|create|call|edit|delete|jobs|status|fg|cancel|logs|wait] [args]", Category: CategoryCore},
	{Name: "skills", Summary: "List available skills", Category: CategoryCore},
	{Name: "btw", Summary: "Ask a side question during execution", ArgumentHint: "<question>", Category: CategoryCore},
	{Name: "tasks", Summary: "Manage agent tasks", ArgumentHint: "[create|list|update|delete] [args]", Category: CategoryCore},
	{Name: "yolo", Summary: "Toggle yolo mode (auto-approve all tool calls)", Category: CategoryCore},
	{Name: "revert", Aliases: []string{"undo"}, Summary: "Revert file changes from the last turn", ArgumentHint: "[all]", Category: CategoryWorkspace},
	{Name: "analyze", Summary: "Analyze project source code and generate summary/graph", ArgumentHint: "[full|summary|graph] [mermaid|dot|json]", Category: CategoryWorkspace},
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
	runtime        Runtime
	taskStore      *tasks.Store
	agentsProvider AgentsProvider
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

// SetAgentsProvider sets the provider used to resolve available agents.
// This must be called before /agents is used if dynamic agent loading is desired.
func (d *Dispatcher) SetAgentsProvider(p AgentsProvider) {
	d.agentsProvider = p
}

// HasRunningAgentJobs returns true if there are any background agent jobs
// that are still running or pending.
func (d *Dispatcher) HasRunningAgentJobs() bool {
	if d.agentsProvider == nil {
		return false
	}
	for _, j := range d.agentsProvider.ListAgentJobs() {
		if j.Status == "running" || j.Status == "pending" {
			return true
		}
	}
	return false
}

// PollCompletedAgentJobs returns job IDs that have completed since the last
// call and marks them as notified so they are not reported again.
func (d *Dispatcher) PollCompletedAgentJobs() []string {
	if d.agentsProvider == nil {
		return nil
	}
	var completed []string
	for _, j := range d.agentsProvider.ListAgentJobs() {
		if j.Status == "completed" || j.Status == "failed" || j.Status == "cancelled" {
			if !d.isJobNotified(j.ID) {
				completed = append(completed, j.ID)
				d.markJobNotified(j.ID)
			}
		}
	}
	return completed
}

// notifiedJobs tracks which completed jobs have already been announced.
var notifiedJobs map[string]bool
var notifiedMu sync.Mutex

func init() {
	notifiedJobs = make(map[string]bool)
}

func (d *Dispatcher) isJobNotified(id string) bool {
	notifiedMu.Lock()
	defer notifiedMu.Unlock()
	return notifiedJobs[id]
}

func (d *Dispatcher) markJobNotified(id string) {
	notifiedMu.Lock()
	defer notifiedMu.Unlock()
	notifiedJobs[id] = true
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
		return d.handleAgents(cmd.Remainder)
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
	case "yolo":
		return d.handleYolo()
	case "revert":
		return d.handleRevert(cmd.Remainder)
	case "analyze":
		return d.handleAnalyze(cmd.Remainder)
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
		sb.WriteString(fmt.Sprintf("  %s:\n", strings.ToUpper(string(cat[:1]))+string(cat[1:])))
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

func (d *Dispatcher) handleYolo() (*Result, error) {
	enabled := d.runtime.ToggleYoloMode()
	if enabled {
		return &Result{Action: "continue", Message: "🔥 YOLO MODE ENABLED 🔥\nAll tool calls will be auto-approved. No confirmation required.\nUse /yolo again to disable."}, nil
	}
	prevMode := d.runtime.GetPermissionMode()
	return &Result{Action: "continue", Message: fmt.Sprintf("Yolo mode disabled. Permissions restored to: %s", prevMode)}, nil
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
		// Show available sessions and sub-agent sessions
		var sb strings.Builder

		// List regular sessions
		sessionsDir := filepath.Join(".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			sb.WriteString("No regular sessions found.")
		} else {
			var sessions []string
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					name := strings.TrimSuffix(e.Name(), ".json")
					sessions = append(sessions, "  "+name)
				}
			}
			if len(sessions) == 0 {
				sb.WriteString("No saved sessions found.")
			} else {
				sb.WriteString("Available sessions:\n")
				sb.WriteString(strings.Join(sessions, "\n"))
			}
		}

		// List sub-agent sessions
		subAgentSessions := d.runtime.GetSubAgentSessions()
		if len(subAgentSessions) > 0 {
			sb.WriteString(fmt.Sprintf("\n\nSub-agent sessions (%d):\n", len(subAgentSessions)))
			for _, sa := range subAgentSessions {
				prompt := sa.Prompt
				if len(prompt) > 60 {
					prompt = prompt[:57] + "..."
				}
				sb.WriteString(fmt.Sprintf("  agent:%s  [%s] %-16s %s\n", sa.AgentID, sa.Status, sa.AgentType, prompt))
			}
		}

		sb.WriteString("\n\nUsage: /resume <session-id> | /resume agent:<agent-id>")
		return &Result{Action: "continue", Message: sb.String()}, nil
	}

	// Check if resuming a sub-agent context
	if strings.HasPrefix(remainder, "agent:") {
		agentID := strings.TrimPrefix(remainder, "agent:")
		if err := d.runtime.ResumeSubAgentSession(agentID); err != nil {
			return &Result{Action: "continue", Message: "Failed to resume sub-agent: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Resumed sub-agent context: %s (%d messages loaded).", agentID, d.runtime.GetMessageCount())}, nil
	}

	// Use the in-REPL session switching
	if err := d.runtime.LoadSession(remainder); err != nil {
		return &Result{Action: "continue", Message: "Failed to resume session: " + err.Error()}, nil
	}
	return &Result{Action: "continue", Message: fmt.Sprintf("Resumed session: %s (%d messages loaded).", d.runtime.GetSessionID(), d.runtime.GetMessageCount())}, nil
}

// --- Session management ---

func (d *Dispatcher) handleSession(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	if len(parts) == 0 {
		// Show current session info plus sub-agent sessions
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Current session: %s\nMessages: %d", d.runtime.GetSessionID(), d.runtime.GetMessageCount()))

		// Show sub-agent sessions if any
		subAgentSessions := d.runtime.GetSubAgentSessions()
		if len(subAgentSessions) > 0 {
			sb.WriteString(fmt.Sprintf("\n\nSub-agent sessions (%d):\n", len(subAgentSessions)))
			for _, sa := range subAgentSessions {
				prompt := sa.Prompt
				if len(prompt) > 40 {
					prompt = prompt[:37] + "..."
				}
				sb.WriteString(fmt.Sprintf("  agent:%s  [%s] %s\n", sa.AgentID, sa.Status, prompt))
			}
			sb.WriteString("\nResume a sub-agent context: /session resume agent:<agent-id>")
		}

		sb.WriteString("\n\nUsage: /session [list|load|new|delete|resume] [session-id|agent:<id>]")
		return &Result{Action: "continue", Message: sb.String()}, nil
	}

	switch parts[0] {
	case "list", "ls":
		sessionsDir := filepath.Join(".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return &Result{Action: "continue", Message: "No sessions directory found. Sessions are saved automatically."}, nil
		}
		var current, other []string
		currentID := d.runtime.GetSessionID()
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
				info, err := e.Info()
				if err != nil {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".json")
				msgCount := ""
				// Try to read message count from session file
				data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
				if err == nil {
					var sess struct {
						Messages []struct{} `json:"messages"`
					}
					if json.Unmarshal(data, &sess) == nil {
						msgCount = fmt.Sprintf("  %d msgs", len(sess.Messages))
					}
				}
				line := fmt.Sprintf("  %s  %s%s", name, info.ModTime().Format("2006-01-02 15:04"), msgCount)
				if name == currentID {
					current = append(current, line+"  ← current")
				} else {
					other = append(other, line)
				}
			}
		}

		var sb strings.Builder

		// Regular sessions
		var sessions []string
		sessions = append(sessions, current...)
		sessions = append(sessions, other...)
		if len(sessions) > 0 {
			sb.WriteString("Sessions:\n")
			sb.WriteString(strings.Join(sessions, "\n"))
		}

		// Sub-agent sessions
		subAgentSessions := d.runtime.GetSubAgentSessions()
		if len(subAgentSessions) > 0 {
			sb.WriteString(fmt.Sprintf("\n\nSub-agent sessions (%d):\n", len(subAgentSessions)))
			for _, sa := range subAgentSessions {
				prompt := sa.Prompt
				if len(prompt) > 50 {
					prompt = prompt[:47] + "..."
				}
				sb.WriteString(fmt.Sprintf("  agent:%s  [%s] %-16s %s\n", sa.AgentID, sa.Status, sa.AgentType, prompt))
			}
		}

		if sb.Len() == 0 {
			return &Result{Action: "continue", Message: "No saved sessions found."}, nil
		}

		sb.WriteString("\n\nSwitch: /session load <id>   New: /session new   Delete: /session delete <id>\nResume sub-agent: /session resume agent:<agent-id>")
		return &Result{Action: "continue", Message: sb.String()}, nil

	case "load", "switch":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /session load <session-id> | /session load agent:<agent-id>\nUse /session list to see available sessions."}, nil
		}
		sessionID := parts[1]

		// Check if loading a sub-agent context
		if strings.HasPrefix(sessionID, "agent:") {
			agentID := strings.TrimPrefix(sessionID, "agent:")
			if err := d.runtime.ResumeSubAgentSession(agentID); err != nil {
				return &Result{Action: "continue", Message: "Failed to resume sub-agent: " + err.Error()}, nil
			}
			return &Result{Action: "continue", Message: fmt.Sprintf("Resumed sub-agent context: %s (%d messages loaded).", agentID, d.runtime.GetMessageCount())}, nil
		}

		if err := d.runtime.LoadSession(sessionID); err != nil {
			return &Result{Action: "continue", Message: "Failed to load session: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Switched to session: %s (%d messages from previous conversation loaded).\nPrevious session saved automatically.", d.runtime.GetSessionID(), d.runtime.GetMessageCount())}, nil

	case "resume":
		// Supports agent:<id> syntax for sub-agent resumption
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /session resume <session-id> | /session resume agent:<agent-id>"}, nil
		}
		target := parts[1]
		if strings.HasPrefix(target, "agent:") {
			agentID := strings.TrimPrefix(target, "agent:")
			if err := d.runtime.ResumeSubAgentSession(agentID); err != nil {
				return &Result{Action: "continue", Message: "Failed to resume sub-agent: " + err.Error()}, nil
			}
			return &Result{Action: "continue", Message: fmt.Sprintf("Resumed sub-agent context: %s (%d messages loaded).", agentID, d.runtime.GetMessageCount())}, nil
		}
		if err := d.runtime.LoadSession(target); err != nil {
			return &Result{Action: "continue", Message: "Failed to resume session: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Resumed session: %s (%d messages loaded).", d.runtime.GetSessionID(), d.runtime.GetMessageCount())}, nil

	case "new":
		oldID := d.runtime.GetSessionID()
		d.runtime.NewSession()
		return &Result{Action: "continue", Message: fmt.Sprintf("New session created: %s\nPrevious session %s saved automatically.", d.runtime.GetSessionID(), oldID)}, nil

	case "delete", "rm":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /session delete <session-id>"}, nil
		}
		// Prevent deleting the current session
		if parts[1] == d.runtime.GetSessionID() {
			return &Result{Action: "continue", Message: "Cannot delete the current session. Use /session new first to switch to a new session."}, nil
		}
		path := filepath.Join(".glaw", "sessions", parts[1]+".json")
		if err := os.Remove(path); err != nil {
			return &Result{Action: "continue", Message: "Failed to delete session: " + err.Error()}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Session %s deleted.", parts[1])}, nil

	default:
		return &Result{Action: "continue", Message: "Usage: /session [list|load|new|delete|resume] [session-id|agent:<id>]\n\n  list              - Show all saved sessions\n  load <id>         - Switch to a different session\n  new               - Create a fresh session\n  delete <id>       - Delete a saved session\n  resume agent:<id> - Resume a sub-agent's context"}, nil
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

func (d *Dispatcher) handleAgents(remainder string) (*Result, error) {
	parts := parseQuotedFields(remainder)
	if len(parts) == 0 {
		return d.handleAgentsList()
	}

	switch parts[0] {
	case "list", "ls":
		return d.handleAgentsList()
	case "show", "info", "get":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents show <name>\nShow detailed information about an agent."}, nil
		}
		return d.handleAgentsShow(parts[1])
	case "create", "new":
		return d.handleAgentsCreate(parts[1:])
	case "call", "run":
		return d.handleAgentsCall(parts[1:])
	case "edit", "update":
		return d.handleAgentsEdit(parts[1:])
	case "delete", "rm", "remove":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents delete <name> [--project|--user]\nDelete a custom agent."}, nil
		}
		return d.handleAgentsDelete(parts[1:])
	case "jobs":
		return d.handleAgentsJobs()
	case "status":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents status <job-id>"}, nil
		}
		return d.handleAgentsJobStatus(parts[1])
	case "fg":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents fg <job-id>"}, nil
		}
		return d.handleAgentsFg(parts[1])
	case "cancel":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents cancel <job-id>"}, nil
		}
		return d.handleAgentsCancel(parts[1])
	case "logs":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents logs <job-id>"}, nil
		}
		return d.handleAgentsLogs(parts[1])
	case "wait":
		if len(parts) < 2 {
			return &Result{Action: "continue", Message: "Usage: /agents wait <job-id>"}, nil
		}
		return d.handleAgentsWait(parts[1])
	default:
		return &Result{Action: "continue", Message: "Agent Management:\n\n  /agents                       List all available agents\n  /agents list                  List all available agents\n  /agents show <name>           Show detailed agent information\n  /agents create <name>         Create a new custom agent\n  /agents call <name> <prompt>  Run an agent in background\n  /agents call <name> <prompt> --wait  Run agent and block until done\n  /agents edit <name>           Edit an existing custom agent\n  /agents delete <name>         Delete a custom agent\n\n  Background Job Management:\n  /agents jobs                  List all background agent jobs\n  /agents status <job-id>       Show detailed job status\n  /agents fg <job-id>           Bring job to foreground (wait for result)\n  /agents cancel <job-id>       Cancel a running job\n  /agents logs <job-id>         Show output of a completed job\n  /agents wait <job-id>         Wait for job to complete\n"}, nil
	}
}

func (d *Dispatcher) handleAgentsList() (*Result, error) {
	var sb strings.Builder
	sb.WriteString("Available Agents:\n\n")

	workspaceRoot := d.runtime.GetWorkspaceRoot()

	if d.agentsProvider != nil {
		agents, err := d.agentsProvider.ListAgents(workspaceRoot)
		if err != nil {
			return &Result{Action: "continue", Message: "Failed to list agents: " + err.Error()}, nil
		}

		// Group by source
		builtinCount := 0
		var custom []AgentInfo
		for _, a := range agents {
			if a.Source == "builtin" {
				if builtinCount == 0 {
					sb.WriteString("  Built-in Agents:\n")
				}
				sb.WriteString(fmt.Sprintf("    %-20s %s\n", a.Name, a.Description))
				builtinCount++
			} else {
				custom = append(custom, a)
			}
		}

		if len(custom) > 0 {
			sb.WriteString("\n  Custom Agents:\n")
			for _, a := range custom {
				sourceTag := ""
				if a.Source == "user" {
					sourceTag = " (global)"
				} else if a.Source == "project" {
					sourceTag = " (project)"
				}
				sb.WriteString(fmt.Sprintf("    %-20s %s%s\n", a.Name, a.Description, sourceTag))
			}
		}

		sb.WriteString(fmt.Sprintf("\n  %d built-in agent(s)", builtinCount))
		if len(custom) > 0 {
			sb.WriteString(fmt.Sprintf(", %d custom agent(s) from .glaw/agents/ and ~/.glaw/agents/", len(custom)))
		}
	} else {
		// Fallback: no provider registered
		sb.WriteString("  No agent provider configured.\n")
	}

	sb.WriteString("\n")
	sb.WriteString("\n  Use /agents show <name> for details, /agents call <name> <prompt> to run.")
	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleAgentsShow(name string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	workspaceRoot := d.runtime.GetWorkspaceRoot()
	agent, err := d.agentsProvider.GetAgent(workspaceRoot, name)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q not found: %s", name, err.Error())}, nil
	}
	if agent == nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q not found.", name)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent: %s\n", agent.Name))
	sb.WriteString(fmt.Sprintf("Source: %s\n", agent.Source))
	sb.WriteString(fmt.Sprintf("Description: %s\n", agent.Description))

	if len(agent.Tools) > 0 {
		sb.WriteString(fmt.Sprintf("Tools: %s\n", strings.Join(agent.Tools, ", ")))
	} else {
		sb.WriteString("Tools: all (inherits from parent)\n")
	}

	if agent.Model != "" {
		sb.WriteString(fmt.Sprintf("Model: %s\n", agent.Model))
	} else {
		sb.WriteString("Model: inherit\n")
	}

	if agent.Prompt != "" {
		prompt := agent.Prompt
		if len(prompt) > 500 {
			prompt = prompt[:497] + "..."
		}
		sb.WriteString(fmt.Sprintf("\nSystem Prompt:\n%s\n", prompt))
	}

	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleAgentsCreate(parts []string) (*Result, error) {
	// /agents create <name> --desc "description" [--tools tool1,tool2] [--model sonnet] [--scope project|user] [--prompt "system prompt"]
	if len(parts) < 1 {
		return &Result{Action: "continue", Message: `Usage: /agents create <name> [options]

Options:
  --desc <description>    Agent description (required)
  --tools <tool1,tool2>   Comma-separated list of tools (default: all)
  --model <model>         Model to use: sonnet, opus, haiku, inherit (default: inherit)
  --scope <scope>         Where to create: project or user (default: project)
  --prompt <prompt>       System prompt for the agent

Examples:
  /agents create my-reviewer --desc "Code review specialist" --tools read_file,grep_search
  /agents create security-agent --desc "Security auditor" --tools read_file,grep_search,bash --model sonnet --scope user
`}, nil
	}

	name := parts[0]
	if name == "" {
		return &Result{Action: "continue", Message: "Agent name is required."}, nil
	}

	// Parse flags
	var desc, toolsStr, model, scope, prompt string
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "--desc", "-d":
			i++
			if i < len(parts) {
				desc = parts[i]
			}
		case "--tools", "-t":
			i++
			if i < len(parts) {
				toolsStr = parts[i]
			}
		case "--model", "-m":
			i++
			if i < len(parts) {
				model = parts[i]
			}
		case "--scope", "-s":
			i++
			if i < len(parts) {
				scope = parts[i]
			}
		case "--prompt", "-p":
			i++
			if i < len(parts) {
				prompt = parts[i]
			}
		}
	}

	if desc == "" {
		desc = "Custom agent: " + name
	}
	if scope == "" {
		scope = "project"
	}

	var tools []string
	if toolsStr != "" {
		tools = strings.Split(toolsStr, ",")
		for i := range tools {
			tools[i] = strings.TrimSpace(tools[i])
		}
	}

	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	workspaceRoot := d.runtime.GetWorkspaceRoot()
	if err := d.agentsProvider.CreateAgent(workspaceRoot, name, desc, scope, tools, model, prompt); err != nil {
		return &Result{Action: "continue", Message: "Failed to create agent: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q created successfully.\n  Description: %s\n  Scope: %s\n  Tools: %s\n  Model: %s\n\nUse /agents call %s <prompt> to run it.", name, desc, scope, toolsStrOrDefault(toolsStr), modelOrDefault(model), name)}, nil
}

func (d *Dispatcher) handleAgentsCall(parts []string) (*Result, error) {
	if len(parts) < 2 {
		return &Result{Action: "continue", Message: "Usage: /agents call <name> <prompt> [--wait]\nRun an agent with a task prompt.\nBy default runs in background. Use --wait to block until done.\n\nExample:\n  /agents call Explore \"Search for all error handling patterns\"\n  /agents call Explore \"Search for patterns\" --wait"}, nil
	}

	name := parts[0]
	// After parseQuotedFields, quoted content is already merged into single tokens.
	// If there are multiple remaining parts, join them (handles unquoted multi-word prompts).
	remaining := parts[1:]

	// Check for --wait flag
	wait := false
	var promptParts []string
	for _, p := range remaining {
		if p == "--wait" || p == "-w" {
			wait = true
		} else {
			promptParts = append(promptParts, p)
		}
	}

	if len(promptParts) == 0 {
		return &Result{Action: "continue", Message: "Usage: /agents call <name> <prompt> [--wait]"}, nil
	}

	taskPrompt := strings.Join(promptParts, " ")

	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	ctx := context.Background()

	if wait {
		// Blocking mode: old behavior
		output, err := d.agentsProvider.CallAgent(ctx, name, taskPrompt)
		if err != nil {
			return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q failed: %s", name, err.Error())}, nil
		}
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q result:\n\n%s", name, output)}, nil
	}

	// Non-blocking mode: spawn in background
	jobID, err := d.agentsProvider.CallAgentBackground(ctx, name, taskPrompt)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Failed to spawn agent %q: %s", name, err.Error())}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q spawned in background.\n  Job ID: %s\n\nUse /agents jobs to list jobs, /agents status %s for details, /agents fg %s to bring to foreground.", name, jobID, jobID, jobID)}, nil
}

func (d *Dispatcher) handleAgentsEdit(parts []string) (*Result, error) {
	if len(parts) < 1 {
		return &Result{Action: "continue", Message: `Usage: /agents edit <name> [options]

Options:
  --desc <description>    Update agent description
  --tools <tool1,tool2>   Update tool list
  --model <model>         Update model
  --prompt <prompt>       Update system prompt

Examples:
  /agents edit my-agent --desc "New description"
  /agents edit my-agent --tools read_file,bash --model sonnet
`}, nil
	}

	name := parts[0]

	// Parse flags
	var desc, toolsStr, model, prompt string
	hasChanges := false
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "--desc", "-d":
			i++
			if i < len(parts) {
				desc = parts[i]
				hasChanges = true
			}
		case "--tools", "-t":
			i++
			if i < len(parts) {
				toolsStr = parts[i]
				hasChanges = true
			}
		case "--model", "-m":
			i++
			if i < len(parts) {
				model = parts[i]
				hasChanges = true
			}
		case "--prompt", "-p":
			i++
			if i < len(parts) {
				prompt = parts[i]
				hasChanges = true
			}
		}
	}

	if !hasChanges {
		return &Result{Action: "continue", Message: "No changes specified. Use --desc, --tools, --model, or --prompt flags."}, nil
	}

	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	workspaceRoot := d.runtime.GetWorkspaceRoot()

	// Get existing agent
	existing, err := d.agentsProvider.GetAgent(workspaceRoot, name)
	if err != nil || existing == nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q not found. Only existing custom agents can be edited.", name)}, nil
	}
	if existing.Source == "builtin" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q is a built-in agent and cannot be edited. Create a custom agent with the same name in .glaw/agents/ to override it.", name)}, nil
	}

	// Build updated config
	if desc == "" {
		desc = existing.Description
	}
	var tools []string
	if toolsStr != "" {
		tools = strings.Split(toolsStr, ",")
		for i := range tools {
			tools[i] = strings.TrimSpace(tools[i])
		}
	} else if len(existing.Tools) > 0 {
		tools = existing.Tools
	}
	if model == "" {
		model = existing.Model
	}
	if prompt == "" {
		prompt = existing.Prompt
	}

	scope := existing.Source

	// Delete and recreate
	if err := d.agentsProvider.DeleteAgent(workspaceRoot, name, scope); err != nil {
		return &Result{Action: "continue", Message: "Failed to update agent: " + err.Error()}, nil
	}
	if err := d.agentsProvider.CreateAgent(workspaceRoot, name, desc, scope, tools, model, prompt); err != nil {
		return &Result{Action: "continue", Message: "Failed to update agent: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q updated successfully.", name)}, nil
}

func (d *Dispatcher) handleAgentsDelete(parts []string) (*Result, error) {
	name := parts[0]
	scope := "project"

	// Parse optional scope flag
	for i := 1; i < len(parts); i++ {
		switch parts[i] {
		case "--project":
			scope = "project"
		case "--user", "--global":
			scope = "user"
		}
	}

	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	workspaceRoot := d.runtime.GetWorkspaceRoot()

	// Check if it's a built-in agent
	existing, err := d.agentsProvider.GetAgent(workspaceRoot, name)
	if err == nil && existing != nil && existing.Source == "builtin" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q is a built-in agent and cannot be deleted.", name)}, nil
	}

	if err := d.agentsProvider.DeleteAgent(workspaceRoot, name, scope); err != nil {
		return &Result{Action: "continue", Message: "Failed to delete agent: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Agent %q deleted.", name)}, nil
}

// --- Agent Job Management ---

func (d *Dispatcher) handleAgentsJobs() (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	jobs := d.agentsProvider.ListAgentJobs()
	if len(jobs) == 0 {
		return &Result{Action: "continue", Message: "No agent jobs. Use /agents call <name> <prompt> to spawn one."}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Agent Jobs (%d):\n\n", len(jobs)))
	for _, j := range jobs {
		prompt := j.Prompt
		if len(prompt) > 50 {
			prompt = prompt[:47] + "..."
		}
		elapsed := ""
		if j.EndTime != nil {
			elapsed = fmt.Sprintf(" (%s)", j.EndTime.Sub(j.StartTime).Round(time.Millisecond))
		} else {
			elapsed = fmt.Sprintf(" (running %s)", time.Since(j.StartTime).Round(time.Second))
		}
		sb.WriteString(fmt.Sprintf("  %s  [%s] %-16s %s%s\n", j.ID, j.Status, j.AgentType, prompt, elapsed))
	}

	sb.WriteString("\nUse /agents status <id> for details, /agents fg <id> to bring to foreground.")
	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleAgentsJobStatus(jobID string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	job, err := d.agentsProvider.GetAgentJobStatus(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q not found.", jobID)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Job ID:     %s\n", job.ID))
	sb.WriteString(fmt.Sprintf("Agent:      %s\n", job.AgentType))
	sb.WriteString(fmt.Sprintf("Status:     %s\n", job.Status))
	sb.WriteString(fmt.Sprintf("Prompt:     %s\n", job.Prompt))
	sb.WriteString(fmt.Sprintf("Started:    %s\n", job.StartTime.Format(time.RFC3339)))
	if job.EndTime != nil {
		sb.WriteString(fmt.Sprintf("Finished:   %s\n", job.EndTime.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("Duration:   %s\n", job.EndTime.Sub(job.StartTime).Round(time.Millisecond)))
	} else {
		sb.WriteString(fmt.Sprintf("Duration:   %s (still running)\n", time.Since(job.StartTime).Round(time.Second)))
	}

	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleAgentsFg(jobID string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	// Check the job exists and is not already terminal
	job, err := d.agentsProvider.GetAgentJobStatus(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q not found.", jobID)}, nil
	}

	if job.Status == "completed" || job.Status == "failed" {
		// Already done — just show logs
		return d.handleAgentsLogs(jobID)
	}

	if job.Status == "cancelled" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q was cancelled.", jobID)}, nil
	}

	// Wait for it
	output, err := d.agentsProvider.WaitAgentJob(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q failed: %s", jobID, err.Error())}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Job %s completed.\n\n%s", jobID, output)}, nil
}

func (d *Dispatcher) handleAgentsCancel(jobID string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	if err := d.agentsProvider.CancelAgentJob(jobID); err != nil {
		return &Result{Action: "continue", Message: "Failed to cancel job: " + err.Error()}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Job %s cancelled.", jobID)}, nil
}

func (d *Dispatcher) handleAgentsLogs(jobID string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	job, err := d.agentsProvider.GetAgentJobStatus(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q not found.", jobID)}, nil
	}

	if job.Status == "running" || job.Status == "pending" {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %s is still %s. Use /agents fg %s to wait for it.", jobID, job.Status, jobID)}, nil
	}

	// For completed/failed jobs, wait to get the result (non-blocking for terminal states)
	output, err := d.agentsProvider.WaitAgentJob(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %s error: %s", jobID, err.Error())}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Job %s [%s]:\n\n", jobID, job.Status))
	sb.WriteString(output)
	return &Result{Action: "continue", Message: sb.String()}, nil
}

func (d *Dispatcher) handleAgentsWait(jobID string) (*Result, error) {
	if d.agentsProvider == nil {
		return &Result{Action: "continue", Message: "No agent provider configured."}, nil
	}

	output, err := d.agentsProvider.WaitAgentJob(jobID)
	if err != nil {
		return &Result{Action: "continue", Message: fmt.Sprintf("Job %q failed: %s", jobID, err.Error())}, nil
	}

	return &Result{Action: "continue", Message: fmt.Sprintf("Job %s completed.\n\n%s", jobID, output)}, nil
}

// parseQuotedFields splits a string into fields like a shell would,
// respecting double-quoted and single-quoted strings.
// e.g. `foo --desc "hello world" --bar baz` → ["foo", "--desc", "hello world", "--bar", "baz"]
func parseQuotedFields(s string) []string {
	var fields []string
	var buf strings.Builder
	inDouble := false
	inSingle := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if inDouble {
			if ch == '"' {
				inDouble = false
			} else {
				buf.WriteByte(ch)
			}
			continue
		}
		if inSingle {
			if ch == '\'' {
				inSingle = false
			} else {
				buf.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '"':
			inDouble = true
		case '\'':
			inSingle = true
		case ' ', '\t':
			if buf.Len() > 0 {
				fields = append(fields, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteByte(ch)
		}
	}

	if buf.Len() > 0 {
		fields = append(fields, buf.String())
	}

	return fields
}

// toolsStrOrDefault returns the tools string or "all"
func toolsStrOrDefault(s string) string {
	if s == "" {
		return "all"
	}
	return s
}

// modelOrDefault returns the model string or "inherit"
func modelOrDefault(s string) string {
	if s == "" {
		return "inherit"
	}
	return s
}

// --- Skills ---

func (d *Dispatcher) handleSkills() (*Result, error) {
	type skillInfo struct {
		name        string
		description string
		source      string // "global" or "project"
	}

	var skills []skillInfo

	// Load skills from project skills directory (.glaw/skills)
	projectDir := filepath.Join(".glaw", "skills")
	if entries, err := os.ReadDir(projectDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			description := ""
			// Read first line as description
			if data, err := os.ReadFile(filepath.Join(projectDir, e.Name())); err == nil {
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				if len(lines) > 0 {
					// Strip leading # if present
					desc := strings.TrimSpace(lines[0])
					desc = strings.TrimPrefix(desc, "#")
					desc = strings.TrimSpace(desc)
					if desc != "" {
						description = desc
					} else if len(lines) > 1 {
						// Try second line
						desc = strings.TrimSpace(lines[1])
						desc = strings.TrimPrefix(desc, "#")
						description = strings.TrimSpace(desc)
					}
				}
			}
			if description == "" {
				description = "(no description)"
			}
			skills = append(skills, skillInfo{name: name, description: description, source: "project"})
		}
	}

	// Load skills from global skills directory (~/.glaw/skills)
	home, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(home, ".glaw", "skills")
		if entries, err := os.ReadDir(globalDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".md")
				// Skip if already loaded from project
				found := false
				for _, s := range skills {
					if s.name == name {
						found = true
						break
					}
				}
				if found {
					continue
				}

				description := ""
				if data, err := os.ReadFile(filepath.Join(globalDir, e.Name())); err == nil {
					lines := strings.Split(strings.TrimSpace(string(data)), "\n")
					if len(lines) > 0 {
						desc := strings.TrimSpace(lines[0])
						desc = strings.TrimPrefix(desc, "#")
						desc = strings.TrimSpace(desc)
						if desc != "" {
							description = desc
						} else if len(lines) > 1 {
							desc = strings.TrimSpace(lines[1])
							desc = strings.TrimPrefix(desc, "#")
							description = strings.TrimSpace(desc)
						}
					}
				}
				if description == "" {
					description = "(no description)"
				}
				skills = append(skills, skillInfo{name: name, description: description, source: "global"})
			}
		}
	}

	if len(skills) == 0 {
		return &Result{Action: "continue", Message: "No skills found.\n\nSkills are loaded from:\n  ~/.glaw/skills/*.md (global)\n  .glaw/skills/*.md (project)\n\nCreate a skill by adding a markdown file in either directory.\nThe first line (or # heading) is used as the description."}, nil
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n\n")
	for _, s := range skills {
		sourceTag := ""
		if s.source == "global" {
			sourceTag = " (global)"
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s%s\n", s.name, s.description, sourceTag))
	}
	sb.WriteString(fmt.Sprintf("\n  %d skill(s) loaded from .glaw/skills/ and ~/.glaw/skills/", len(skills)))
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

// --- /analyze: Analyze project source code ---

func (d *Dispatcher) handleAnalyze(remainder string) (*Result, error) {
	parts := strings.Fields(remainder)
	mode := "full"
	format := "text"

	for _, part := range parts {
		switch part {
		case "full", "summary", "graph":
			mode = part
		case "mermaid", "mmd":
			format = "mermaid"
		case "dot", "graphviz":
			format = "dot"
		case "json":
			format = "json"
		case "text":
			format = "text"
		}
	}

	workspaceRoot := d.runtime.GetWorkspaceRoot()
	if workspaceRoot == "" {
		return &Result{Action: "continue", Message: "No workspace root set. Please run from a project directory."}, nil
	}

	return &Result{
		Action:  "continue",
		Message: "Use the analyze tool (mode: " + mode + ", format: " + format + ") for project analysis.\nYou can also ask the AI: \"analyze this project\" and it will use the analyze tool.",
	}, nil
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
