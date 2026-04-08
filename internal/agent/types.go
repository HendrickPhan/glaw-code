package agent

import (
	"time"
)

// AgentStatus represents the current state of an agent, returned by listing
// and inspection methods so callers never need to handle internal mutexes.
type AgentStatus struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	StartTime   time.Time  `json:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty"`
}

// AgentResult holds the output of a completed agent.
type AgentResult struct {
	Output     string `json:"output"`
	ToolCalls  int    `json:"tool_calls"`
	TokensUsed int    `json:"tokens_used"`
	Error      error  `json:"-"`
}

// AgentJob represents a background agent job tracked by the Manager.
type AgentJob struct {
	ID          string
	AgentType   string
	Prompt      string
	Status      string // pending, running, completed, failed, cancelled
	Result      *AgentResult
	StartTime   time.Time
	EndTime     *time.Time
	Foreground  bool // whether user is currently waiting on this
}

// AgentType enumerates the supported agent specialisations.
type AgentType string

const (
	// TypeGeneral is a general-purpose agent with access to all tools except
	// the agent-spawning tool itself (to avoid unbounded recursion).
	TypeGeneral AgentType = "general-purpose"

	// TypeExplore is a read-only agent that can search and read files, fetch
	// web pages, and run web searches but cannot modify anything.
	TypeExplore AgentType = "Explore"

	// TypePlan is an agent that has read-only access plus the ability to
	// write TODO / plan items.
	TypePlan AgentType = "Plan"

	// TypeVerification is an agent that has read-only access plus the ability
	// to run bash commands (primarily for executing tests).
	TypeVerification AgentType = "Verification"
)

// AllowedTools returns the set of tool names the agent type is permitted to
// use.  This is used later when wiring the agent into the tool executor.
func (t AgentType) AllowedTools() []string {
	switch t {
	case TypeExplore:
		return []string{"read_file", "glob_search", "grep_search", "web_fetch", "web_search"}
	case TypePlan:
		return []string{"read_file", "glob_search", "grep_search", "web_fetch", "web_search", "todo_write"}
	case TypeVerification:
		return []string{"read_file", "glob_search", "grep_search", "web_fetch", "web_search", "bash"}
	case TypeGeneral:
		return []string{"bash", "read_file", "write_file", "edit_file", "glob_search", "grep_search", "web_fetch", "web_search", "todo_write"}
	default:
		return []string{"bash", "read_file", "write_file", "edit_file", "glob_search", "grep_search", "web_fetch", "web_search", "todo_write"}
	}
}

// IsValid reports whether the agent type string is recognised.
func IsValidAgentType(s string) bool {
	switch AgentType(s) {
	case TypeGeneral, TypeExplore, TypePlan, TypeVerification:
		return true
	default:
		return false
	}
}
