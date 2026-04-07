package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/commands"
	"github.com/hieu-glaw/glaw-code/internal/config"
	"github.com/hieu-glaw/glaw-code/internal/lsp"
)

// spinnerFrames are the animation frames for the loading spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner displays an animated loading indicator in the terminal.
type spinner struct {
	label   string
	frame   int
	stop    chan struct{}
	stopped chan struct{}
	mu      sync.Mutex
	done    bool
}

func newSpinner(label string) *spinner {
	s := &spinner{
		label:   label,
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *spinner) Update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *spinner) run() {
	defer close(s.stopped)
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			fmt.Print("\r\033[K")
			return
		case <-ticker.C:
			s.mu.Lock()
			label := s.label
			frame := spinnerFrames[s.frame%len(spinnerFrames)]
			s.frame++
			s.mu.Unlock()
			fmt.Printf("\r\033[36m%s\033[0m %s", frame, label)
		}
	}
}

func (s *spinner) Stop() {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.mu.Unlock()
	close(s.stop)
	<-s.stopped
}

// Hide temporarily hides the spinner (clears the line) without stopping it.
// Call Show to restore it.
func (s *spinner) Hide() {
	fmt.Print("\r\033[K")
}

// Show restores the spinner after a Hide call.
func (s *spinner) Show() {
	s.mu.Lock()
	label := s.label
	frame := spinnerFrames[s.frame%len(spinnerFrames)]
	s.frame++
	s.mu.Unlock()
	fmt.Printf("\r\033[36m%s\033[0m %s", frame, label)
}

// ANSI codes for terminal output
const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiCyan    = "\033[36m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiRed     = "\033[31m"
	ansiItalic  = "\033[3m"
)

// toolDisplayInfo extracts a short description from tool input JSON.
func toolDisplayInfo(name string, input json.RawMessage) string {
	switch name {
	case "bash":
		var args struct{ Command string `json:"command"` }
		if json.Unmarshal(input, &args) == nil && args.Command != "" {
			if len(args.Command) > 60 {
				return args.Command[:57] + "..."
			}
			return args.Command
		}
	case "write_file", "edit_file", "read_file":
		var args struct{ Path string `json:"path"` }
		if json.Unmarshal(input, &args) == nil {
			return args.Path
		}
	case "glob_search":
		var args struct{ Pattern string `json:"pattern"` }
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	case "grep_search":
		var args struct{ Pattern string `json:"pattern"` }
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	}
	return ""
}

// renderToolHeader renders the one-line header shown when a tool starts.
func renderToolHeader(name string, input json.RawMessage) string {
	info := toolDisplayInfo(name, input)
	icon := "⚙"
	switch name {
	case "bash":
		icon = "$"
	case "write_file", "edit_file":
		icon = "✎"
	case "read_file", "glob_search", "grep_search":
		icon = "📄"
	}
	if info != "" {
		return fmt.Sprintf("%s%s%s %s%-12s%s %s%s%s", ansiYellow, icon, ansiReset, ansiBold, name, ansiReset, ansiDim, info, ansiReset)
	}
	return fmt.Sprintf("%s%s%s %s%s%s", ansiYellow, icon, ansiReset, ansiBold, name, ansiReset)
}

// renderToolDone renders the completion status after a tool finishes.
func renderToolDone(name string, output string, isError bool, elapsed time.Duration) string {
	ms := elapsed.Seconds() * 1000
	if isError {
		display := output
		if len(display) > 100 {
			display = display[:97] + "..."
		}
		return fmt.Sprintf("%s  ✗ %s%s %s(%.0fms)%s %s%s%s", ansiRed, name, ansiReset, ansiDim, ms, ansiReset, ansiRed, display, ansiReset)
	}
	display := output
	if len(display) > 80 {
		display = display[:77] + "..."
	}
	return fmt.Sprintf("%s  ✓ %s%s %s(%.0fms)%s %s", ansiGreen, name, ansiReset, ansiDim, ms, ansiReset, display)
}

// ConversationMessage represents a single message in a conversation.
type ConversationMessage struct {
	Role   string             `json:"role"`
	Blocks []api.ContentBlock `json:"blocks"`
	Usage *api.Usage          `json:"usage,omitempty"`
}

// Session represents a conversation session.
type Session struct {
	Version  int                  `json:"version"`
	Messages []ConversationMessage `json:"messages"`
	ID       string                `json:"id"`
	mu       sync.RWMutex
}

// NewSession creates a new empty session.
func NewSession() *Session {
	return &Session{
		Version:  1,
		Messages: []ConversationMessage{},
		ID:       fmt.Sprintf("sess_%d", time.Now().UnixMilli()),
	}
}

// AddUserMessage appends a user message with content blocks.
func (s *Session) AddUserMessage(blocks []api.ContentBlock) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleUser),
		Blocks: blocks,
	})
}

// AddUserMessageFromText appends a user message from a text string.
func (s *Session) AddUserMessageFromText(text string) {
	s.AddUserMessage([]api.ContentBlock{api.NewTextBlock(text)})
}

// AddUserMessagesFromText appends a user message from a text string (alias).
func (s *Session) AddUserMessagesFromText(text string) {
	s.AddUserMessageFromText(text)
}

// AddAssistantMessage appends an assistant message.
func (s *Session) AddAssistantMessage(blocks []api.ContentBlock, usage *api.Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleAssistant),
		Blocks: blocks,
		Usage:   usage,
	})
}

// AddToolResult adds a tool result as a new user message.
// The Anthropic Messages API requires tool_result blocks to be in user
// messages, not in assistant messages.
func (s *Session) AddToolResult(toolUseID, content string, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add as a user message containing a tool_result block.
	// This matches the Anthropic API convention where tool results
	// are sent as user messages with role "user".
	s.Messages = append(s.Messages, ConversationMessage{
		Role:   string(api.RoleUser),
		Blocks: []api.ContentBlock{api.NewToolResultBlock(toolUseID, content, isError)},
	})
}

// MessageCount returns the number of messages.
func (s *Session) MessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Messages)
}

// AsAPIMessages converts session messages to API format.
// It ensures tool_result blocks are in user messages (per Anthropic API spec)
// and sanitizes content blocks to prevent API errors.
// The ContentBlock.MarshalJSON method handles the primary defense against
// nil/empty required fields, but this method provides additional cleanup
// for data loaded from persisted sessions.
func (s *Session) AsAPIMessages() []api.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var msgs []api.Message
	for _, m := range s.Messages {
		if len(m.Blocks) == 0 {
			continue
		}

		cleanBlocks := make([]api.ContentBlock, 0, len(m.Blocks))
		for _, b := range m.Blocks {
			switch b.Type {
			case api.ContentToolUse:
				// Ensure tool_use blocks have non-nil Input
				if b.Input == nil {
					b.Input = json.RawMessage(`{}`)
				}
			case api.ContentToolResult:
				// Ensure tool_result blocks have a non-empty ToolUseID.
				// Empty content is handled by MarshalJSON but we guard
				// against missing ToolUseID here.
				if b.ToolUseID == "" {
					continue
				}
			}
			cleanBlocks = append(cleanBlocks, b)
		}

		if len(cleanBlocks) == 0 {
			continue
		}

		msgs = append(msgs, api.Message{
			Role:    api.MessageRole(m.Role),
			Content: cleanBlocks,
		})
	}
	return msgs
}

// SaveSession persists a session to disk.
func SaveSession(session *Session, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating session dir: %w", err)
	}

	path := filepath.Join(dir, session.ID+".json")
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling session: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("writing session file: %w", err)
	}

	return path, nil
}

// LoadSession reads a session from disk.
func LoadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// PermissionMode defines the permission level for tool execution.
type PermissionMode string

const (
	PermReadOnly         PermissionMode = "read_only"
	PermWorkspaceWrite   PermissionMode = "workspace_write"
	PermDangerFullAccess PermissionMode = "danger_full_access"
	PermPrompt           PermissionMode = "prompt"
	PermAllow            PermissionMode = "allow"
	PermYolo             PermissionMode = "yolo"
)

// Permission represents a specific permission type.
type Permission string

const (
	PermReadFile       Permission = "read_file"
	PermWriteFile      Permission = "write_file"
	PermEditFile       Permission = "edit_file"
	PermExecuteCommand Permission = "execute_command"
	PermNetwork        Permission = "network"
)

// PermissionManager handles permission checking.
type PermissionManager struct {
	Mode          PermissionMode
	WorkspaceRoot string
	Allowed       map[Permission]bool
	PreviousMode  PermissionMode // stores the mode before yolo was enabled
}

// NewPermissionManager creates a new permission manager.
func NewPermissionManager(mode PermissionMode, workspaceRoot string) *PermissionManager {
	return &PermissionManager{
		Mode:          mode,
		WorkspaceRoot: workspaceRoot,
		Allowed:       make(map[Permission]bool),
	}
}

// IsYolo returns true if yolo mode is active.
func (m *PermissionManager) IsYolo() bool {
	return m.Mode == PermYolo
}

// ToggleYolo enables or disables yolo mode.
// When enabling, it saves the current mode so it can be restored.
// When disabling, it restores the previous mode.
func (m *PermissionManager) ToggleYolo() bool {
	if m.Mode == PermYolo {
		// Disable yolo: restore previous mode
		if m.PreviousMode != "" {
			m.Mode = m.PreviousMode
		} else {
			m.Mode = PermWorkspaceWrite
		}
		m.PreviousMode = ""
		return false
	}
	// Enable yolo: save current mode
	m.PreviousMode = m.Mode
	m.Mode = PermYolo
	return true
}

// Check evaluates whether a permission is granted.
func (m *PermissionManager) Check(perm Permission) bool {
	switch m.Mode {
	case PermAllow, PermDangerFullAccess, PermYolo:
		return true
	case PermReadOnly:
		return perm == PermReadFile || perm == PermNetwork
	case PermWorkspaceWrite:
		return perm != PermExecuteCommand
	case PermPrompt:
		return true // In prompt mode, always allow (UI handles prompting)
	default:
		return false
	}
}

// CheckTool evaluates whether a tool can be used based on required permission.
func (m *PermissionManager) CheckTool(toolName string, requiredPerm PermissionMode) bool {
	switch m.Mode {
	case PermAllow, PermDangerFullAccess, PermYolo:
		return true
	case PermReadOnly:
		return requiredPerm == PermReadOnly || requiredPerm == ""
	case PermWorkspaceWrite:
		return requiredPerm != PermDangerFullAccess
	case PermPrompt:
		return true
	default:
		return false
	}
}

// ModelPricing holds per-token cost information.
type ModelPricing struct {
	InputCostPerMillion         float64
	OutputCostPerMillion        float64
	CacheCreationCostPerMillion float64
	CacheReadCostPerMillion     float64
}

// UsageTracker tracks token usage across turns.
type UsageTracker struct {
	LatestTurn  api.Usage
	Cumulative  api.Usage
	Turns       int
	mu          sync.Mutex
}

// NewUsageTracker creates a new usage tracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{}
}

// Record adds a usage entry.
func (t *UsageTracker) Record(usage api.Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LatestTurn = usage
	t.Cumulative.InputTokens += usage.InputTokens
	t.Cumulative.OutputTokens += usage.OutputTokens
	t.Cumulative.CacheCreationInputTokens += usage.CacheCreationInputTokens
	t.Cumulative.CacheReadInputTokens += usage.CacheReadInputTokens
	t.Turns++
}

// EstimateCost calculates the estimated cost in USD.
func (t *UsageTracker) EstimateCost(model string) (float64, float64, float64) {
	pricing := PricingForModel(model)
	input := float64(t.Cumulative.InputTokens) * pricing.InputCostPerMillion / 1_000_000
	output := float64(t.Cumulative.OutputTokens) * pricing.OutputCostPerMillion / 1_000_000
	cacheCreate := float64(t.Cumulative.CacheCreationInputTokens) * pricing.CacheCreationCostPerMillion / 1_000_000
	cacheRead := float64(t.Cumulative.CacheReadInputTokens) * pricing.CacheReadCostPerMillion / 1_000_000
	total := input + output + cacheCreate + cacheRead
	return input, output, total
}

// PricingForModel returns pricing for the given model.
func PricingForModel(model string) ModelPricing {
	switch {
	case contains(model, "haiku"):
		return ModelPricing{1.0, 5.0, 1.25, 0.1}
	case contains(model, "opus"):
		return ModelPricing{15.0, 75.0, 18.75, 1.5}
	default: // sonnet and others
		return ModelPricing{15.0, 75.0, 18.75, 1.5}
	}
}

// FormatUSD formats a float as USD string.
func FormatUSD(amount float64) string {
	return fmt.Sprintf("$%.4f", amount)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ToolOutput represents the result of a tool execution.
type ToolOutput struct {
	Content string
	IsError bool
}

// ToolExecutor is the interface for executing tools.
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error)
	GetToolSpecs() []api.ToolDefinition
}

// BuiltinToolExecutor provides basic built-in tool execution.
type BuiltinToolExecutor struct {
	WorkspaceRoot string
}

// NewBuiltinToolExecutor creates a new tool executor.
func NewBuiltinToolExecutor(workspaceRoot string) *BuiltinToolExecutor {
	return &BuiltinToolExecutor{WorkspaceRoot: workspaceRoot}
}

// ExecuteTool dispatches to the appropriate tool handler.
func (e *BuiltinToolExecutor) ExecuteTool(ctx context.Context, name string, input json.RawMessage) (*ToolOutput, error) {
	switch name {
	case "bash":
		return e.executeBash(ctx, input)
	case "read_file":
		return e.executeReadFile(input)
	case "write_file":
		return e.executeWriteFile(input)
	case "edit_file":
		return e.executeEditFile(input)
	default:
		return &ToolOutput{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}, nil
	}
}

func (e *BuiltinToolExecutor) executeBash(ctx context.Context, input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing bash input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command)
	cmd.Dir = e.WorkspaceRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n" + stderr.String()
	}
	if output == "" && err != nil {
		output = err.Error()
	}

	return &ToolOutput{Content: output, IsError: err != nil}, nil
}

func (e *BuiltinToolExecutor) executeReadFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing read_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: string(data)}, nil
}

func (e *BuiltinToolExecutor) executeWriteFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing write_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error creating directory: %v", err), IsError: true}, nil
	}

	if err := os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: fmt.Sprintf("Successfully wrote %s", path)}, nil
}

func (e *BuiltinToolExecutor) executeEditFile(input json.RawMessage) (*ToolOutput, error) {
	var args struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, fmt.Errorf("parsing edit_file input: %w", err)
	}

	path := e.resolvePath(args.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
	}

	content := string(data)
	if !strings.Contains(content, args.OldString) {
		return &ToolOutput{Content: fmt.Sprintf("old_string not found in %s", path), IsError: true}, nil
	}

	newContent := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
		return &ToolOutput{Content: fmt.Sprintf("Error writing file: %v", err), IsError: true}, nil
	}

	return &ToolOutput{Content: fmt.Sprintf("Successfully edited %s", path)}, nil
}

// resolvePath resolves a path relative to the workspace root.
// If the path is absolute, it's returned as-is.
func (e *BuiltinToolExecutor) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(e.WorkspaceRoot, p)
}

// GetToolSpecs returns available tool definitions.
func (e *BuiltinToolExecutor) GetToolSpecs() []api.ToolDefinition {
	return []api.ToolDefinition{
		{Name: "bash", Description: "Execute shell commands", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
		{Name: "read_file", Description: "Read file contents", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		{Name: "write_file", Description: "Write file contents", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)},
		{Name: "edit_file", Description: "Edit file with string replacement", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"}}}`)},
	}
}

// Config holds application configuration.
type Config struct {
	Model            string         `json:"model"`
	APIKey           string         `json:"apiKey,omitempty"`
	BaseURL          string         `json:"baseUrl,omitempty"`
	MaxTokens        int            `json:"maxTokens"`
	Temperature      float64        `json:"temperature"`
	SystemPromptPath string         `json:"systemPromptPath,omitempty"`
	PermissionMode   PermissionMode `json:"permissionMode"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Model:          "claude-sonnet-4-6",
		MaxTokens:      16384,
		Temperature:    1.0,
		PermissionMode: PermWorkspaceWrite,
	}
}

// ConfigFromSettings converts a config.Settings into a runtime Config.
func ConfigFromSettings(s config.Settings) *Config {
	c := DefaultConfig()

	if s.Model != "" {
		c.Model = s.Model
	}
	if s.MaxTokens != 0 {
		c.MaxTokens = s.MaxTokens
	}
	if s.Temperature != nil {
		c.Temperature = *s.Temperature
	}
	if s.APIKey != "" {
		c.APIKey = s.APIKey
	}
	if s.APIBaseURL != "" {
		c.BaseURL = s.APIBaseURL
	}
	if s.SystemPrompt != "" {
		c.SystemPromptPath = s.SystemPrompt
	}
	if s.Permissions.Mode != "" {
		c.PermissionMode = PermissionMode(s.Permissions.Mode)
	}

	return c
}

// LoadConfig loads configuration from a file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &config, nil
}

// ApplyOverrides applies CLI flag overrides to the config.
func (c *Config) ApplyOverrides(model string, permMode string) {
	if model != "" {
		c.Model = model
	}
	if permMode != "" {
		c.PermissionMode = PermissionMode(permMode)
	}
}

// SystemPromptBuilder constructs system prompts.
type SystemPromptBuilder struct {
	ProjectContext     string
	InstructionFiles   map[string]string
	GitStatus          string
	GitDiff            string
	ToolDescriptions   []string
	CustomInstructions string
	LSPEnrichment      string
}

// NewSystemPromptBuilder creates a new builder.
func NewSystemPromptBuilder() *SystemPromptBuilder {
	return &SystemPromptBuilder{
		InstructionFiles: make(map[string]string),
	}
}

// LoadInstructionFiles discovers and loads GLAW.md (and CLAW.md backward-compat)
// instruction files from the workspace root.
// Search order:
//  1. {root}/GLAW.md
//  2. {root}/.glaw/GLAW.md
//  3. {root}/.glaw/instructions/*.md
//  4. {root}/CLAW.md  (backward compat)
//  5. {root}/.glaw/CLAW.md (backward compat)
func LoadInstructionFiles(root string) map[string]string {
	files := make(map[string]string)

	// Primary: GLAW.md at project root
	loadInstructionFile(files, root, "GLAW.md")
	// Primary: .glaw/GLAW.md
	loadInstructionFile(files, root, ".glaw/GLAW.md")
	// Primary: .glaw/instructions/*.md
	loadInstructionDir(files, root, ".glaw/instructions")
	// Backward compat: CLAW.md
	loadInstructionFile(files, root, "CLAW.md")
	// Backward compat: .glaw/CLAW.md
	loadInstructionFile(files, root, ".glaw/CLAW.md")

	return files
}

func loadInstructionFile(files map[string]string, root, relPath string) {
	fullPath := filepath.Join(root, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	files[relPath] = string(data)
}

func loadInstructionDir(files map[string]string, root, relDir string) {
	dirPath := filepath.Join(root, relDir)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		relPath := filepath.Join(relDir, entry.Name())
		loadInstructionFile(files, root, relPath)
	}
}

// Build assembles the system prompt.
func (b *SystemPromptBuilder) Build() string {
	var parts []string

	parts = append(parts, `You are glaw-code, an AI coding assistant. You help users with software engineering tasks.

IMPORTANT: You have access to tools. When the user asks you to create files, edit code, run commands, or perform any action, you MUST use the appropriate tool rather than just describing what you would do. For example:
- To create or write files, use the write_file tool
- To edit existing files, use the edit_file tool
- To read files, use the read_file tool
- To run commands, use the bash tool
- To search for files, use glob_search
- To search file contents, use grep_search

Always execute the actual tool calls to accomplish the task. Do NOT just describe what you would do - actually do it.`)

	if b.ProjectContext != "" {
		parts = append(parts, "\n## Project Context\n"+b.ProjectContext)
	}

	for name, content := range b.InstructionFiles {
		parts = append(parts, fmt.Sprintf("\n## Instructions from %s\n%s", name, content))
	}

	if b.GitStatus != "" {
		parts = append(parts, "\n## Git Status\n"+b.GitStatus)
	}

	if b.ToolDescriptions != nil {
		parts = append(parts, "\n## Available Tools")
		parts = append(parts, b.ToolDescriptions...)
	}

	if b.CustomInstructions != "" {
		parts = append(parts, "\n## Custom Instructions\n"+b.CustomInstructions)
	}

	if b.LSPEnrichment != "" {
		parts = append(parts, "\n"+b.LSPEnrichment)
	}

	return join(parts, "\n")
}

func join(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

// ConversationRuntime is the central orchestrator.
type ConversationRuntime struct {
	APIClient    api.ProviderClient
	Session      *Session
	Config       *Config
	Permissions  *PermissionManager
	Usage        *UsageTracker
	ToolExecutor ToolExecutor
	Snapshotter  *SnapshottingExecutor
	SystemPrompt string
	LSPManager   LSPProvider

	// PermissionChecker is called before each tool execution.
	// If it returns false, the tool is not executed and the message is
	// returned as a tool error to the model.
	PermissionChecker func(toolName string, input json.RawMessage) bool

	// running tracks whether an agentic action is currently in progress.
	running bool
	// cancelAction can be called to cancel the current running action.
	cancelAction context.CancelFunc
	mu          sync.Mutex
}

// IsRunning returns whether an agentic action is currently in progress.
func (r *ConversationRuntime) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// SetRunning marks an action as running and stores the cancel function.
func (r *ConversationRuntime) SetRunning(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = true
	r.cancelAction = cancel
}

// SetIdle marks the action as no longer running.
func (r *ConversationRuntime) SetIdle() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.cancelAction = nil
}

// CancelAction cancels the currently running action (if any).
// Returns true if an action was cancelled.
func (r *ConversationRuntime) CancelAction() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelAction != nil {
		r.cancelAction()
		r.cancelAction = nil
		r.running = false
		return true
	}
	return false
}

// LSPProvider provides LSP context enrichment for the system prompt.
type LSPProvider interface {
	SupportsPath(path string) bool
	ContextEnrichment(ctx context.Context, filePath string, line, character int) (*lsp.ContextEnrichment, error)
	Status() []lsp.ServerStatus
	SupportedExtensions() []string
	Shutdown() error
}

// NewConversationRuntime creates a new runtime.
func NewConversationRuntime(
	client api.ProviderClient,
	config *Config,
	session *Session,
	permManager *PermissionManager,
	exec ToolExecutor,
) *ConversationRuntime {
	return &ConversationRuntime{
		APIClient:    client,
		Session:      session,
		Config:       config,
		Permissions:  permManager,
		Usage:        NewUsageTracker(),
		ToolExecutor: exec,
	}
}

// TurnResult holds the result of a single agentic turn.
type TurnResult struct {
	Response   *api.Response
	ToolCalls  []api.ContentBlock
	StopReason api.StopReason
	Usage      api.Usage
}

// Turn executes a single agentic turn.
func (r *ConversationRuntime) Turn(ctx context.Context) (*TurnResult, error) {
	systemPrompt := r.BuildSystemPrompt()
	toolDefs := r.BuildToolDefinitions()
	messages := r.Session.AsAPIMessages()

	req := api.Request{
		Model:      r.Config.Model,
		Messages:   messages,
		Tools:      toolDefs,
		MaxTokens:  r.Config.MaxTokens,
		Stream:     false,
		System:     systemPrompt,
	}

	// Show spinner while waiting for API response
	spin := newSpinner("Thinking...")
	resp, err := r.APIClient.SendMessage(ctx, req)
	spin.Stop()
	if err != nil {
		// Check if the context was cancelled (user-initiated cancel)
		if ctx.Err() != nil {
			return nil, &ActionCancelledError{}
		}
		return nil, err
	}

	// Track usage
	r.Usage.Record(resp.Usage)

	// Add assistant message to session
	r.Session.AddAssistantMessage(resp.Content, &resp.Usage)

	// Collect tool calls
	var toolCalls []api.ContentBlock
	for _, block := range resp.Content {
		if block.Type == api.ContentToolUse {
			toolCalls = append(toolCalls, block)
		}
	}

	return &TurnResult{
		Response:   resp,
		ToolCalls:  toolCalls,
		StopReason: resp.StopReason,
		Usage:      resp.Usage,
	}, nil
}

// ActionCancelledError is returned when the user cancels a running action.
type ActionCancelledError struct{}

func (e *ActionCancelledError) Error() string {
	return "action cancelled by user"
}

// IsActionCancelled returns true if the error is due to user cancellation.
func IsActionCancelled(err error) bool {
	_, ok := err.(*ActionCancelledError)
	return ok
}

// RunLoop executes multiple turns until done with rich terminal output.
func (r *ConversationRuntime) RunLoop(ctx context.Context) error {
	for {
		// Check for cancellation before each turn
		select {
		case <-ctx.Done():
			return &ActionCancelledError{}
		default:
		}

		result, err := r.Turn(ctx)
		if err != nil {
			return err
		}

		// Render text content from the response
		for _, block := range result.Response.Content {
			if block.Type == api.ContentText && block.Text != "" {
				text := strings.TrimSpace(block.Text)
				if text == "" {
					continue
				}
				lines := strings.Split(text, "\n")
				if len(lines) <= 3 {
					// Short text: show as collapsible single block
					fmt.Printf("%s%s⏺ %s%s\n", ansiDim, ansiItalic, lines[0], ansiReset)
				} else {
					// Longer text: show header + collapsed body
					header := lines[0]
					if len(header) > 80 {
						header = header[:77] + "..."
					}
					fmt.Printf("%s%s▼ %s%s\n", ansiCyan, ansiItalic, header, ansiReset)
					for _, line := range lines[1:] {
						display := line
						if len(display) > 100 {
							display = display[:97] + "..."
						}
						fmt.Printf("%s  │ %s%s\n", ansiDim, display, ansiReset)
					}
				}
			}
		}

		// If tool use, execute tools with spinners and continue the loop
		if result.StopReason == api.StopToolUse {
			for _, tc := range result.ToolCalls {
				// Check for cancellation before each tool
				select {
				case <-ctx.Done():
					return &ActionCancelledError{}
				default:
				}

				// Show tool header
				fmt.Println(renderToolHeader(tc.Name, tc.Input))

				// Check permission before executing
				if r.PermissionChecker != nil && !r.PermissionChecker(tc.Name, tc.Input) {
					msg := fmt.Sprintf("Permission denied for tool %q. The user did not approve this action.", tc.Name)
					fmt.Println(renderToolDone(tc.Name, "Permission denied", true, 0))
					r.Session.AddToolResult(tc.ID, msg, true)
					continue
				}

				// Run tool with spinner
				toolSpin := newSpinner("Running...")
				start := time.Now()
				output, err := r.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
				elapsed := time.Since(start)
				toolSpin.Stop()

				if err != nil {
					if ctx.Err() != nil {
						fmt.Println(renderToolDone(tc.Name, "Cancelled", true, elapsed))
						r.Session.AddToolResult(tc.ID, "Tool execution cancelled by user", true)
						return &ActionCancelledError{}
					}
					fmt.Println(renderToolDone(tc.Name, err.Error(), true, elapsed))
					r.Session.AddToolResult(tc.ID, err.Error(), true)
				} else {
					fmt.Println(renderToolDone(tc.Name, output.Content, output.IsError, elapsed))
					r.Session.AddToolResult(tc.ID, output.Content, output.IsError)
				}
			}
			continue
		}

		// For any other stop reason (end_turn, max_tokens, stop_sequence, etc.), stop
		return nil
	}
}

// RunToolLoop continues executing tool calls after a response.
func (r *ConversationRuntime) RunToolLoop(ctx context.Context, result *TurnResult) error {
	for _, tc := range result.ToolCalls {
		fmt.Println(renderToolHeader(tc.Name, tc.Input))

		// Check permission before executing
		if r.PermissionChecker != nil && !r.PermissionChecker(tc.Name, tc.Input) {
			msg := fmt.Sprintf("Permission denied for tool %q. The user did not approve this action.", tc.Name)
			fmt.Println(renderToolDone(tc.Name, "Permission denied", true, 0))
			r.Session.AddToolResult(tc.ID, msg, true)
			continue
		}

		toolSpin := newSpinner("Running...")
		start := time.Now()
		output, err := r.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
		elapsed := time.Since(start)
		toolSpin.Stop()
		if err != nil {
			fmt.Println(renderToolDone(tc.Name, err.Error(), true, elapsed))
			r.Session.AddToolResult(tc.ID, err.Error(), true)
		} else {
			fmt.Println(renderToolDone(tc.Name, output.Content, output.IsError, elapsed))
			r.Session.AddToolResult(tc.ID, output.Content, output.IsError)
		}
	}
	return r.RunLoop(ctx)
}

// BuildSystemPrompt constructs the system prompt.
func (r *ConversationRuntime) BuildSystemPrompt() string {
	if r.SystemPrompt != "" {
		return r.SystemPrompt
	}
	builder := NewSystemPromptBuilder()

	// Load GLAW.md instruction files from workspace
	if r.Permissions != nil && r.Permissions.WorkspaceRoot != "" {
		builder.InstructionFiles = LoadInstructionFiles(r.Permissions.WorkspaceRoot)
	}

	// Enrich with LSP context if available
	if r.LSPManager != nil {
		enrichment := buildLSPEnrichmentSection(r.LSPManager)
		if enrichment != "" {
			builder.LSPEnrichment = enrichment
		}
	}

	return builder.Build()
}

// BuildToolDefinitions builds tool definitions for the API request.
func (r *ConversationRuntime) BuildToolDefinitions() []api.ToolDefinition {
	if r.ToolExecutor != nil {
		return r.ToolExecutor.GetToolSpecs()
	}
	return nil
}

// Commands Runtime interface implementation

// GetModel returns the current model name.
func (r *ConversationRuntime) GetModel() string {
	return r.Config.Model
}

// SetModel changes the model.
func (r *ConversationRuntime) SetModel(model string) {
	r.Config.Model = model
}

// GetPermissionMode returns the current permission mode.
func (r *ConversationRuntime) GetPermissionMode() string {
	return string(r.Permissions.Mode)
}

// SetPermissionMode changes the permission mode.
func (r *ConversationRuntime) SetPermissionMode(mode string) {
	r.Permissions.Mode = PermissionMode(mode)
}

// IsYoloMode returns whether yolo mode is currently active.
func (r *ConversationRuntime) IsYoloMode() bool {
	return r.Permissions.IsYolo()
}

// ToggleYoloMode toggles yolo mode on/off.
// Returns true if yolo is now enabled, false if disabled.
func (r *ConversationRuntime) ToggleYoloMode() bool {
	return r.Permissions.ToggleYolo()
}

// GetMessageCount returns the number of messages in the session.
func (r *ConversationRuntime) GetMessageCount() int {
	return r.Session.MessageCount()
}

// GetSessionID returns the session ID.
func (r *ConversationRuntime) GetSessionID() string {
	return r.Session.ID
}

// LoadSession replaces the current session with one loaded from disk.
// It first saves the current session, then loads the requested one,
// and resets the usage tracker.
func (r *ConversationRuntime) LoadSession(sessionID string) error {
	workspaceRoot := r.GetWorkspaceRoot()
	sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")

	// Save current session first
	if r.Session.ID != "" {
		if _, err := SaveSession(r.Session, sessionsDir); err != nil {
			return fmt.Errorf("saving current session: %w", err)
		}
	}

	// Load the requested session
	path := filepath.Join(sessionsDir, sessionID+".json")
	session, err := LoadSession(path)
	if err != nil {
		return fmt.Errorf("loading session %s: %w", sessionID, err)
	}

	// Replace session and reset usage
	r.Session = session
	r.Usage = NewUsageTracker()

	return nil
}

// NewSession saves the current session and creates a fresh empty one.
func (r *ConversationRuntime) NewSession() {
	workspaceRoot := r.GetWorkspaceRoot()
	sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")

	// Save current session first
	if r.Session.ID != "" {
		if _, err := SaveSession(r.Session, sessionsDir); err == nil {
			// saved successfully
		}
	}

	// Create fresh session and reset usage
	r.Session = NewSession()
	r.Usage = NewUsageTracker()
}

// GetUsageInfo returns usage statistics for the commands interface.
func (r *ConversationRuntime) GetUsageInfo() commands.UsageInfo {
	_, _, total := r.Usage.EstimateCost(r.Config.Model)
	return commands.UsageInfo{
		InputTokens:  r.Usage.Cumulative.InputTokens,
		OutputTokens: r.Usage.Cumulative.OutputTokens,
		TotalCostUSD: total,
	}
}

// CompactSession compacts the session history.
func (r *ConversationRuntime) CompactSession() error {
	// Simple truncation: keep last 20 messages
	if r.Session.MessageCount() > 20 {
		r.Session.Messages = r.Session.Messages[len(r.Session.Messages)-20:]
	}
	return nil
}

// ClearSession clears the conversation.
func (r *ConversationRuntime) ClearSession() {
	r.Session.Messages = nil
}

// RevertLastTurn restores all files modified in the most recent turn.
func (r *ConversationRuntime) RevertLastTurn() (int, error) {
	if r.Snapshotter == nil {
		return 0, fmt.Errorf("no snapshot data available")
	}
	return r.Snapshotter.RevertLastTurn()
}

// RevertAll restores all files modified across all turns.
func (r *ConversationRuntime) RevertAll() (int, error) {
	if r.Snapshotter == nil {
		return 0, fmt.Errorf("no snapshot data available")
	}
	return r.Snapshotter.RevertAll()
}

// RunGitCommand executes a git command.
func (r *ConversationRuntime) RunGitCommand(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetWorkspaceRoot returns the workspace root path.
func (r *ConversationRuntime) GetWorkspaceRoot() string {
	if r.Permissions != nil {
		return r.Permissions.WorkspaceRoot
	}
	return ""
}

// GetAllSettings returns the current config as a generic map.
func (r *ConversationRuntime) GetAllSettings() map[string]interface{} {
	result := make(map[string]interface{})
	data, err := json.Marshal(r.Config)
	if err != nil {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result
	}
	return result
}

// SetConfigValue sets a top-level config key and persists to project settings.
func (r *ConversationRuntime) SetConfigValue(key, value string) error {
	// Load current project settings
	workspaceRoot := r.GetWorkspaceRoot()
	settings, err := config.LoadAll(workspaceRoot)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	switch key {
	case "model":
		settings.Model = value
		r.Config.Model = value
	case "maxTokens":
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
			return fmt.Errorf("invalid integer: %s", value)
		}
		settings.MaxTokens = n
		r.Config.MaxTokens = n
	case "temperature":
		var f float64
		if _, err := fmt.Sscanf(value, "%f", &f); err != nil {
			return fmt.Errorf("invalid float: %s", value)
		}
		settings.Temperature = &f
		r.Config.Temperature = f
	case "apiKey":
		settings.APIKey = value
		r.Config.APIKey = value
	case "apiBaseUrl":
		settings.APIBaseURL = value
		r.Config.BaseURL = value
	case "systemPrompt":
		settings.SystemPrompt = value
		r.Config.SystemPromptPath = value
	default:
		return fmt.Errorf("unknown config key: %q", key)
	}

	// Save to project settings
	return config.SaveProject(workspaceRoot, settings)
}


// buildLSPEnrichmentSection creates a system prompt section describing LSP capabilities.
func buildLSPEnrichmentSection(mgr LSPProvider) string {
	exts := mgr.SupportedExtensions()
	if len(exts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## LSP Integration\n\n")
	sb.WriteString("Language Server Protocol (LSP) is available for this workspace. ")
	sb.WriteString(fmt.Sprintf("Supported file types: %s\n\n", strings.Join(exts, ", ")))
	sb.WriteString("You have access to the following LSP tools for code navigation and analysis:\n")
	sb.WriteString("- **lsp_go_to_definition**: Jump to the definition of a symbol at a given position\n")
	sb.WriteString("- **lsp_find_references**: Find all references to a symbol\n")
	sb.WriteString("- **lsp_hover**: Get type and documentation info for a symbol\n")
	sb.WriteString("- **lsp_document_symbol**: List all symbols in a file\n")
	sb.WriteString("- **lsp_workspace_symbol**: Search for symbols across the workspace\n")
	sb.WriteString("- **lsp_go_to_implementation**: Find concrete implementations of an interface\n")
	sb.WriteString("- **lsp_incoming_calls**: Find all callers of a function/method\n")
	sb.WriteString("- **lsp_outgoing_calls**: Find all functions called by a function/method\n\n")
	sb.WriteString("Use these tools to understand code structure, navigate definitions, trace call hierarchies, and verify refactoring safety.")
	return sb.String()
}

// GetLSPStatus returns LSP server statuses for the commands interface.
func (r *ConversationRuntime) GetLSPStatus() []commands.LSPServerStatus {
	if r.LSPManager == nil {
		return nil
	}
	statuses := r.LSPManager.Status()
	result := make([]commands.LSPServerStatus, len(statuses))
	for i, s := range statuses {
		result[i] = commands.LSPServerStatus{
			Name:    s.Name,
			Command: s.Command,
			Running: s.Running,
		}
	}
	return result
}
