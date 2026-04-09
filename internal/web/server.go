package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

//go:embed static/*
var webUI embed.FS

// RuntimeFactory creates a conversation runtime for a given session.
// Returns the runtime, a cleanup function, and any error.
type RuntimeFactory func(sess *runtime.Session) (*runtime.ConversationRuntime, func(), error)

// WebServer is the HTTP/WebSocket server for the web UI.
type WebServer struct {
	store          *WebSessionStore
	runtimeFactory RuntimeFactory
	staticFS       fs.FS
	workspaceRoot  string
}

// NewWebServer creates a new web server.
func NewWebServer(rf RuntimeFactory, workspaceRoot string) *WebServer {
	// Strip the "static/" prefix from the embedded FS so that
	// requests for "/_next/..." map to "static/_next/..." correctly.
	sub, err := fs.Sub(webUI, "static")
	if err != nil {
		panic("failed to create sub FS: " + err.Error())
	}
	store := NewWebSessionStore()
	store.SetWorkspaceRoot(workspaceRoot)

	return &WebServer{
		store:          store,
		runtimeFactory: rf,
		staticFS:       sub,
		workspaceRoot:  workspaceRoot,
	}
}

// Handler returns the root HTTP handler with all routes wired in.
func (s *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.HandleWebSocket)

	// REST API endpoints
	mux.HandleFunc("/api/sessions", s.handleAPISessions)
	mux.HandleFunc("/api/sessions/", s.handleAPISessionByID)

	// Static files (embedded UI)
	mux.Handle("/", http.FileServer(http.FS(s.staticFS)))

	// Wrap with CORS
	return withCORS(mux)
}

// withCORS wraps a handler with CORS headers.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleAPISessions handles POST (create) and GET (list) for /api/sessions.
func (s *WebServer) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		id := s.store.CreateSession()
		writeJSON(w, http.StatusCreated, map[string]string{"session_id": id})
	case http.MethodGet:
		sessions := s.store.ListSessions(s.workspaceRoot)
		writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": sessions})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPISessionByID handles GET for /api/sessions/{id}.
func (s *WebServer) handleAPISessionByID(w http.ResponseWriter, r *http.Request) {
	id := extractSessionID(r.URL.Path, "/api/sessions/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		sess, ok := s.store.GetSession(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":         sess.ID,
			"created_at": sess.CreatedAt,
			"session":    sess.Conversation,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// extractSessionID extracts the session ID from a URL path.
func extractSessionID(path, prefix string) string {
	if len(path) <= len(prefix) {
		return ""
	}
	remaining := path[len(prefix):]
	for i, c := range remaining {
		if c == '/' {
			return remaining[:i]
		}
	}
	return remaining
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
