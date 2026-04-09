package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/commands"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// SessionInfo is a summary of a session for listing.
type SessionInfo struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	MessageCount int       `json:"message_count"`
}

// WrappedSession wraps a runtime.Session with metadata for the web layer.
type WrappedSession struct {
	ID           string
	CreatedAt    time.Time
	Conversation *runtime.Session
	Runtime      *runtime.ConversationRuntime
	Dispatcher   *commands.Dispatcher
	Cleanup      func()
}

// WebSessionStore manages sessions for the web server.
type WebSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*WrappedSession
	nextID   uint64

	// workspaceRoot is used to load sessions from the filesystem (.glaw/sessions/).
	workspaceRoot string

	// currentSessionID tracks the currently active session for save-before-switch behavior.
	currentSessionID string
}

// NewWebSessionStore creates a new session store.
func NewWebSessionStore() *WebSessionStore {
	return &WebSessionStore{
		sessions: make(map[string]*WrappedSession),
	}
}

// SetWorkspaceRoot sets the workspace root used for filesystem session lookups.
func (s *WebSessionStore) SetWorkspaceRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaceRoot = root
}

// SetCurrentSession tracks the currently active session ID.
func (s *WebSessionStore) SetCurrentSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentSessionID = id
}

// GetCurrentSession returns the currently active wrapped session, or nil.
func (s *WebSessionStore) GetCurrentSession() *WrappedSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentSessionID == "" {
		return nil
	}
	return s.sessions[s.currentSessionID]
}

// CreateSession creates a new session and returns its ID.
func (s *WebSessionStore) CreateSession() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	id := formatSessionID(s.nextID)
	s.sessions[id] = &WrappedSession{
		ID:           id,
		CreatedAt:    time.Now(),
		Conversation: runtime.NewSession(),
	}
	return id
}

// CreateSessionWithRuntime creates a new session with a live runtime and dispatcher.
func (s *WebSessionStore) CreateSessionWithRuntime(rf RuntimeFactory) (*WrappedSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	id := formatSessionID(s.nextID)
	sess := runtime.NewSession()

	ws := &WrappedSession{
		ID:           id,
		CreatedAt:    time.Now(),
		Conversation: sess,
	}

	if rf != nil {
		rt, cleanup, err := rf(sess)
		if err != nil {
			return nil, fmt.Errorf("creating runtime: %w", err)
		}
		ws.Runtime = rt
		ws.Cleanup = cleanup
		ws.Dispatcher = commands.NewDispatcher(rt)
	}

	s.sessions[id] = ws
	s.currentSessionID = id
	return ws, nil
}

// loadFromDisk attempts to load a session from the filesystem directory
// (.glaw/sessions/{id}.json) and register it in the in-memory store.
// Must be called with s.mu held (write lock).
func (s *WebSessionStore) loadFromDisk(id string) (*WrappedSession, error) {
	if s.workspaceRoot == "" {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	path := filepath.Join(s.workspaceRoot, ".glaw", "sessions", id+".json")
	session, err := runtime.LoadSession(path)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", id)
	}

	// Use the file's modification time as the CreatedAt timestamp
	var createdAt time.Time
	if info, err := os.Stat(path); err == nil {
		createdAt = info.ModTime()
	} else {
		createdAt = time.Now()
	}

	ws := &WrappedSession{
		ID:           id,
		CreatedAt:    createdAt,
		Conversation: session,
	}
	s.sessions[id] = ws
	return ws, nil
}

// EnsureRuntime lazily creates a runtime for a session that doesn't have one yet.
// If the session is not in memory, it attempts to load it from the filesystem.
func (s *WebSessionStore) EnsureRuntime(id string, rf RuntimeFactory) (*WrappedSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws, ok := s.sessions[id]
	if !ok {
		// Try loading from filesystem (e.g. sessions created by CLI)
		var err error
		ws, err = s.loadFromDisk(id)
		if err != nil {
			return nil, err
		}
	}

	if ws.Runtime != nil {
		return ws, nil
	}

	if rf == nil {
		return ws, nil
	}

	rt, cleanup, err := rf(ws.Conversation)
	if err != nil {
		return nil, fmt.Errorf("creating runtime: %w", err)
	}
	ws.Runtime = rt
	ws.Cleanup = cleanup
	ws.Dispatcher = commands.NewDispatcher(rt)

	return ws, nil
}

// GetSession returns a session by ID.
// If the session is not in memory, it attempts to load it from the filesystem.
func (s *WebSessionStore) GetSession(id string) (*WrappedSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if ok {
		return sess, true
	}

	// Try loading from filesystem
	sess, err := s.loadFromDisk(id)
	if err != nil {
		return nil, false
	}
	return sess, true
}

// ListSessions returns all session summaries, combining in-memory web sessions
// with sessions persisted on the filesystem.
func (s *WebSessionStore) ListSessions(workspaceRoot string) []SessionInfo {
	// Build a merged map: key = session ID, value = SessionInfo
	merged := make(map[string]SessionInfo)

	// 1. Add all filesystem sessions first
	if workspaceRoot != "" {
		sessionsDir := filepath.Join(workspaceRoot, ".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					name := strings.TrimSuffix(e.Name(), ".json")
					info, err := e.Info()
					if err != nil {
						continue
					}
					msgCount := 0
					data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
					if err == nil {
						var sess struct {
							Messages []struct{} `json:"messages"`
						}
						if json.Unmarshal(data, &sess) == nil {
							msgCount = len(sess.Messages)
						}
					}
					merged[name] = SessionInfo{
						ID:           name,
						CreatedAt:    info.ModTime(),
						MessageCount: msgCount,
					}
				}
			}
		}
	}

	// 2. Add/overlay in-memory web sessions.
	// Use the store key (e.g. "web-session-1") as the ID. For sessions that
	// have been saved to the filesystem, also update the filesystem entry
	// using the conversation's internal ID so counts are accurate.
	s.mu.RLock()
	for _, sess := range s.sessions {
		// Add the web session under its store key
		merged[sess.ID] = SessionInfo{
			ID:           sess.ID,
			CreatedAt:    sess.CreatedAt,
			MessageCount: sess.Conversation.MessageCount(),
		}
		// If this session was saved to disk, also update the filesystem
		// entry with live message count
		fsKey := sess.Conversation.ID
		if _, exists := merged[fsKey]; exists {
			merged[fsKey] = SessionInfo{
				ID:           fsKey,
				CreatedAt:    sess.CreatedAt,
				MessageCount: sess.Conversation.MessageCount(),
			}
		}
	}
	s.mu.RUnlock()

	// 3. Convert to sorted slice (newest first)
	result := make([]SessionInfo, 0, len(merged))
	for _, info := range merged {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result
}

// DeleteSession removes a session from the store and runs its cleanup.
// It also removes the persisted file from disk.
func (s *WebSessionStore) DeleteSession(id string, workspaceRoot string) {
	s.mu.Lock()
	if ws, ok := s.sessions[id]; ok {
		if ws.Cleanup != nil {
			ws.Cleanup()
		}
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	// Also remove from filesystem
	if workspaceRoot != "" {
		path := filepath.Join(workspaceRoot, ".glaw", "sessions", id+".json")
		os.Remove(path)
	}
}

// formatSessionID returns a formatted session ID.
func formatSessionID(n uint64) string {
	return "web-session-" + uint64ToDecimal(n)
}

func uint64ToDecimal(n uint64) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
