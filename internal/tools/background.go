package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// CommandStatus represents the status of a background command.
type CommandStatus string

const (
	StatusPending   CommandStatus = "pending"
	StatusRunning   CommandStatus = "running"
	StatusCompleted CommandStatus = "completed"
	StatusFailed    CommandStatus = "failed"
	StatusCancelled CommandStatus = "cancelled"
)

// BackgroundCommand represents a running or completed background bash command.
type BackgroundCommand struct {
	ID        string
	Command   string
	Status    CommandStatus
	StartTime time.Time
	EndTime   *time.Time
	Output    string
	Error     error
	cancel    context.CancelFunc
	process   *os.Process
	mu        sync.RWMutex
}

// IsDone returns true if the command is in a terminal state.
func (c *BackgroundCommand) IsDone() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status == StatusCompleted || c.Status == StatusFailed || c.Status == StatusCancelled
}

// GetOutput returns the current output of the command.
func (c *BackgroundCommand) GetOutput() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Output
}

// BackgroundCommandManager manages background bash commands.
type BackgroundCommandManager struct {
	commands map[string]*BackgroundCommand
	mu       sync.RWMutex
	seq      atomic.Int64
	workspaceRoot string
}

// NewBackgroundCommandManager creates a new manager.
func NewBackgroundCommandManager(workspaceRoot string) *BackgroundCommandManager {
	return &BackgroundCommandManager{
		commands:     make(map[string]*BackgroundCommand),
		workspaceRoot: workspaceRoot,
	}
}

// Spawn starts a command in the background and returns its ID immediately.
func (m *BackgroundCommandManager) Spawn(command string, timeout time.Duration, workspaceRoot string) (string, error) {
	id := fmt.Sprintf("bash-%d-%d", time.Now().UnixMilli(), m.seq.Add(1))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workspaceRoot

	// Check if the command needs terminal access
	needsTerminal := isInteractiveCommand(command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	bgCmd := &BackgroundCommand{
		ID:        id,
		Command:   command,
		Status:    StatusPending,
		StartTime: time.Now(),
		cancel:    cancel,
	}

	m.mu.Lock()
	m.commands[id] = bgCmd
	m.mu.Unlock()

	go func() {
		bgCmd.mu.Lock()
		bgCmd.Status = StatusRunning
		bgCmd.mu.Unlock()

		var err error
		if needsTerminal {
			// For interactive commands, connect to terminal
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			// Clear output for interactive commands since it goes to terminal
			bgCmd.mu.Lock()
			bgCmd.Output = "(interactive command - output sent to terminal)"
			bgCmd.mu.Unlock()
		} else {
			err = cmd.Run()
			output := stdout.String()
			if stderr.Len() > 0 {
				output += "\n" + stderr.String()
			}
			bgCmd.mu.Lock()
			bgCmd.Output = output
			bgCmd.mu.Unlock()
		}

		now := time.Now()
		bgCmd.mu.Lock()
		bgCmd.EndTime = &now

		if ctx.Err() == context.DeadlineExceeded {
			bgCmd.Status = StatusFailed
			bgCmd.Error = fmt.Errorf("command timed out after %v", timeout)
		} else if err != nil {
			bgCmd.Status = StatusFailed
			bgCmd.Error = err
		} else {
			bgCmd.Status = StatusCompleted
		}
		bgCmd.mu.Unlock()
	}()

	return id, nil
}

// Get retrieves a command by its ID.
func (m *BackgroundCommandManager) Get(id string) (*BackgroundCommand, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cmd, ok := m.commands[id]
	if !ok {
		return nil, fmt.Errorf("command %q not found", id)
	}
	return cmd, nil
}

// Wait blocks until the command finishes and returns its result.
func (m *BackgroundCommandManager) Wait(id string, timeout time.Duration) (*BackgroundCommand, error) {
	cmd, err := m.Get(id)
	if err != nil {
		return nil, err
	}

	// If already done, return immediately
	if cmd.IsDone() {
		return cmd, nil
	}

	// Poll for completion
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ticker.C:
			if cmd.IsDone() {
				return cmd, nil
			}
		case <-timeoutTimer.C:
			return cmd, fmt.Errorf("wait timed out")
		}
	}
}

// Stop cancels a running command.
func (m *BackgroundCommandManager) Stop(id string) error {
	m.mu.Lock()
	cmd, ok := m.commands[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("command %q not found", id)
	}

	cmd.mu.Lock()
	defer cmd.mu.Unlock()

	if cmd.IsDone() {
		return fmt.Errorf("command %q already in terminal state: %s", id, cmd.Status)
	}

	cmd.Status = StatusCancelled
	now := time.Now()
	cmd.EndTime = &now
	if cmd.cancel != nil {
		cmd.cancel()
	}

	return nil
}

// List returns a snapshot of all background commands.
func (m *BackgroundCommandManager) List() []*BackgroundCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cmds := make([]*BackgroundCommand, 0, len(m.commands))
	for _, cmd := range m.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}
