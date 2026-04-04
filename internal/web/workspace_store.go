package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Section represents a section within a workspace.
// Sections are sub-divisions of a workspace (e.g., "frontend", "backend", "docs").
type Section struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"` // hex color for UI badge
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Workspace represents a working directory context for a project.
// Each workspace can have multiple sections to organize work.
type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`        // filesystem path (working directory)
	Description string    `json:"description,omitempty"`
	Sections    []Section `json:"sections,omitempty"`
	IsActive    bool      `json:"is_active,omitempty"` // currently active workspace
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// WorkspaceStore manages workspaces with persistence.
type WorkspaceStore struct {
	mu      sync.RWMutex
	ws      map[string]*Workspace
	seq     atomic.Int64
	fileDir string // directory for persistence
}

// NewWorkspaceStore creates a new workspace store.
// If fileDir is empty, workspaces are in-memory only.
func NewWorkspaceStore(fileDir string) *WorkspaceStore {
	s := &WorkspaceStore{
		ws:      make(map[string]*Workspace),
		fileDir: fileDir,
	}
	if fileDir != "" {
		s.load()
	}
	return s
}

// Create adds a new workspace.
func (s *WorkspaceStore) Create(name, path, description string) *Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	id := fmt.Sprintf("ws-%d", s.seq.Add(1))

	w := &Workspace{
		ID:          id,
		Name:        name,
		Path:        path,
		Description: description,
		Sections:    []Section{},
		IsActive:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.ws[id] = w
	s.save()
	return w
}

// Get returns a workspace by ID.
func (s *WorkspaceStore) Get(id string) (*Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.ws[id]
	if !ok {
		return nil, false
	}
	return w, true
}

// List returns all workspaces.
func (s *WorkspaceStore) List() []*Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Workspace, 0, len(s.ws))
	for _, w := range s.ws {
		result = append(result, w)
	}
	return result
}

// Update modifies a workspace's name, path, and/or description.
func (s *WorkspaceStore) Update(id, name, path, description string) (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.ws[id]
	if !ok {
		return nil, fmt.Errorf("workspace %s not found", id)
	}

	if name != "" {
		w.Name = name
	}
	if path != "" {
		w.Path = path
	}
	if description != "" {
		w.Description = description
	}
	w.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return w, nil
}

// Delete removes a workspace.
func (s *WorkspaceStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.ws[id]; !ok {
		return fmt.Errorf("workspace %s not found", id)
	}
	delete(s.ws, id)
	s.save()
	return nil
}

// SetActive marks a workspace as active and deactivates all others.
func (s *WorkspaceStore) SetActive(id string) (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.ws[id]
	if !ok {
		return nil, fmt.Errorf("workspace %s not found", id)
	}

	// Deactivate all
	for _, ws := range s.ws {
		ws.IsActive = false
	}
	w.IsActive = true
	w.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save()
	return w, nil
}

// GetActive returns the currently active workspace (if any).
func (s *WorkspaceStore) GetActive() *Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, w := range s.ws {
		if w.IsActive {
			return w
		}
	}
	return nil
}

// --- Section CRUD ---

// CreateSection adds a new section to a workspace.
func (s *WorkspaceStore) CreateSection(workspaceID, name, description, color string) (*Section, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.ws[workspaceID]
	if !ok {
		return nil, fmt.Errorf("workspace %s not found", workspaceID)
	}

	now := time.Now().Format(time.RFC3339)
	sectionID := fmt.Sprintf("sec-%d-%d", s.seq.Add(1), len(w.Sections)+1)

	sec := &Section{
		ID:          sectionID,
		Name:        name,
		Description: description,
		Color:       color,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	w.Sections = append(w.Sections, *sec)
	w.UpdatedAt = now
	s.save()
	return sec, nil
}

// UpdateSection modifies a section within a workspace.
func (s *WorkspaceStore) UpdateSection(workspaceID, sectionID, name, description, color string) (*Section, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.ws[workspaceID]
	if !ok {
		return nil, fmt.Errorf("workspace %s not found", workspaceID)
	}

	for i := range w.Sections {
		if w.Sections[i].ID == sectionID {
			if name != "" {
				w.Sections[i].Name = name
			}
			if description != "" {
				w.Sections[i].Description = description
			}
			if color != "" {
				w.Sections[i].Color = color
			}
			w.Sections[i].UpdatedAt = time.Now().Format(time.RFC3339)
			w.UpdatedAt = time.Now().Format(time.RFC3339)
			s.save()
			return &w.Sections[i], nil
		}
	}
	return nil, fmt.Errorf("section %s not found in workspace %s", sectionID, workspaceID)
}

// DeleteSection removes a section from a workspace.
func (s *WorkspaceStore) DeleteSection(workspaceID, sectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.ws[workspaceID]
	if !ok {
		return fmt.Errorf("workspace %s not found", workspaceID)
	}

	for i, sec := range w.Sections {
		if sec.ID == sectionID {
			w.Sections = append(w.Sections[:i], w.Sections[i+1:]...)
			w.UpdatedAt = time.Now().Format(time.RFC3339)
			s.save()
			return nil
		}
	}
	return fmt.Errorf("section %s not found in workspace %s", sectionID, workspaceID)
}

// --- Persistence ---

func (s *WorkspaceStore) load() {
	if s.fileDir == "" {
		return
	}
	path := filepath.Join(s.fileDir, "workspaces.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []*Workspace
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for _, w := range list {
		s.ws[w.ID] = w
		// Track max seq from workspace IDs
		var n int64
		if _, err := fmt.Sscanf(w.ID, "ws-%d", &n); err == nil {
			for {
				cur := s.seq.Load()
				if n <= cur || s.seq.CompareAndSwap(cur, n) {
					break
				}
			}
		}
		// Track max seq from section IDs
		for _, sec := range w.Sections {
			var sn int64
			if _, err := fmt.Sscanf(sec.ID, "sec-%d-", &sn); err == nil {
				for {
					cur := s.seq.Load()
					if sn <= cur || s.seq.CompareAndSwap(cur, sn) {
						break
					}
				}
			}
		}
	}
}

func (s *WorkspaceStore) save() {
	if s.fileDir == "" {
		return
	}
	list := make([]*Workspace, 0, len(s.ws))
	for _, w := range s.ws {
		list = append(list, w)
	}
	dir := s.fileDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "workspaces.json")
	data, _ := json.MarshalIndent(list, "", "  ")
	_ = os.WriteFile(path, data, 0o644)
}
