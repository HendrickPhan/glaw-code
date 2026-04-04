package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Status constants for tasks.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusDeleted    = "deleted"
)

// Task represents a single task.
type Task struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner,omitempty"`
	Blocks      []string `json:"blocks,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// Store manages tasks with persistence.
type Store struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	seq     atomic.Int64
	filePath string
}

// NewStore creates a new task store. If filePath is empty, tasks are in-memory only.
func NewStore(filePath string) *Store {
	s := &Store{
		tasks:    make(map[string]*Task),
		filePath: filePath,
	}
	if filePath != "" {
		s.load()
	}
	return s
}

// Create creates a new task and returns it.
func (s *Store) Create(subject string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	id := fmt.Sprintf("%d", s.seq.Add(1))

	t := &Task{
		ID:        id,
		Subject:   subject,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.tasks[id] = t
	s.save()
	return t
}

// List returns all non-deleted tasks.
func (s *Store) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if t.Status != StatusDeleted {
			result = append(result, t)
		}
	}
	return result
}

// Get returns a task by ID.
func (s *Store) Get(id string) (*Task, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tasks[id]
	if !ok || t.Status == StatusDeleted {
		return nil, false
	}
	return t, true
}

// Update modifies a task's status and/or description.
func (s *Store) Update(id string, status string, description string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok || t.Status == StatusDeleted {
		return nil, fmt.Errorf("task %s not found", id)
	}

	if status != "" {
		switch status {
		case StatusPending, StatusInProgress, StatusCompleted, StatusDeleted:
			t.Status = status
		default:
			return nil, fmt.Errorf("invalid status %q; valid: pending, in_progress, completed, deleted", status)
		}
	}
	if description != "" {
		t.Description = description
	}
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return t, nil
}

// Delete removes a task (soft-delete).
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	t.Status = StatusDeleted
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return nil
}

// SetDependencies sets the blocks/blockedBy relationships for a task.
func (s *Store) SetDependencies(id string, blocks []string, blockedBy []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[id]
	if !ok || t.Status == StatusDeleted {
		return fmt.Errorf("task %s not found", id)
	}
	t.Blocks = blocks
	t.BlockedBy = blockedBy
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return nil
}

func (s *Store) load() {
	if s.filePath == "" {
		return
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var list []*Task
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for _, t := range list {
		s.tasks[t.ID] = t
		// Track max seq
		var n int64
		if _, err := fmt.Sscanf(t.ID, "%d", &n); err != nil {
			continue
		}
		for {
			cur := s.seq.Load()
			if n <= cur || s.seq.CompareAndSwap(cur, n) {
				break
			}
		}
	}
}

func (s *Store) save() {
	if s.filePath == "" {
		return
	}
	list := make([]*Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		list = append(list, t)
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	if err := os.WriteFile(s.filePath, data, 0o644); err != nil {
		return
	}
}
