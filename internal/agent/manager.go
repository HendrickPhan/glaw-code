package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// Manager orchestrates multiple concurrent agents.  It is safe to call all
// methods from multiple goroutines.
type Manager struct {
	agents map[string]*Agent
	mu     sync.RWMutex
	rt     *runtime.ConversationRuntime
	seq    atomic.Int64
}

// NewManager creates a new agent manager backed by the given runtime.
func NewManager(rt *runtime.ConversationRuntime) *Manager {
	return &Manager{
		agents: make(map[string]*Agent),
		rt:     rt,
	}
}

// Spawn creates a new agent of the given type, starts it in a background
// goroutine, and returns it.  The returned Agent can be used to monitor
// progress or wait for completion.
//
// agentType must be one of the recognised AgentType values (e.g.
// "general-purpose", "Explore", "Plan", "Verification").  The returned agent
// ID is unique and has the form "agent-<unix-millis>-<seq>".
func (m *Manager) Spawn(ctx context.Context, prompt string, agentType string) (*Agent, error) {
	at := AgentType(agentType)
	if !IsValidAgentType(agentType) {
		return nil, fmt.Errorf("unknown agent type %q; valid types: general-purpose, Explore, Plan, Verification", agentType)
	}

	id := fmt.Sprintf("agent-%d-%d", time.Now().UnixMilli(), m.seq.Add(1))

	agentCtx, cancel := context.WithCancel(ctx)
	a := newAgent(id, at, prompt, m.rt)
	a.cancel = cancel

	m.mu.Lock()
	m.agents[id] = a
	m.mu.Unlock()

	go func() {
		a.run(agentCtx)
	}()

	return a, nil
}

// List returns a snapshot of every agent managed by this Manager.
func (m *Manager) List() []*AgentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]*AgentStatus, 0, len(m.agents))
	for _, a := range m.agents {
		statuses = append(statuses, a.StatusSnapshot())
	}
	return statuses
}

// Get retrieves an agent by its ID.  Returns an error if no agent with that ID
// exists.
func (m *Manager) Get(id string) (*Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	a, ok := m.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", id)
	}
	return a, nil
}

// SendToBackground marks the agent as a background task.  This is purely
// informational for now (the agent is already running in a goroutine); it
// serves as a hook for future UI integration.
func (m *Manager) SendToBackground(id string) error {
	a, err := m.Get(id)
	if err != nil {
		return err
	}
	if a.IsTerminal() {
		return fmt.Errorf("agent %q is already in a terminal state: %s", id, a.Status)
	}
	// No-op for now; future work could detach the agent from the foreground
	// output stream.
	return nil
}

// BringToFront brings a background agent to the foreground and blocks until it
// completes, returning its result.
func (m *Manager) BringToFront(id string) (*AgentResult, error) {
	a, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	result := a.Wait()
	if result.Error != nil {
		return result, result.Error
	}
	return result, nil
}

// Cancel requests cancellation of the agent with the given ID.  It is safe to
// call for agents that have already finished.
func (m *Manager) Cancel(id string) error {
	a, err := m.Get(id)
	if err != nil {
		return err
	}
	a.Cancel()
	return nil
}

// Wait blocks until the agent with the given ID finishes and returns its
// result.
func (m *Manager) Wait(id string) (*AgentResult, error) {
	a, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	result := a.Wait()
	if result.Error != nil {
		return result, result.Error
	}
	return result, nil
}

// Shutdown cancels every running agent and waits for them all to finish.
func (m *Manager) Shutdown() {
	m.mu.RLock()
	agents := make([]*Agent, 0, len(m.agents))
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	m.mu.RUnlock()

	for _, a := range agents {
		a.Cancel()
	}
	for _, a := range agents {
		a.Wait()
	}
}
