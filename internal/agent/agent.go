package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// Status constants for an Agent.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Agent represents a single running (or completed) agent.
//
// An agent is created via Manager.Spawn and executes asynchronously in its own
// goroutine.  Callers can check status, wait for completion, or cancel the
// agent at any time.
type Agent struct {
	ID      string
	Type    AgentType
	Status  string
	Prompt  string
	Result  *AgentResult
	rt      *runtime.ConversationRuntime

	// mu protects the mutable fields above from concurrent access.
	mu sync.RWMutex

	// cancel is the context cancel function used to abort the agent.
	cancel context.CancelFunc

	// done is closed once the agent finishes (completed, failed, or cancelled).
	done chan struct{}

	// startTime is when the agent was created.
	startTime time.Time

	// endTime is when the agent finished.
	endTime *time.Time
}

// GetStatus returns the agent's current status in a thread-safe manner.
func (a *Agent) GetStatus() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status
}

// newAgent allocates an Agent ready for execution.  The caller must still
// call run() (typically from Manager.Spawn) to start processing.
func newAgent(id string, agentType AgentType, prompt string, rt *runtime.ConversationRuntime) *Agent {
	return &Agent{
		ID:        id,
		Type:      agentType,
		Status:    StatusPending,
		Prompt:    prompt,
		rt:        rt,
		done:      make(chan struct{}),
		startTime: time.Now(),
	}
}

// run executes the agent's work in the current goroutine.  It transitions the
// agent through the pending -> running -> completed/failed lifecycle and stores
// a placeholder result.
//
// This is a stub implementation: the agent does not yet make real LLM calls.
// It simulates work by sleeping briefly and storing a placeholder result so
// that the surrounding infrastructure (manager, REPL, commands) can be built
// and tested independently.
func (a *Agent) run(ctx context.Context) {
	a.mu.Lock()
	a.Status = StatusRunning
	a.mu.Unlock()

	// Build a placeholder result.  In the future this will be replaced with
	// real LLM interaction via a.runtime.
	result := &AgentResult{
		Output:     fmt.Sprintf("Agent %s (%s) processed prompt: %s", a.ID, a.Type, truncate(a.Prompt, 120)),
		ToolCalls:  0,
		TokensUsed: 0,
	}

	// Simulate a small amount of work so the status transitions are visible.
	select {
	case <-ctx.Done():
		a.finish(StatusCancelled, &AgentResult{Error: ctx.Err()})
		return
	case <-time.After(100 * time.Millisecond):
		// Work complete.
	}

	a.finish(StatusCompleted, result)
}

// finish transitions the agent to a terminal state and signals waiters.
func (a *Agent) finish(status string, result *AgentResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	a.Status = status
	a.Result = result
	a.endTime = &now
	close(a.done)
}

// StatusSnapshot returns a read-only snapshot of the agent's current status.
func (a *Agent) StatusSnapshot() *AgentStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()

	s := &AgentStatus{
		ID:          a.ID,
		Type:        string(a.Type),
		Status:      a.Status,
		Description: a.Prompt,
		StartTime:   a.startTime,
		EndTime:     a.endTime,
	}
	return s
}

// Done returns a channel that is closed when the agent finishes.
func (a *Agent) Done() <-chan struct{} {
	return a.done
}

// Wait blocks until the agent finishes and returns its result.
func (a *Agent) Wait() *AgentResult {
	<-a.done
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Result
}

// Cancel requests cancellation of the agent.  It is safe to call multiple
// times.
func (a *Agent) Cancel() {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cancel != nil {
		a.cancel()
	}
}

// IsTerminal returns true if the agent is in a terminal state.
func (a *Agent) IsTerminal() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Status == StatusCompleted || a.Status == StatusFailed || a.Status == StatusCancelled
}

// truncate shortens s to at most maxRunes characters, appending "..." if
// truncated.
func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes > 3 {
		return string(runes[:maxRunes-3]) + "..."
	}
	return string(runes[:maxRunes])
}
