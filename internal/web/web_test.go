package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSessionStoreCreate(t *testing.T) {
	store := NewWebSessionStore()
	id := store.CreateSession()

	if id == "" {
		t.Error("expected non-empty session ID")
	}
	if !strings.HasPrefix(id, "web-session-") {
		t.Errorf("ID = %q, expected to start with web-session-", id)
	}
}

func TestWebSessionStoreGet(t *testing.T) {
	store := NewWebSessionStore()
	id := store.CreateSession()

	sess, ok := store.GetSession(id)
	if !ok {
		t.Error("expected to find session")
	}
	if sess.ID != id {
		t.Errorf("session ID = %q, want %q", sess.ID, id)
	}
	if sess.Conversation == nil {
		t.Error("expected Conversation to be non-nil")
	}
}

func TestWebSessionStoreGetNotFound(t *testing.T) {
	store := NewWebSessionStore()
	_, ok := store.GetSession("nonexistent")
	if ok {
		t.Error("expected session not found")
	}
}

func TestWebSessionStoreList(t *testing.T) {
	store := NewWebSessionStore()
	id1 := store.CreateSession()
	id2 := store.CreateSession()

	sessions := store.ListSessions("")
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids[id1] || !ids[id2] {
		t.Errorf("expected both session IDs in list, got %v", sessions)
	}
}

func TestWebSessionStoreListEmpty(t *testing.T) {
	store := NewWebSessionStore()
	sessions := store.ListSessions("")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestFormatSessionID(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{1, "web-session-1"},
		{10, "web-session-10"},
		{123, "web-session-123"},
	}
	for _, tt := range tests {
		got := formatSessionID(tt.n)
		if got != tt.want {
			t.Errorf("formatSessionID(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestUint64ToDecimal(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{999, "999"},
	}
	for _, tt := range tests {
		got := uint64ToDecimal(tt.n)
		if got != tt.want {
			t.Errorf("uint64ToDecimal(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestNewWebServer(t *testing.T) {
	srv := NewWebServer(nil, "")
	if srv == nil {
		t.Fatal("expected non-nil server")
		return
	}
	if srv.store == nil {
		t.Error("expected non-nil store")
	}
	if srv.Handler() == nil {
		t.Error("expected non-nil handler")
	}
}

func TestWebServerHandlerRoutes(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// GET / should serve index.html
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// GET /api/sessions (empty)
	resp, err = http.Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/sessions status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// POST /api/sessions
	resp, err = http.Post(ts.URL+"/api/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/sessions error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /api/sessions status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	var createResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if createResp["session_id"] == "" {
		t.Error("expected session_id in response")
	}
}

func TestWebServerCORS(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// OPTIONS preflight
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/sessions", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("CORS Origin header missing or wrong: %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS Methods header missing")
	}
}

func TestWebServerGetSessionNotFound(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/sessions/nonexistent")
	if err != nil {
		t.Fatalf("GET /api/sessions/nonexistent error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /api/sessions/nonexistent status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestWebServerSessionListAfterCreate(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create two sessions
	for i := 0; i < 2; i++ {
		resp, err := http.Post(ts.URL+"/api/sessions", "application/json", nil)
		if err != nil {
			t.Fatalf("POST /api/sessions error: %v", err)
		}
		resp.Body.Close()
	}

	// List sessions
	resp, err := http.Get(ts.URL + "/api/sessions")
	if err != nil {
		t.Fatalf("GET /api/sessions error: %v", err)
	}
	defer resp.Body.Close()

	var listResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		sessions, ok := listResp["sessions"].([]interface{})
	if !ok {
		t.Fatalf("expected sessions array, got %T", listResp["sessions"])
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   string
	}{
		{"/api/sessions/abc123", "/api/sessions/", "abc123"},
		{"/api/sessions/abc123/messages", "/api/sessions/", "abc123"},
		{"/api/sessions/", "/api/sessions/", ""},
		{"/api/sessions", "/api/sessions/", ""},
	}
	for _, tt := range tests {
		got := extractSessionID(tt.path, tt.prefix)
		if got != tt.want {
			t.Errorf("extractSessionID(%q, %q) = %q, want %q", tt.path, tt.prefix, got, tt.want)
		}
	}
}

func TestWebServerAPIDisallowedMethods(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// DELETE /api/sessions (not allowed)
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/sessions", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/sessions status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestWebServerCreateAndGetSession(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create
	resp, err := http.Post(ts.URL+"/api/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/sessions error: %v", err)
	}
	var createResp map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		resp.Body.Close()

	sessionID := createResp["session_id"]
	if sessionID == "" {
		t.Fatal("expected session_id")
	}

	// Get
	resp, err = http.Get(ts.URL + "/api/sessions/" + sessionID)
	if err != nil {
		t.Fatalf("GET /api/sessions/%s error: %v", sessionID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET session status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var details map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if details["id"] != sessionID {
		t.Errorf("session id = %v, want %q", details["id"], sessionID)
	}
}

func TestStaticFilesServed(t *testing.T) {
	srv := NewWebServer(nil, "")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, expected text/html", ct)
	}
}
