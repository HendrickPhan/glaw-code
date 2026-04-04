package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

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
	workspaces     *WorkspaceStore
	runtimeFactory RuntimeFactory
	staticFS       fs.FS
}

// NewWebServer creates a new web server.
func NewWebServer(rf RuntimeFactory) *WebServer {
	// Strip the "static/" prefix from the embedded FS so that
	// requests for "/_next/..." map to "static/_next/..." correctly.
	sub, err := fs.Sub(webUI, "static")
	if err != nil {
		panic("failed to create sub FS: " + err.Error())
	}

	// Try to persist workspaces in the global config directory
	wsDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		wsDir = filepath.Join(home, ".glaw")
	}

	return &WebServer{
		store:          NewWebSessionStore(),
		workspaces:     NewWorkspaceStore(wsDir),
		runtimeFactory: rf,
		staticFS:       sub,
	}
}

// Handler returns the root HTTP handler with all routes wired in.
func (s *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.HandleWebSocket)

	// REST API endpoints — Sessions
	mux.HandleFunc("/api/sessions", s.handleAPISessions)
	mux.HandleFunc("/api/sessions/", s.handleAPISessionByID)

	// REST API endpoints — Workspaces
	mux.HandleFunc("/api/workspaces", s.handleAPIWorkspaces)
	mux.HandleFunc("/api/workspaces/", s.handleAPIWorkspaceByID)

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
		sessions := s.store.ListSessions()
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

// --- Workspace REST API Handlers ---

// handleAPIWorkspaces handles POST (create) and GET (list) for /api/workspaces.
func (s *WebServer) handleAPIWorkspaces(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workspaces := s.workspaces.List()
		writeJSON(w, http.StatusOK, map[string]interface{}{"workspaces": workspaces})

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		ws := s.workspaces.Create(req.Name, req.Path, req.Description)
		writeJSON(w, http.StatusCreated, ws)

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPIWorkspaceByID handles GET, PUT, DELETE for /api/workspaces/{id}
// and section sub-routes: /api/workspaces/{id}/sections, /api/workspaces/{id}/sections/{sectionId}
func (s *WebServer) handleAPIWorkspaceByID(w http.ResponseWriter, r *http.Request) {
	// Extract the full remaining path after "/api/workspaces/"
	prefix := "/api/workspaces/"
	if len(r.URL.Path) <= len(prefix) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing workspace ID"})
		return
	}
	remaining := r.URL.Path[len(prefix):]

	// Parse the workspace ID and optional sub-path
	id := remaining
	subPath := ""
	if idx := indexOfSlash(remaining); idx >= 0 {
		id = remaining[:idx]
		subPath = remaining[idx:] // includes leading "/"
	}

	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing workspace ID"})
		return
	}

	// Route to section handlers if subPath starts with "/sections"
	if subPath == "/sections" {
		s.handleAPISections(w, r, id, "")
		return
	}
	if len(subPath) > len("/sections") && subPath[:len("/sections")] == "/sections" {
		s.handleAPISections(w, r, id, subPath[len("/sections"):])
		return
	}

	// Handle /activate sub-route
	if subPath == "/activate" {
		if r.Method == http.MethodPost {
			ws, err := s.workspaces.SetActive(id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, ws)
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Unknown sub-path
	if subPath != "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		ws, ok := s.workspaces.Get(id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
			return
		}
		writeJSON(w, http.StatusOK, ws)

	case http.MethodPut:
		var req struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		ws, err := s.workspaces.Update(id, req.Name, req.Path, req.Description)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ws)

	case http.MethodDelete:
		if err := s.workspaces.Delete(id); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// handleAPISections handles section CRUD for a workspace.
func (s *WebServer) handleAPISections(w http.ResponseWriter, r *http.Request, workspaceID, sectionPath string) {
	switch r.Method {
	case http.MethodGet:
		// GET /api/workspaces/{id}/sections — list sections
		ws, ok := s.workspaces.Get(workspaceID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "workspace not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"sections": ws.Sections})

	case http.MethodPost:
		// POST /api/workspaces/{id}/sections — create section
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Color       string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		sec, err := s.workspaces.CreateSection(workspaceID, req.Name, req.Description, req.Color)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, sec)

	case http.MethodPut:
		// PUT /api/workspaces/{id}/sections/{sectionId} — update section
		sectionID := extractSectionID(sectionPath)
		if sectionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing section ID"})
			return
		}
		var req struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Color       string `json:"color"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		sec, err := s.workspaces.UpdateSection(workspaceID, sectionID, req.Name, req.Description, req.Color)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, sec)

	case http.MethodDelete:
		// DELETE /api/workspaces/{id}/sections/{sectionId} — delete section
		sectionID := extractSectionID(sectionPath)
		if sectionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing section ID"})
			return
		}
		if err := s.workspaces.DeleteSection(workspaceID, sectionID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// extractSectionID extracts section ID from a path like "/sec-1-1".
func extractSectionID(path string) string {
	if len(path) == 0 || path[0] != '/' {
		return ""
	}
	return path[1:]
}

// indexOfSlash returns the index of the first '/' in s, or -1.
func indexOfSlash(s string) int {
	for i, c := range s {
		if c == '/' {
			return i
		}
	}
	return -1
}
