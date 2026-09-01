package entity

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

	api "github.com/hieu-glaw/glaw-code/internal/api"
	commands "github.com/hieu-glaw/glaw-code/internal/modules/commands/domain/entity"
	config "github.com/hieu-glaw/glaw-code/internal/modules/config/domain/entity"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
)

// Config holds application configuration.
type Config struct {
	Model              string                    `json:"model"`
	APIKey             string                    `json:"apiKey,omitempty"`
	BaseURL            string                    `json:"baseUrl,omitempty"`
	MaxTokens          int                       `json:"maxTokens"`
	Temperature        float64                   `json:"temperature"`
	SystemPromptPath   string                    `json:"systemPromptPath,omitempty"`
	PermissionMode     permentity.PermissionMode `json:"permissionMode"`
	MaxContextMessages int                       `json:"maxContextMessages,omitempty"` // Maximum messages to include in context (0 = unlimited)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Model:              "openrouter:nvidia/nemotron-3-super-120b-a12b:free",
		MaxTokens:          16384,
		Temperature:        1.0,
		PermissionMode:     permentity.PermWorkspaceWrite,
		MaxContextMessages: 50, // Keep last 50 messages by default
	}
}

// ConfigFromSettings converts a config.Settings into a Config.
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
		c.PermissionMode = permentity.PermissionMode(s.Permissions.Mode)
	}
	if s.MaxContextMessages > 0 {
		c.MaxContextMessages = s.MaxContextMessages
	}

	return c
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

// SubAgentSessionProvider is an interface for querying sub-agent session information.
type SubAgentSessionProvider interface {
	ListAgentStatuses() []commands.SubAgentSessionInfo
	LoadAgentSession(agentID string) (sessionID string, msgCount int, err error)
}

// ConversationRuntime is the central orchestrator.
type ConversationRuntime struct {
	APIClient    api.ProviderClient
	Session      *sessionentity.Session
	Config       *Config
	Permissions  *permservice.PermissionManager
	Usage        *UsageTracker
	ToolExecutor ToolExecutor
	Snapshotter  *SnapshottingExecutor
	SystemPrompt string

	SubAgentMgr SubAgentSessionProvider

	ClientFactory func(model string) (api.ProviderClient, error)

	PermissionChecker func(toolName string, input json.RawMessage) bool

	running      bool
	cancelAction context.CancelFunc
	mu           sync.Mutex
}

// NewConversationRuntime creates a new runtime.
func NewConversationRuntime(
	client api.ProviderClient,
	config *Config,
	session *sessionentity.Session,
	permManager *permservice.PermissionManager,
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
	messages := r.Session.AsAPIMessagesWithLimit(r.Config.MaxContextMessages)

	req := api.Request{
		Model:     r.Config.Model,
		Messages:  messages,
		Tools:     toolDefs,
		MaxTokens: r.Config.MaxTokens,
		Stream:    false,
		System:    systemPrompt,
	}

	resp, err := r.APIClient.SendMessage(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, &ActionCancelledError{}
		}
		return nil, err
	}

	r.Usage.Record(resp.Usage)
	r.Session.AddAssistantMessage(resp.Content, &resp.Usage)

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

// BuildSystemPrompt constructs the system prompt.
func (r *ConversationRuntime) BuildSystemPrompt() string {
	if r.SystemPrompt != "" {
		return r.SystemPrompt
	}
	builder := NewSystemPromptBuilder()
	if r.Permissions != nil && r.Permissions.WorkspaceRoot != "" {
		builder.InstructionFiles = LoadInstructionFiles(r.Permissions.WorkspaceRoot)
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
	if r.ClientFactory != nil {
		if newClient, err := r.ClientFactory(model); err == nil {
			r.APIClient = newClient
		}
	}
}

// GetPermissionMode returns the current permission mode.
func (r *ConversationRuntime) GetPermissionMode() string {
	return string(r.Permissions.Mode)
}

// SetPermissionMode changes the permission mode.
func (r *ConversationRuntime) SetPermissionMode(mode string) {
	r.Permissions.Mode = permentity.PermissionMode(mode)
}

// IsYoloMode returns whether yolo mode is currently active.
func (r *ConversationRuntime) IsYoloMode() bool {
	return r.Permissions.IsYolo()
}

// ToggleYoloMode toggles yolo mode on/off.
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
func (r *ConversationRuntime) LoadSession(sessionID string) error {
	workspaceRoot := r.GetWorkspaceRoot()
	sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")

	if r.Session.ID != "" {
		if _, err := SaveSession(r.Session, sessionsDir); err != nil {
			return fmt.Errorf("saving current session: %w", err)
		}
	}

	path := filepath.Join(sessionsDir, sessionID+".json")
	session, err := LoadSession(path)
	if err != nil {
		return fmt.Errorf("loading session %s: %w", sessionID, err)
	}

	r.Session = session
	r.Usage = NewUsageTracker()
	return nil
}

// NewSession saves the current session and creates a fresh empty one.
func (r *ConversationRuntime) NewSession() {
	workspaceRoot := r.GetWorkspaceRoot()
	sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")

	if r.Session.ID != "" {
		_, _ = SaveSession(r.Session, sessionsDir)
	}

	r.Session = sessionentity.NewSession()
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

	return config.SaveProject(workspaceRoot, settings)
}

// GetSubAgentSessions returns metadata for all tracked sub-agents.
func (r *ConversationRuntime) GetSubAgentSessions() []commands.SubAgentSessionInfo {
	if r.SubAgentMgr == nil {
		return nil
	}
	return r.SubAgentMgr.ListAgentStatuses()
}

// ResumeSubAgentSession saves the current session, loads the sub-agent's session.
func (r *ConversationRuntime) ResumeSubAgentSession(agentID string) error {
	if r.SubAgentMgr == nil {
		return fmt.Errorf("no sub-agent manager configured")
	}

	sessionID, _, err := r.SubAgentMgr.LoadAgentSession(agentID)
	if err != nil {
		return fmt.Errorf("resuming sub-agent %s: %w", agentID, err)
	}

	workspaceRoot := r.GetWorkspaceRoot()
	sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")
	if r.Session.ID != "" {
		_, _ = SaveSession(r.Session, sessionsDir)
	}

	_ = sessionID
	r.Usage = NewUsageTracker()
	return nil
}

// RunLoop executes multiple turns until done with rich terminal output.
// This is the non-streaming version used by the one-shot runner.
func (r *ConversationRuntime) RunLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return &ActionCancelledError{}
		default:
		}

		result, err := r.Turn(ctx)
		if err != nil {
			return err
		}

		for _, block := range result.Response.Content {
			if block.Type == api.ContentText && block.Text != "" {
				text := strings.TrimSpace(block.Text)
				if text == "" {
					continue
				}
				lines := strings.Split(text, "\n")
				if len(lines) <= 3 {
					fmt.Printf("%s%s⏺ %s%s\n", ansiDim, ansiItalic, lines[0], ansiReset)
				} else {
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

		if result.StopReason == api.StopToolUse {
			for _, tc := range result.ToolCalls {
				select {
				case <-ctx.Done():
					return &ActionCancelledError{}
				default:
				}

				fmt.Println(renderToolHeader(tc.Name, tc.Input))

				if r.PermissionChecker != nil && !r.PermissionChecker(tc.Name, tc.Input) {
					msg := fmt.Sprintf("Permission denied for tool %q. The user did not approve this action.", tc.Name)
					fmt.Println(renderToolDone(tc.Name, "Permission denied", true, 0))
					r.Session.AddToolResult(tc.ID, msg, true)
					continue
				}

				start := time.Now()
				output, err := r.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
				elapsed := time.Since(start)

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

		return nil
	}
}

// RunLoopStream executes multiple turns using streaming, calling textCallback
// with each text delta as it arrives from the API. When a tool_use stop reason
// is encountered, tool calls are displayed inline and executed. The function
// continues processing events from the same stream channel, supporting multiple
// turns within a single streaming session. This provides real-time output for the CLI.
//
// The textCallback receives raw text deltas (not yet markdown-rendered) and should
// print them to stdout. The caller is responsible for handling the spinner,
// cursor positioning, etc.
func (r *ConversationRuntime) RunLoopStream(ctx context.Context, textCallback func(string)) error {
	// Start a streaming turn
	ch, err := r.StreamTurn(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return &ActionCancelledError{}
		}
		return err
	}

	// Consume the stream: send text deltas to callback, handle multiple turns
	for {
		select {
		case <-ctx.Done():
			return &ActionCancelledError{}
		default:
		}

		// Accumulate a single turn from the stream
		result, err := r.ConsumeStream(ctx, ch, textCallback)
		if err != nil {
			if ctx.Err() != nil {
				return &ActionCancelledError{}
			}
			return err
		}

		// Channel closed - we're done
		if result == nil {
			return nil
		}

		// Record usage
		r.Usage.Record(result.Usage)

		// Add the assistant message to the session
		if len(result.Content) > 0 {
			r.Session.AddAssistantMessage(result.Content, &result.Usage)
		}

		// If the stop reason is tool_use, execute tools and continue to next turn
		if result.StopReason == api.StopToolUse && len(result.ToolCalls) > 0 {
			if err := r.RunToolLoopStream(ctx, result.ToolCalls, textCallback); err != nil {
				return err
			}
			// After tool execution, continue consuming the same stream for next turn
			continue
		}

		// End turn — not tool_use, so we're done
		return nil
	}
}

// ConsumeStream reads events from the streaming channel, calling textCallback
// for each text delta in real time. It returns the fully accumulated StreamResponse
// once the stream completes. This separates the real-time display concern from
// the accumulation concern.
func (r *ConversationRuntime) ConsumeStream(ctx context.Context, ch <-chan api.StreamEvent, textCallback func(string)) (*StreamResponse, error) {
	resp := &StreamResponse{}

	for {
		select {
		case <-ctx.Done():
			return nil, &ActionCancelledError{}

		case event, ok := <-ch:
			if !ok {
				// Channel closed
				if len(resp.Content) == 0 {
					resp.Content = []api.ContentBlock{api.NewTextBlock("")}
				}
				return resp, nil
			}

			if event.Error != nil {
				if ctx.Err() != nil {
					return nil, &ActionCancelledError{}
				}
				return resp, event.Error
			}

			switch event.Type {
			case api.EventContentBlockDelta:
				var text string
				// Anthropic sends raw JSON deltas; OpenAI sends plain strings.
				if content, ok := event.Content.(string); ok {
					text = content
				} else if rawJSON, ok := event.Content.([]byte); ok {
					text = extractTextFromAnthropicDelta(rawJSON)
				}
				if text != "" {
					resp.TextBlocks = append(resp.TextBlocks, text)
					if textCallback != nil {
						textCallback(text)
					}
				}

			case api.EventMessageDelta:
				if sr, ok := event.Content.(api.StopReason); ok {
					resp.StopReason = sr
				}

			case api.EventMessageStop:
				// EventMessageStop carries a *Response with fully-formed content blocks
				// (including tool_use blocks). Use those as the authoritative source.
				if fullResp, ok := event.Content.(*api.Response); ok {
					resp.Content = fullResp.Content
					resp.StopReason = fullResp.StopReason
					resp.Usage = fullResp.Usage
					// Extract tool_use blocks
					for _, block := range resp.Content {
						if block.Type == api.ContentToolUse {
							resp.ToolCalls = append(resp.ToolCalls, block)
						}
					}
					return resp, nil
				}
				// Fallback: build content from accumulated text blocks
				resp.Content = make([]api.ContentBlock, 0, len(resp.TextBlocks)+len(resp.ToolCalls))
				for _, t := range resp.TextBlocks {
					resp.Content = append(resp.Content, api.NewTextBlock(t))
				}
				return resp, nil

			case api.EventError:
				if ctx.Err() != nil {
					return nil, &ActionCancelledError{}
				}
				return resp, event.Error

			case api.EventDone:
				if len(resp.Content) == 0 {
					resp.Content = []api.ContentBlock{api.NewTextBlock("")}
				}
				return resp, nil
			}
		}
	}
}

// StreamTurn calls the API with streaming and returns the channel of events.
func (r *ConversationRuntime) StreamTurn(ctx context.Context) (<-chan api.StreamEvent, error) {
	req := api.Request{
		Model:     r.Config.Model,
		Messages:  r.Session.AsAPIMessagesWithLimit(r.Config.MaxContextMessages),
		Tools:     r.BuildToolDefinitions(),
		MaxTokens: r.Config.MaxTokens,
		Stream:    true,
		System:    r.BuildSystemPrompt(),
	}

	return r.APIClient.StreamMessage(ctx, req)
}

// StreamResponse holds the accumulated result of a streamed turn.
type StreamResponse struct {
	Content    []api.ContentBlock // final content blocks (text + tool_use)
	TextBlocks []string           // accumulated text deltas (raw, pre-merge)
	ToolCalls  []api.ContentBlock // extracted tool_use blocks
	StopReason api.StopReason
	Usage      api.Usage
}

// MergeText returns all accumulated text deltas as a single string.
func (sr *StreamResponse) MergeText() string {
	return strings.Join(sr.TextBlocks, "")
}

// AccumulateStream reads events from the streaming channel and builds a complete response.
// It handles both Anthropic (delta = raw JSON) and OpenAI (delta = plain string) formats.
func (r *ConversationRuntime) AccumulateStream(ctx context.Context, ch <-chan api.StreamEvent) (*StreamResponse, error) {
	resp := &StreamResponse{}

	for event := range ch {
		if event.Error != nil {
			if ctx.Err() != nil {
				return nil, &ActionCancelledError{}
			}
			return resp, event.Error
		}

		switch event.Type {
		case api.EventContentBlockDelta:
			// Anthropic sends raw JSON deltas; OpenAI sends plain strings.
			// Try string first (OpenAI path), then parse JSON (Anthropic path).
			if content, ok := event.Content.(string); ok {
				resp.TextBlocks = append(resp.TextBlocks, content)
			} else if rawJSON, ok := event.Content.([]byte); ok {
				text := extractTextFromAnthropicDelta(rawJSON)
				if text != "" {
					resp.TextBlocks = append(resp.TextBlocks, text)
				}
			}

		case api.EventMessageDelta:
			if sr, ok := event.Content.(api.StopReason); ok {
				resp.StopReason = sr
			}

		case api.EventMessageStop:
			// EventMessageStop carries a *Response with fully-formed content blocks
			// (including tool_use blocks). Use those as the authoritative source.
			if fullResp, ok := event.Content.(*api.Response); ok {
				resp.Content = fullResp.Content
				resp.StopReason = fullResp.StopReason
				resp.Usage = fullResp.Usage
				// Extract tool_use blocks
				for _, block := range resp.Content {
					if block.Type == api.ContentToolUse {
						resp.ToolCalls = append(resp.ToolCalls, block)
					}
				}
				return resp, nil
			}
			// Fallback: build content from accumulated text blocks
			resp.Content = make([]api.ContentBlock, 0, len(resp.TextBlocks)+len(resp.ToolCalls))
			for _, text := range resp.TextBlocks {
				resp.Content = append(resp.Content, api.NewTextBlock(text))
			}
			return resp, nil

		case api.EventError:
			if ctx.Err() != nil {
				return nil, &ActionCancelledError{}
			}
			return resp, event.Error

		case api.EventDone:
			// Ensure we have a valid response even if no events were received.
			if len(resp.Content) == 0 {
				resp.Content = []api.ContentBlock{api.NewTextBlock("")}
			}
			return resp, nil
		}
	}

	// If we get here, return what we have.
	if len(resp.Content) == 0 {
		resp.Content = []api.ContentBlock{api.NewTextBlock("")}
	}
	return resp, nil
}

// anthropicDelta represents the JSON structure of a content_block_delta from Anthropic.
type anthropicDelta struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"` // "text_delta" or "input_json_delta"
		Text string `json:"text"` // for text_delta
		// JSON args deltas for tool_use are accumulated by the API client
	} `json:"delta"`
}

// extractTextFromAnthropicDelta parses an Anthropic content_block_delta JSON
// and extracts the text portion. Returns empty string for non-text deltas.
func extractTextFromAnthropicDelta(rawJSON []byte) string {
	var delta anthropicDelta
	if err := json.Unmarshal(rawJSON, &delta); err != nil {
		return ""
	}
	if delta.Delta.Type == "text_delta" {
		return delta.Delta.Text
	}
	return ""
}

// RunToolLoopStream executes tool calls from a previously streamed turn.
// After tool execution, the caller (RunLoopStream) continues consuming the
// same stream for the next turn's events. This enables the full agentic
// streaming flow: stream text → display tools → execute tools → continue stream.
func (r *ConversationRuntime) RunToolLoopStream(ctx context.Context, toolCalls []api.ContentBlock, textCallback func(string)) error {
	for _, tc := range toolCalls {
		select {
		case <-ctx.Done():
			return &ActionCancelledError{}
		default:
		}

		fmt.Println(renderToolHeader(tc.Name, tc.Input))

		if r.PermissionChecker != nil && !r.PermissionChecker(tc.Name, tc.Input) {
			msg := fmt.Sprintf("Permission denied for tool %q. The user did not approve this action.", tc.Name)
			fmt.Println(renderToolDone(tc.Name, "Permission denied", true, 0))
			r.Session.AddToolResult(tc.ID, msg, true)
			continue
		}

		start := time.Now()
		output, err := r.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
		elapsed := time.Since(start)

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

	// Tools executed successfully — caller will continue consuming the stream
	return nil
}

// RunToolLoop continues executing tool calls after a response.
func (r *ConversationRuntime) RunToolLoop(ctx context.Context, result *TurnResult) error {
	for _, tc := range result.ToolCalls {
		fmt.Println(renderToolHeader(tc.Name, tc.Input))

		if r.PermissionChecker != nil && !r.PermissionChecker(tc.Name, tc.Input) {
			msg := fmt.Sprintf("Permission denied for tool %q. The user did not approve this action.", tc.Name)
			fmt.Println(renderToolDone(tc.Name, "Permission denied", true, 0))
			r.Session.AddToolResult(tc.ID, msg, true)
			continue
		}

		start := time.Now()
		output, err := r.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
		elapsed := time.Since(start)
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

// --- Helper functions for session persistence ---

// SaveSession persists a session to disk.
func SaveSession(session *sessionentity.Session, dir string) (string, error) {
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
func LoadSession(path string) (*sessionentity.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}
	var session sessionentity.Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}
	return &session, nil
}

// ANSI codes for terminal output (used by RunLoop rendering).
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiCyan   = "\033[36m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiItalic = "\033[3m"
)

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

// toolDisplayInfo extracts a short description from tool input JSON.
func toolDisplayInfo(name string, input json.RawMessage) string {
	switch name {
	case "bash":
		var args struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &args) == nil && args.Command != "" {
			if len(args.Command) > 60 {
				return args.Command[:57] + "..."
			}
			return args.Command
		}
	case "write_file", "edit_file", "read_file":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Path
		}
	case "glob_search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	case "grep_search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if json.Unmarshal(input, &args) == nil {
			return args.Pattern
		}
	}
	return ""
}
