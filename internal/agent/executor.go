package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// SubAgentExecutor runs a sub-agent with isolated context, filtered tools,
// and a specialized system prompt. It implements the orchestrator-worker
// pattern where the parent agent delegates tasks to specialized sub-agents.
type SubAgentExecutor struct {
	config       *SubAgentConfig
	toolExecutor runtime.ToolExecutor
	allTools     []api.ToolDefinition
	parentModel  string
	apiClient    api.ProviderClient
}

// NewSubAgentExecutor creates a new executor for the given sub-agent config.
func NewSubAgentExecutor(
	config *SubAgentConfig,
	toolExecutor runtime.ToolExecutor,
	allTools []api.ToolDefinition,
	parentModel string,
) *SubAgentExecutor {
	return &SubAgentExecutor{
		config:       config,
		toolExecutor: toolExecutor,
		allTools:     allTools,
		parentModel:  parentModel,
	}
}

// NewSubAgentExecutorWithClient creates a new executor with an API client
// for real LLM-backed execution.
func NewSubAgentExecutorWithClient(
	config *SubAgentConfig,
	toolExecutor runtime.ToolExecutor,
	allTools []api.ToolDefinition,
	parentModel string,
	apiClient api.ProviderClient,
) *SubAgentExecutor {
	return &SubAgentExecutor{
		config:       config,
		toolExecutor: toolExecutor,
		allTools:     allTools,
		parentModel:  parentModel,
		apiClient:    apiClient,
	}
}

// Execute runs the sub-agent with the given task prompt and returns the result.
// The sub-agent operates in its own context window with:
// - A custom system prompt from the agent config
// - Only the tools the agent is allowed to use
// - A focused task prompt (not the full conversation history)
// The result is a concise summary returned to the parent agent.
func (e *SubAgentExecutor) Execute(ctx context.Context, taskPrompt string) (*AgentResult, error) {
	// Build filtered tool definitions
	filteredTools := e.FilterTools()

	// Build the system prompt
	systemPrompt := e.BuildSystemPrompt()

	// Build messages with just the task prompt
	messages := []api.Message{
		{
			Role:    api.RoleUser,
			Content: []api.ContentBlock{api.NewTextBlock(taskPrompt)},
		},
	}

	// Determine model
	model := e.config.ResolvedModel(e.parentModel)

	// Create the API request
	req := api.Request{
		Model:     model,
		Messages:  messages,
		Tools:     filteredTools,
		MaxTokens: 16384,
		Stream:    false,
		System:    systemPrompt,
	}

	startTime := time.Now()
	result := &AgentResult{}

	// Execute the sub-agent loop
	output, toolCalls, tokensUsed, err := e.runAgentLoop(ctx, req, filteredTools)
	if err != nil {
		result.Error = err
		return result, err
	}

	result.Output = output
	result.ToolCalls = toolCalls
	result.TokensUsed = tokensUsed

	elapsed := time.Since(startTime)
	_ = elapsed // available for logging

	return result, nil
}

// runAgentLoop executes the sub-agent's agentic loop.
// If an API client is available, it runs a real agentic loop:
// call LLM → execute tools → feed results back → repeat until done.
// Otherwise falls back to a stub response.
func (e *SubAgentExecutor) runAgentLoop(ctx context.Context, req api.Request, filteredTools []api.ToolDefinition) (string, int, int, error) {
	// If no API client, return stub response
	if e.apiClient == nil {
		output := fmt.Sprintf("Sub-agent %q executed with %d available tools.\nTask: %s",
			e.config.Name,
			len(filteredTools),
			truncate(req.Messages[0].Content[0].Text, 200),
		)
		return output, 0, 0, nil
	}

	// Real agentic loop: LLM → tool calls → results → LLM → ... → end
	var totalToolCalls int
	var totalTokensUsed int
	maxIterations := 20

	for i := 0; i < maxIterations; i++ {
		resp, err := e.apiClient.SendMessage(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Sprintf("Sub-agent %q cancelled.", e.config.Name), totalToolCalls, totalTokensUsed, nil
			}
			return "", totalToolCalls, totalTokensUsed, fmt.Errorf("sub-agent %q API call failed: %w", e.config.Name, err)
		}

		totalTokensUsed += resp.Usage.InputTokens + resp.Usage.OutputTokens

		// Collect text and tool_use blocks from response
		var textParts []string
		var toolCalls []api.ContentBlock
		for _, block := range resp.Content {
			switch block.Type {
			case api.ContentText:
				if block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			case api.ContentToolUse:
				toolCalls = append(toolCalls, block)
			}
		}

		// If no tool calls, we're done — return the text
		if resp.StopReason != api.StopToolUse || len(toolCalls) == 0 {
			output := strings.Join(textParts, "\n")
			if output == "" {
				output = fmt.Sprintf("Sub-agent %q completed with no text output.", e.config.Name)
			}
			return output, totalToolCalls, totalTokensUsed, nil
		}

		// Add assistant message (with tool_use blocks) to conversation
		req.Messages = append(req.Messages, api.Message{
			Role:    api.RoleAssistant,
			Content: resp.Content,
		})

		// Execute each tool call and collect results
		var toolResults []api.ContentBlock
		for _, tc := range toolCalls {
			totalToolCalls++

			output, err := e.toolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
			if err != nil {
				toolResults = append(toolResults, api.NewToolResultBlock(tc.ID, fmt.Sprintf("Tool error: %v", err), true))
			} else {
				toolResults = append(toolResults, api.NewToolResultBlock(tc.ID, output.Content, output.IsError))
			}
		}

		// Add tool results as a user message (Anthropic API convention)
		req.Messages = append(req.Messages, api.Message{
			Role:    api.RoleUser,
			Content: toolResults,
		})
	}

	// Safety: max iterations reached
	return fmt.Sprintf("Sub-agent %q reached max iterations (%d). Partial output:\n%s",
		e.config.Name, maxIterations, truncate(fmt.Sprintf("%v", req.Messages), 500)), totalToolCalls, totalTokensUsed, nil
}

// FilterTools returns only the tool definitions that the sub-agent is allowed
// to use, based on its config.
func (e *SubAgentExecutor) FilterTools() []api.ToolDefinition {
	if len(e.config.Tools) == 0 {
		// No tools specified = inherit all tools
		return e.allTools
	}

	toolSet := make(map[string]bool)
	for _, name := range e.config.Tools {
		toolSet[name] = true
	}

	var filtered []api.ToolDefinition
	for _, td := range e.allTools {
		if toolSet[td.Name] {
			filtered = append(filtered, td)
		}
	}

	return filtered
}

// BuildSystemPrompt constructs the system prompt for the sub-agent.
func (e *SubAgentExecutor) BuildSystemPrompt() string {
	var parts []string

	parts = append(parts, e.config.Prompt)

	parts = append(parts, fmt.Sprintf("\n## Agent Identity\nYou are the %q sub-agent. You have been delegated a specific task by the parent agent.", e.config.Name))
	parts = append(parts, "You must focus ONLY on the task you have been given.")
	parts = append(parts, "Return a concise summary of your work when complete.")

	if len(e.config.Tools) > 0 {
		parts = append(parts, fmt.Sprintf("\n## Available Tools\nYou have access to: %s", strings.Join(e.config.Tools, ", ")))
		parts = append(parts, "Do NOT attempt to use tools not in this list.")
	}

	return strings.Join(parts, "\n")
}

// SubAgentTask represents a task delegated to a sub-agent.
type SubAgentTask struct {
	ID          string       `json:"id"`
	AgentName   string       `json:"agent_name"`
	Prompt      string       `json:"prompt"`
	Status      string       `json:"status"`
	Result      *AgentResult `json:"result,omitempty"`
	StartTime   time.Time    `json:"start_time"`
	EndTime     *time.Time   `json:"end_time,omitempty"`
	ParentID    string       `json:"parent_id,omitempty"`
}

// SubAgentOrchestrator manages the lifecycle of sub-agent tasks. It handles
// spawning, tracking, and collecting results from sub-agents.
type SubAgentOrchestrator struct {
	toolExecutor runtime.ToolExecutor
	allTools     []api.ToolDefinition
	parentModel  string
	apiClient    api.ProviderClient
	tasks        map[string]*SubAgentTask
	mu           sync.RWMutex
	seq          int64
}

// NewSubAgentOrchestrator creates a new orchestrator.
func NewSubAgentOrchestrator(
	toolExecutor runtime.ToolExecutor,
	allTools []api.ToolDefinition,
	parentModel string,
) *SubAgentOrchestrator {
	return &SubAgentOrchestrator{
		toolExecutor: toolExecutor,
		allTools:     allTools,
		tasks:        make(map[string]*SubAgentTask),
		parentModel:  parentModel,
	}
}

// NewSubAgentOrchestratorWithClient creates a new orchestrator with an API
// client for real LLM-backed sub-agent execution.
func NewSubAgentOrchestratorWithClient(
	toolExecutor runtime.ToolExecutor,
	allTools []api.ToolDefinition,
	parentModel string,
	apiClient api.ProviderClient,
) *SubAgentOrchestrator {
	return &SubAgentOrchestrator{
		toolExecutor: toolExecutor,
		allTools:     allTools,
		tasks:        make(map[string]*SubAgentTask),
		parentModel:  parentModel,
		apiClient:    apiClient,
	}
}

// SpawnTask creates and starts a new sub-agent task.
func (o *SubAgentOrchestrator) SpawnTask(ctx context.Context, agentName string, prompt string) (*SubAgentTask, error) {
	// Find the agent config (built-in or custom)
	config := o.resolveAgentConfig(agentName)
	if config == nil {
		return nil, fmt.Errorf("unknown sub-agent %q; available agents: %s",
			agentName, strings.Join(append(BuiltinSubAgentNames(), o.customAgentNames()...), ", "))
	}

	o.mu.Lock()
	o.seq++
	taskID := fmt.Sprintf("task-%d-%d", time.Now().UnixMilli(), o.seq)
	task := &SubAgentTask{
		ID:        taskID,
		AgentName: agentName,
		Prompt:    prompt,
		Status:    StatusPending,
		StartTime: time.Now(),
	}
	o.tasks[taskID] = task
	o.mu.Unlock()

	// Create executor — use the API client if available
	var executor *SubAgentExecutor
	if o.apiClient != nil {
		executor = NewSubAgentExecutorWithClient(config, o.toolExecutor, o.allTools, o.parentModel, o.apiClient)
	} else {
		executor = NewSubAgentExecutor(config, o.toolExecutor, o.allTools, o.parentModel)
	}

	// Run in background goroutine
	go func() {
		o.mu.Lock()
		task.Status = StatusRunning
		o.mu.Unlock()

		result, err := executor.Execute(ctx, prompt)

		o.mu.Lock()
		defer o.mu.Unlock()
		now := time.Now()
		task.EndTime = &now

		if err != nil {
			task.Status = StatusFailed
			if result == nil {
				task.Result = &AgentResult{Error: err}
			} else {
				task.Result = result
			}
		} else {
			task.Status = StatusCompleted
			task.Result = result
		}
	}()

	return task, nil
}

// GetTask returns a task by ID.
func (o *SubAgentOrchestrator) GetTask(id string) (*SubAgentTask, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	task, ok := o.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %q not found", id)
	}
	return task, nil
}

// ListTasks returns all tasks.
func (o *SubAgentOrchestrator) ListTasks() []*SubAgentTask {
	o.mu.RLock()
	defer o.mu.RUnlock()

	tasks := make([]*SubAgentTask, 0, len(o.tasks))
	for _, t := range o.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// WaitTask blocks until the task completes and returns its result.
func (o *SubAgentOrchestrator) WaitTask(id string) (*AgentResult, error) {
	// Poll until done
	for {
		task, err := o.GetTask(id)
		if err != nil {
			return nil, err
		}

		if task.Status == StatusCompleted || task.Status == StatusFailed || task.Status == StatusCancelled {
			if task.Result != nil && task.Result.Error != nil {
				return task.Result, task.Result.Error
			}
			return task.Result, nil
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// CancelTask requests cancellation of a task.
func (o *SubAgentOrchestrator) CancelTask(id string) error {
	task, err := o.GetTask(id)
	if err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if task.Status == StatusCompleted || task.Status == StatusFailed || task.Status == StatusCancelled {
		return fmt.Errorf("task %q already in terminal state: %s", id, task.Status)
	}

	task.Status = StatusCancelled
	now := time.Now()
	task.EndTime = &now
	return nil
}

// customAgentConfigs are additional configs loaded from disk.
var customConfigs []*SubAgentConfig
var customConfigsMu sync.RWMutex

// SetCustomConfigs sets the custom sub-agent configs loaded from disk.
func SetCustomConfigs(configs []*SubAgentConfig) {
	customConfigsMu.Lock()
	defer customConfigsMu.Unlock()
	customConfigs = configs
}

// GetCustomConfigs returns the custom sub-agent configs.
func GetCustomConfigs() []*SubAgentConfig {
	customConfigsMu.RLock()
	defer customConfigsMu.RUnlock()
	result := make([]*SubAgentConfig, len(customConfigs))
	copy(result, customConfigs)
	return result
}

// resolveAgentConfig finds the config for a given agent name.
func (o *SubAgentOrchestrator) resolveAgentConfig(name string) *SubAgentConfig {
	// Check built-in agents first
	if config := GetBuiltinSubAgent(name); config != nil {
		return config
	}

	// Check custom agents
	for _, config := range GetCustomConfigs() {
		if config.Name == name {
			return config
		}
	}

	return nil
}

// customAgentNames returns the names of all loaded custom agents.
func (o *SubAgentOrchestrator) customAgentNames() []string {
	configs := GetCustomConfigs()
	names := make([]string, len(configs))
	for i, c := range configs {
		names[i] = c.Name
	}
	return names
}

// AllAvailableAgents returns all available agent configs (built-in + custom).
func AllAvailableAgents() []*SubAgentConfig {
	result := make([]*SubAgentConfig, len(BuiltinSubAgents))
	copy(result, BuiltinSubAgents)
	result = append(result, GetCustomConfigs()...)
	return result
}

// AgentToolSpecs returns the tool spec names needed by the sub_agent tool.
func AgentToolSpecs() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"agent_name": {
				"type": "string",
				"description": "Name of the sub-agent to use. Available agents: Explore, Plan, Verification, code-reviewer, security-auditor, test-writer, docs-writer, refactorer, general-purpose, or any custom agent defined in .glaw/agents/"
			},
			"prompt": {
				"type": "string",
				"description": "The task description to delegate to the sub-agent. Be specific about what the agent should do."
			},
			"wait": {
				"type": "boolean",
				"description": "Whether to wait for the sub-agent to complete before returning. Default: true.",
				"default": true
			}
		},
		"required": ["agent_name", "prompt"]
	}`)
}
