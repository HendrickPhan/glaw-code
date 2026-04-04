package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer()
	if s.store == nil {
		t.Error("store should not be nil")
	}
}

func TestSessionStoreAllocateID(t *testing.T) {
	store := NewSessionStore()
	id1 := store.AllocateID()
	id2 := store.AllocateID()
	if id1 == id2 {
		t.Errorf("IDs should be unique: %q == %q", id1, id2)
	}
	if !strings.HasPrefix(id1, "session-") {
		t.Errorf("ID = %q, should start with session-", id1)
	}
}

func TestCreateSession(t *testing.T) {
	s := NewServer()
	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	w := httptest.NewRecorder()
	s.createSession(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp CreateSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
}

func TestListSessionsEmpty(t *testing.T) {
	s := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	s.listSessions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp ListSessionsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(resp.Sessions) != 0 {
		t.Errorf("Sessions = %d, want 0", len(resp.Sessions))
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := NewServer()
	req := httptest.NewRequest(http.MethodGet, "/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	s.getSession(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCreateAndGetSession(t *testing.T) {
	s := NewServer()

	// Create
	createReq := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	createW := httptest.NewRecorder()
	s.createSession(createW, createReq)

	var createResp CreateSessionResponse
	json.NewDecoder(createW.Body).Decode(&createResp)

	// Get
	getReq := httptest.NewRequest(http.MethodGet, "/sessions/"+createResp.SessionID, nil)
	getW := httptest.NewRecorder()
	s.getSession(getW, getReq, createResp.SessionID)

	if getW.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", getW.Code, http.StatusOK)
	}

	var details SessionDetailsResponse
	json.NewDecoder(getW.Body).Decode(&details)
	if details.ID != createResp.SessionID {
		t.Errorf("ID = %q, want %q", details.ID, createResp.SessionID)
	}
}

func TestListSessionsAfterCreate(t *testing.T) {
	s := NewServer()

	// Create two sessions
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
		w := httptest.NewRecorder()
		s.createSession(w, req)
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	s.listSessions(w, req)

	var resp ListSessionsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Sessions) != 2 {
		t.Errorf("Sessions count = %d, want 2", len(resp.Sessions))
	}
}

func TestSendMessageNotFound(t *testing.T) {
	s := NewServer()
	body := strings.NewReader(`{"message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/sessions/nonexistent/message", body)
	w := httptest.NewRecorder()
	s.sendMessage(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestSendMessageInvalidBody(t *testing.T) {
	s := NewServer()

	// Create session first
	createReq := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	createW := httptest.NewRecorder()
	s.createSession(createW, createReq)
	var createResp CreateSessionResponse
	json.NewDecoder(createW.Body).Decode(&createResp)

	// Send invalid JSON
	body := strings.NewReader("not json")
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+createResp.SessionID+"/message", body)
	w := httptest.NewRecorder()
	s.sendMessage(w, req, createResp.SessionID)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestBroadcaster(t *testing.T) {
	b := NewBroadcaster(10)
	ch := b.Subscribe()

	event := SessionEvent{
		Type:      "message",
		SessionID: "test-session",
		Data:      "hello",
	}

	b.Send(event)

	select {
	case received := <-ch:
		if received.Type != "message" {
			t.Errorf("Type = %q, want %q", received.Type, "message")
		}
		if received.SessionID != "test-session" {
			t.Errorf("SessionID = %q", received.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast event")
	}

	b.Unsubscribe(ch)
}

func TestSessionCreated(t *testing.T) {
	s := NewSession("test-id", nil)
	if s.ID != "test-id" {
		t.Errorf("ID = %q, want %q", s.ID, "test-id")
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if s.Events == nil {
		t.Error("Events should not be nil")
	}
}

func TestRoutes(t *testing.T) {
	s := NewServer()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// POST /sessions
	resp, err := http.Post(ts.URL+"/sessions", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /sessions error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST /sessions status = %d", resp.StatusCode)
	}

	// GET /sessions
	resp, err = http.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("GET /sessions error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /sessions status = %d", resp.StatusCode)
	}

	// GET /sessions/nonexistent
	resp, err = http.Get(ts.URL + "/sessions/nonexistent")
	if err != nil {
		t.Fatalf("GET /sessions/nonexistent error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /sessions/nonexistent status = %d", resp.StatusCode)
	}
}
