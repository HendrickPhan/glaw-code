package web

import (
	"sync"
	"time"

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
}

// WebSessionStore manages sessions for the web server.
type WebSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*WrappedSession
	nextID   uint64
}

// NewWebSessionStore creates a new session store.
func NewWebSessionStore() *WebSessionStore {
	return &WebSessionStore{
		sessions: make(map[string]*WrappedSession),
	}
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

// GetSession returns a session by ID.
func (s *WebSessionStore) GetSession(id string) (*WrappedSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// ListSessions returns all session summaries.
func (s *WebSessionStore) ListSessions() []SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]SessionInfo, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, SessionInfo{
			ID:           sess.ID,
			CreatedAt:    sess.CreatedAt,
			MessageCount: sess.Conversation.MessageCount(),
		})
	}
	return result
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
