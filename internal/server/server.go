package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// SessionID is a unique session identifier.
type SessionID = string

// SessionStore holds active sessions.
type SessionStore struct {
	mu       sync.RWMutex
	Sessions map[SessionID]*Session
	nextID   atomic.Uint64
}

// NewSessionStore creates a new session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		Sessions: make(map[SessionID]*Session),
	}
}

// AllocateID generates a new session ID.
func (s *SessionStore) AllocateID() SessionID {
	n := s.nextID.Add(1)
	return fmt.Sprintf("session-%d", n)
}

// Session represents a server-side conversation session.
type Session struct {
	ID           string
	CreatedAt    time.Time
	Conversation *runtime.Session
	Events       *Broadcaster
}

// NewSession creates a new server session.
func NewSession(id string, conv *runtime.Session) *Session {
	return &Session{
		ID:           id,
		CreatedAt:    time.Now(),
		Conversation: conv,
		Events:       NewBroadcaster(64),
	}
}

// SessionEvent is broadcast to SSE subscribers.
type SessionEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id"`
	Data      interface{} `json:"data"`
}

// Broadcaster manages SSE subscribers.
type Broadcaster struct {
	capacity int
	mu       sync.Mutex
	subs     []chan SessionEvent
}

// NewBroadcaster creates a new broadcaster with the given channel capacity.
func NewBroadcaster(capacity int) *Broadcaster {
	return &Broadcaster{capacity: capacity}
}

// Subscribe creates a new subscription channel.
func (b *Broadcaster) Subscribe() chan SessionEvent {
	ch := make(chan SessionEvent, b.capacity)
	b.mu.Lock()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscription.
func (b *Broadcaster) Unsubscribe(ch chan SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, sub := range b.subs {
		if sub == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// Send broadcasts an event to all subscribers.
func (b *Broadcaster) Send(event SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs {
		select {
		case sub <- event:
		default:
			// Client too slow, drop event
			log.Printf("SSE: client too slow, dropping event for session %s", event.SessionID)
		}
	}
}

// API request/response types

type CreateSessionResponse struct {
	SessionID string `json:"session_id"`
}

type SessionSummary struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	MessageCount int       `json:"message_count"`
}

type ListSessionsResponse struct {
	Sessions []SessionSummary `json:"sessions"`
}

type SessionDetailsResponse struct {
	ID        string           `json:"id"`
	CreatedAt time.Time        `json:"created_at"`
	Session   *runtime.Session `json:"session"`
}

type SendMessageRequest struct {
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// Server is the HTTP server for the REST API.
type Server struct {
	store  *SessionStore
	router *http.ServeMux
}

// NewServer creates a new HTTP server.
func NewServer() *Server {
	s := &Server{
		store:  NewSessionStore(),
		router: http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) routes() {
	s.router.HandleFunc("/sessions", s.handleSessions)
	s.router.HandleFunc("/sessions/", s.handleSessionByID)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createSession(w, r)
	case http.MethodGet:
		s.listSessions(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
	}
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	path := strings.TrimPrefix(r.URL.Path, "/sessions/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]

	if len(parts) > 1 && parts[1] == "events" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
			return
		}
		s.streamEvents(w, r, sessionID)
		return
	}

	if len(parts) > 1 && parts[1] == "message" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
			return
		}
		s.sendMessage(w, r, sessionID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSession(w, r, sessionID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{Error: "Method not allowed"})
	}
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	id := s.store.AllocateID()
	session := NewSession(id, runtime.NewSession())

	s.store.mu.Lock()
	s.store.Sessions[id] = session
	s.store.mu.Unlock()

	writeJSON(w, http.StatusCreated, CreateSessionResponse{SessionID: id})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	var summaries []SessionSummary
	for _, session := range s.store.Sessions {
		summaries = append(summaries, SessionSummary{
			ID:           session.ID,
			CreatedAt:    session.CreatedAt,
			MessageCount: session.Conversation.MessageCount(),
		})
	}
	s.store.mu.RUnlock()

	if summaries == nil {
		summaries = []SessionSummary{}
	}
	writeJSON(w, http.StatusOK, ListSessionsResponse{Sessions: summaries})
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request, id string) {
	s.store.mu.RLock()
	session, ok := s.store.Sessions[id]
	s.store.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Session not found"})
		return
	}

	writeJSON(w, http.StatusOK, SessionDetailsResponse{
		ID:        session.ID,
		CreatedAt: session.CreatedAt,
		Session:   session.Conversation,
	})
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, id string) {
	s.store.mu.RLock()
	session, ok := s.store.Sessions[id]
	s.store.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Session not found"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "Streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send initial snapshot
	snapshot := SessionEvent{
		Type:      "snapshot",
		SessionID: id,
		Data:      session.Conversation,
	}
	writeSSE(w, flusher, snapshot)

	// Subscribe to events
	sub := session.Events.Subscribe()
	defer session.Events.Unsubscribe(sub)

	// Keep-alive ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-sub:
			if !ok {
				return
			}
			writeSSE(w, flusher, event)
		case <-ticker.C:
			// SSE comment as keep-alive
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request, id string) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "Invalid request body"})
		return
	}

	s.store.mu.RLock()
	session, ok := s.store.Sessions[id]
	s.store.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "Session not found"})
		return
	}

	// Add user message
	session.Conversation.AddUserMessageFromText(req.Message)

	// Broadcast the new message
	session.Events.Send(SessionEvent{
		Type:      "message",
		SessionID: id,
		Data:      req.Message,
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event SessionEvent) {
	data, _ := json.Marshal(event.Data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
	flusher.Flush()
}

// Start starts the HTTP server on the given address.
func (s *Server) Start(addr string) error {
	log.Printf("Starting server on %s", addr)
	return http.ListenAndServe(addr, s.Handler())
}

// Helper to build session directory path.
func sessionDir(baseDir string) string {
	return filepath.Join(baseDir, ".glaw", "sessions")
}

// ParseSessionID extracts session ID from a filename.
func ParseSessionID(filename string) string {
	base := filepath.Base(filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
