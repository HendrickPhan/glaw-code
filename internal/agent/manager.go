package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// Manager orchestrates multiple concurrent agents.  It is safe to call all
// methods from multiple goroutines.
type Manager struct {
	agents map[string]*Agent
	jobs   map[string]*AgentJob
	mu     sync.RWMutex
	rt     *runtime.ConversationRuntime
	seq    atomic.Int64
}

// NewManager creates a new agent manager backed by the given runtime.
func NewManager(rt *runtime.ConversationRuntime) *Manager {
	return &Manager{
		agents: make(map[string]*Agent),
		jobs:   make(map[string]*AgentJob),
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

// SpawnBackground creates a new agent, starts it in a background goroutine,
// and returns a Job handle immediately without waiting.  The caller can use
// the returned AgentJob to monitor progress or wait for completion later.
func (m *Manager) SpawnBackground(ctx context.Context, prompt string, agentType string) (*AgentJob, error) {
	agent, err := m.Spawn(ctx, prompt, agentType)
	if err != nil {
		return nil, err
	}

	job := &AgentJob{
		ID:        agent.ID,
		AgentType: agentType,
		Prompt:    prompt,
		Status:    StatusPending,
		StartTime: time.Now(),
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	// Monitor the agent in a goroutine and update the job status when done.
	go func() {
		result := agent.Wait()

		m.mu.Lock()
		now := time.Now()
		job.EndTime = &now
		job.Result = result
		if result.Error != nil {
			job.Status = StatusFailed
		} else {
			job.Status = StatusCompleted
		}
		m.mu.Unlock()
	}()

	return job, nil
}

// ListJobs returns a snapshot of every tracked job.
func (m *Manager) ListJobs() []*AgentJob {
	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make([]*AgentJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

// GetJob retrieves a job by its ID.
func (m *Manager) GetJob(id string) (*AgentJob, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	j, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %q not found", id)
	}
	return j, nil
}

// WaitJob blocks until the job with the given ID finishes and returns its result.
func (m *Manager) WaitJob(id string) (*AgentResult, error) {
	// Look up the underlying agent and wait on it.
	a, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	result := a.Wait()

	// Update job status.
	m.mu.Lock()
	now := time.Now()
	if j, ok := m.jobs[id]; ok {
		j.EndTime = &now
		j.Result = result
		if result.Error != nil {
			j.Status = StatusFailed
		} else {
			j.Status = StatusCompleted
		}
	}
	m.mu.Unlock()

	if result.Error != nil {
		return result, result.Error
	}
	return result, nil
}

// CancelJob requests cancellation of the job with the given ID.
func (m *Manager) CancelJob(id string) error {
	j, err := m.GetJob(id)
	if err != nil {
		return err
	}
	if j.Status == StatusCompleted || j.Status == StatusFailed || j.Status == StatusCancelled {
		return fmt.Errorf("job %q already in terminal state: %s", id, j.Status)
	}
	m.mu.Lock()
	j.Status = StatusCancelled
	now := time.Now()
	j.EndTime = &now
	m.mu.Unlock()
	return m.Cancel(id)
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

// --- commands.AgentsProvider adapter ---

// AgentsProviderAdapter implements commands.AgentsProvider by combining
// built-in sub-agents with custom agents loaded from disk.
type AgentsProviderAdapter struct {
	mgr *Manager
}

// NewAgentsProviderAdapter creates a new adapter. If mgr is non-nil, the
// CallAgent method will use it to spawn real agents.
func NewAgentsProviderAdapter(mgr *Manager) *AgentsProviderAdapter {
	return &AgentsProviderAdapter{mgr: mgr}
}

// ListAgents returns all available agents (built-in + custom from disk).
func (a *AgentsProviderAdapter) ListAgents(workspaceRoot string) ([]commands.AgentInfo, error) {
	var result []commands.AgentInfo

	// Built-in agents
	for _, sa := range BuiltinSubAgents {
		result = append(result, commands.AgentInfo{
			Name:        sa.Name,
			Description: sa.Description,
			Source:      "builtin",
			Tools:       sa.Tools,
			Model:       sa.Model,
			Prompt:      sa.Prompt,
		})
	}

	// Custom agents from disk
	if workspaceRoot != "" {
		custom, err := LoadAllSubAgents(workspaceRoot)
		if err == nil {
			for _, sa := range custom {
				source := "project"
				if sa.Level == "user" {
					source = "user"
				}
				result = append(result, commands.AgentInfo{
					Name:        sa.Name,
					Description: sa.Description,
					Source:      source,
					Tools:       sa.Tools,
					Model:       sa.Model,
					Prompt:      sa.Prompt,
				})
			}
		}
	}

	return result, nil
}

// GetAgent returns detailed information about a specific agent by name.
func (a *AgentsProviderAdapter) GetAgent(workspaceRoot, name string) (*commands.AgentInfo, error) {
	// Check built-in agents
	for _, sa := range BuiltinSubAgents {
		if sa.Name == name {
			return &commands.AgentInfo{
				Name:        sa.Name,
				Description: sa.Description,
				Source:      "builtin",
				Tools:       sa.Tools,
				Model:       sa.Model,
				Prompt:      sa.Prompt,
			}, nil
		}
	}

	// Check custom agents from disk
	if workspaceRoot != "" {
		custom, err := LoadAllSubAgents(workspaceRoot)
		if err == nil {
			for _, sa := range custom {
				if sa.Name == name {
					source := "project"
					if sa.Level == "user" {
						source = "user"
					}
					return &commands.AgentInfo{
						Name:        sa.Name,
						Description: sa.Description,
						Source:      source,
						Tools:       sa.Tools,
						Model:       sa.Model,
						Prompt:      sa.Prompt,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("agent %q not found", name)
}

// CreateAgent creates a new custom agent by writing a markdown file to the
// appropriate agents directory (.glaw/agents/ or ~/.glaw/agents/).
func (a *AgentsProviderAdapter) CreateAgent(workspaceRoot, name, description, scope string, tools []string, model, prompt string) error {
	if name == "" {
		return fmt.Errorf("agent name is required")
	}

	var dir string
	var err error
	switch scope {
	case "user", "global":
		dir, err = EnsureUserAgentsDir()
		if err != nil {
			return err
		}
	default: // "project"
		if workspaceRoot == "" {
			return fmt.Errorf("workspace root is required for project-scoped agents")
		}
		dir, err = EnsureAgentsDir(workspaceRoot)
		if err != nil {
			return err
		}
	}

	config := &SubAgentConfig{
		Name:        name,
		Description: description,
		Tools:       tools,
		Model:       model,
		Prompt:      prompt,
	}

	return CreateSubAgentFile(dir, config)
}

// DeleteAgent removes a custom agent's markdown file from disk.
func (a *AgentsProviderAdapter) DeleteAgent(workspaceRoot, name, scope string) error {
	if name == "" {
		return fmt.Errorf("agent name is required")
	}

	var path string
	switch scope {
	case "user", "global":
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("getting home directory: %w", err)
		}
		path = filepath.Join(home, ".glaw", "agents", name+".md")
	default: // "project"
		if workspaceRoot == "" {
			return fmt.Errorf("workspace root is required for project-scoped agents")
		}
		path = filepath.Join(workspaceRoot, ".glaw", "agents", name+".md")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent %q not found at %s", name, path)
	}

	return os.Remove(path)
}

// CallAgent runs a sub-agent by name with the given prompt and returns the
// output as a string. It uses the Manager if available, otherwise returns
// an error. This blocks until the agent completes.
func (a *AgentsProviderAdapter) CallAgent(ctx context.Context, name string, prompt string) (string, error) {
	if a.mgr == nil {
		return "", fmt.Errorf("agent manager not available; cannot call agents from the CLI")
	}

	agent, err := a.mgr.Spawn(ctx, prompt, name)
	if err != nil {
		return "", fmt.Errorf("spawning agent %q: %w", name, err)
	}

	result := agent.Wait()
	if result.Error != nil {
		return "", fmt.Errorf("agent %q failed: %w", name, result.Error)
	}

	return result.Output, nil
}

// CallAgentBackground spawns a sub-agent in the background and returns the
// job ID immediately without waiting for completion.
func (a *AgentsProviderAdapter) CallAgentBackground(ctx context.Context, name string, prompt string) (string, error) {
	if a.mgr == nil {
		return "", fmt.Errorf("agent manager not available; cannot call agents from the CLI")
	}

	job, err := a.mgr.SpawnBackground(ctx, prompt, name)
	if err != nil {
		return "", fmt.Errorf("spawning agent %q: %w", name, err)
	}

	return job.ID, nil
}

// GetAgentJobStatus returns the current status of a background agent job.
func (a *AgentsProviderAdapter) GetAgentJobStatus(jobID string) (*commands.AgentJobStatus, error) {
	if a.mgr == nil {
		return nil, fmt.Errorf("agent manager not available")
	}
	job, err := a.mgr.GetJob(jobID)
	if err != nil {
		return nil, err
	}
	return &commands.AgentJobStatus{
		ID:        job.ID,
		AgentType: job.AgentType,
		Status:    job.Status,
		Prompt:    job.Prompt,
		StartTime: job.StartTime,
		EndTime:   job.EndTime,
	}, nil
}

// ListAgentJobs returns all tracked background agent jobs.
func (a *AgentsProviderAdapter) ListAgentJobs() []*commands.AgentJobStatus {
	if a.mgr == nil {
		return nil
	}
	jobs := a.mgr.ListJobs()
	result := make([]*commands.AgentJobStatus, len(jobs))
	for i, j := range jobs {
		result[i] = &commands.AgentJobStatus{
			ID:        j.ID,
			AgentType: j.AgentType,
			Status:    j.Status,
			Prompt:    j.Prompt,
			StartTime: j.StartTime,
			EndTime:   j.EndTime,
		}
	}
	return result
}

// WaitAgentJob blocks until the given job completes and returns its output.
func (a *AgentsProviderAdapter) WaitAgentJob(jobID string) (string, error) {
	if a.mgr == nil {
		return "", fmt.Errorf("agent manager not available")
	}
	result, err := a.mgr.WaitJob(jobID)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// CancelAgentJob cancels a running background agent job.
func (a *AgentsProviderAdapter) CancelAgentJob(jobID string) error {
	if a.mgr == nil {
		return fmt.Errorf("agent manager not available")
	}
	return a.mgr.CancelJob(jobID)
}

// --- runtime.SubAgentSessionProvider adapter ---

// SubAgentSessionAdapter implements runtime.SubAgentSessionProvider by
// querying the agent.Manager for active/completed agents and their sessions.
type SubAgentSessionAdapter struct {
	mgr *Manager
}

// NewSubAgentSessionAdapter creates a new adapter wrapping the given manager.
func NewSubAgentSessionAdapter(mgr *Manager) *SubAgentSessionAdapter {
	return &SubAgentSessionAdapter{mgr: mgr}
}

// ListAgentStatuses returns metadata for every tracked sub-agent.
func (a *SubAgentSessionAdapter) ListAgentStatuses() []commands.SubAgentSessionInfo {
	if a.mgr == nil {
		return nil
	}
	statuses := a.mgr.List()
	var result []commands.SubAgentSessionInfo
	for _, s := range statuses {
		prompt := s.Description
		if len(prompt) > 100 {
			prompt = prompt[:97] + "..."
		}
		result = append(result, commands.SubAgentSessionInfo{
			AgentID:   s.ID,
			AgentType: s.Type,
			Status:    s.Status,
			Prompt:    prompt,
		})
	}
	return result
}

// LoadAgentSession loads the session file associated with an agent.
// Currently returns an error since sub-agents don't persist sessions to disk.
// This is a placeholder for future session persistence support.
func (a *SubAgentSessionAdapter) LoadAgentSession(agentID string) (string, int, error) {
	if a.mgr == nil {
		return "", 0, fmt.Errorf("no agent manager configured")
	}
	agent, err := a.mgr.Get(agentID)
	if err != nil {
		return "", 0, fmt.Errorf("agent %q not found: %w", agentID, err)
	}
	// Sub-agents currently run in-memory; return placeholder info.
	// When session persistence is added, this will load the session from disk.
	return agent.ID, 0, nil
}
