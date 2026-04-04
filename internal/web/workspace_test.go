package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- WorkspaceStore tests ---

func TestWorkspaceStoreCreate(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("My Project", "/home/user/project", "A cool project")

	if ws.ID == "" {
		t.Error("expected non-empty workspace ID")
	}
	if ws.Name != "My Project" {
		t.Errorf("Name = %q, want %q", ws.Name, "My Project")
	}
	if ws.Path != "/home/user/project" {
		t.Errorf("Path = %q, want %q", ws.Path, "/home/user/project")
	}
	if ws.Description != "A cool project" {
		t.Errorf("Description = %q, want %q", ws.Description, "A cool project")
	}
	if ws.IsActive {
		t.Error("new workspace should not be active")
	}
	if len(ws.Sections) != 0 {
		t.Errorf("expected empty sections, got %d", len(ws.Sections))
	}
}

func TestWorkspaceStoreGet(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("Test", "/tmp", "")

	found, ok := store.Get(ws.ID)
	if !ok {
		t.Error("expected to find workspace")
	}
	if found.ID != ws.ID {
		t.Errorf("workspace ID = %q, want %q", found.ID, ws.ID)
	}
}

func TestWorkspaceStoreGetNotFound(t *testing.T) {
	store := NewWorkspaceStore("")
	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("expected workspace not found")
	}
}

func TestWorkspaceStoreList(t *testing.T) {
	store := NewWorkspaceStore("")
	store.Create("WS1", "/path1", "")
	store.Create("WS2", "/path2", "")

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(list))
	}

	names := map[string]bool{}
	for _, w := range list {
		names[w.Name] = true
	}
	if !names["WS1"] || !names["WS2"] {
		t.Errorf("expected both workspaces in list, got %v", list)
	}
}

func TestWorkspaceStoreUpdate(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("Old Name", "/old", "old desc")

	updated, err := store.Update(ws.ID, "New Name", "/new", "new desc")
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("Name = %q, want %q", updated.Name, "New Name")
	}
	if updated.Path != "/new" {
		t.Errorf("Path = %q, want %q", updated.Path, "/new")
	}
	if updated.Description != "new desc" {
		t.Errorf("Description = %q, want %q", updated.Description, "new desc")
	}
}

func TestWorkspaceStoreUpdateNotFound(t *testing.T) {
	store := NewWorkspaceStore("")
	_, err := store.Update("nonexistent", "x", "y", "z")
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestWorkspaceStoreDelete(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("To Delete", "/tmp", "")

	if err := store.Delete(ws.ID); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	_, ok := store.Get(ws.ID)
	if ok {
		t.Error("expected workspace to be deleted")
	}
}

func TestWorkspaceStoreDeleteNotFound(t *testing.T) {
	store := NewWorkspaceStore("")
	err := store.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for deleting nonexistent workspace")
	}
}

func TestWorkspaceStoreSetActive(t *testing.T) {
	store := NewWorkspaceStore("")
	ws1 := store.Create("WS1", "/1", "")
	ws2 := store.Create("WS2", "/2", "")

	// Activate first workspace
	active, err := store.SetActive(ws1.ID)
	if err != nil {
		t.Fatalf("SetActive error: %v", err)
	}
	if !active.IsActive {
		t.Error("expected workspace to be active")
	}

	// Activate second workspace — first should become inactive
	active2, err := store.SetActive(ws2.ID)
	if err != nil {
		t.Fatalf("SetActive error: %v", err)
	}
	if !active2.IsActive {
		t.Error("expected second workspace to be active")
	}

	// First workspace should be inactive now
	found, _ := store.Get(ws1.ID)
	if found.IsActive {
		t.Error("expected first workspace to be inactive after activating second")
	}
}

func TestWorkspaceStoreGetActive(t *testing.T) {
	store := NewWorkspaceStore("")

	// No active workspace initially
	if active := store.GetActive(); active != nil {
		t.Error("expected nil active workspace initially")
	}

	ws := store.Create("WS", "/tmp", "")
	store.SetActive(ws.ID)

	active := store.GetActive()
	if active == nil {
		t.Error("expected active workspace")
	}
	if active.ID != ws.ID {
		t.Errorf("active ID = %q, want %q", active.ID, ws.ID)
	}
}

// --- Section tests ---

func TestWorkspaceStoreCreateSection(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("WS", "/tmp", "")

	sec, err := store.CreateSection(ws.ID, "Frontend", "React app", "#3b82f6")
	if err != nil {
		t.Fatalf("CreateSection error: %v", err)
	}
	if sec.Name != "Frontend" {
		t.Errorf("section Name = %q, want %q", sec.Name, "Frontend")
	}
	if sec.Color != "#3b82f6" {
		t.Errorf("section Color = %q, want %q", sec.Color, "#3b82f6")
	}

	// Verify section is in workspace
	found, _ := store.Get(ws.ID)
	if len(found.Sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(found.Sections))
	}
	if found.Sections[0].Name != "Frontend" {
		t.Errorf("section Name = %q, want %q", found.Sections[0].Name, "Frontend")
	}
}

func TestWorkspaceStoreCreateSectionNotFound(t *testing.T) {
	store := NewWorkspaceStore("")
	_, err := store.CreateSection("nonexistent", "Sec", "", "")
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestWorkspaceStoreUpdateSection(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("WS", "/tmp", "")
	sec, _ := store.CreateSection(ws.ID, "Old", "old desc", "#000")

	updated, err := store.UpdateSection(ws.ID, sec.ID, "New", "new desc", "#fff")
	if err != nil {
		t.Fatalf("UpdateSection error: %v", err)
	}
	if updated.Name != "New" {
		t.Errorf("section Name = %q, want %q", updated.Name, "New")
	}
	if updated.Color != "#fff" {
		t.Errorf("section Color = %q, want %q", updated.Color, "#fff")
	}
}

func TestWorkspaceStoreDeleteSection(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("WS", "/tmp", "")
	sec, _ := store.CreateSection(ws.ID, "Sec1", "", "#000")
	store.CreateSection(ws.ID, "Sec2", "", "#111")

	if err := store.DeleteSection(ws.ID, sec.ID); err != nil {
		t.Fatalf("DeleteSection error: %v", err)
	}

	found, _ := store.Get(ws.ID)
	if len(found.Sections) != 1 {
		t.Fatalf("expected 1 section after delete, got %d", len(found.Sections))
	}
	if found.Sections[0].Name != "Sec2" {
		t.Errorf("remaining section Name = %q, want %q", found.Sections[0].Name, "Sec2")
	}
}

func TestWorkspaceStoreDeleteSectionNotFound(t *testing.T) {
	store := NewWorkspaceStore("")
	ws := store.Create("WS", "/tmp", "")

	err := store.DeleteSection(ws.ID, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent section")
	}
}

// --- REST API tests ---

func TestWebServerWorkspaceAPIList(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// GET list (may contain persisted workspaces from previous runs)
	resp, err := http.Get(ts.URL + "/api/workspaces")
	if err != nil {
		t.Fatalf("GET /api/workspaces error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var listResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	workspaces, ok := listResp["workspaces"].([]interface{})
	if !ok {
		t.Fatalf("expected workspaces array, got %T", listResp["workspaces"])
	}

	// Create a new workspace and verify it appears in the list
	body := `{"name":"List Test WS","path":"/tmp"}`
	resp2, _ := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	defer resp2.Body.Close()

	resp3, _ := http.Get(ts.URL + "/api/workspaces")
	defer resp3.Body.Close()
	var listResp2 map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&listResp2)
	workspaces2 := listResp2["workspaces"].([]interface{})
	if len(workspaces2) != len(workspaces)+1 {
		t.Errorf("expected %d workspaces after create, got %d", len(workspaces)+1, len(workspaces2))
	}
}

func TestWebServerWorkspaceAPICreateAndGet(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create workspace
	body := `{"name":"Test Project","path":"/tmp/test","description":"A test"}`
	resp, err := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/workspaces error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var ws map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&ws); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ws["name"] != "Test Project" {
		t.Errorf("name = %v, want %q", ws["name"], "Test Project")
	}
	if ws["path"] != "/tmp/test" {
		t.Errorf("path = %v, want %q", ws["path"], "/tmp/test")
	}

	wsID, ok := ws["id"].(string)
	if !ok || wsID == "" {
		t.Fatal("expected non-empty id")
	}

	// Get workspace by ID
	resp2, err := http.Get(ts.URL + "/api/workspaces/" + wsID)
	if err != nil {
		t.Fatalf("GET /api/workspaces/%s error: %v", wsID, err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	var ws2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&ws2); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ws2["id"] != wsID {
		t.Errorf("id = %v, want %q", ws2["id"], wsID)
	}
}

func TestWebServerWorkspaceAPICreateNoName(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := `{"path":"/tmp/test"}`
	resp, err := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestWebServerWorkspaceAPIUpdate(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create
	body := `{"name":"Old","path":"/old"}`
	resp, _ := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	var ws map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&ws)
	resp.Body.Close()
	wsID := ws["id"].(string)

	// Update
	updateBody := `{"name":"Updated","path":"/new","description":"updated"}`
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/workspaces/"+wsID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT error: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	var updated map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&updated)
	if updated["name"] != "Updated" {
		t.Errorf("name = %v, want %q", updated["name"], "Updated")
	}
}

func TestWebServerWorkspaceAPIDelete(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create
	body := `{"name":"ToDelete","path":"/tmp"}`
	resp, _ := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	var ws map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&ws)
	resp.Body.Close()
	wsID := ws["id"].(string)

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/workspaces/"+wsID, nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE error: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	// Verify deleted
	resp3, _ := http.Get(ts.URL + "/api/workspaces/" + wsID)
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", resp3.StatusCode)
	}
}

func TestWebServerWorkspaceAPIGetNotFound(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/api/workspaces/nonexistent")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestWebServerWorkspaceAPISections(t *testing.T) {
	srv := NewWebServer(nil)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create workspace
	body := `{"name":"WS","path":"/tmp"}`
	resp, _ := http.Post(ts.URL+"/api/workspaces", "application/json", strings.NewReader(body))
	var ws map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&ws)
	resp.Body.Close()
	wsID := ws["id"].(string)

	// Create section
	secBody := `{"name":"Frontend","description":"React","color":"#3b82f6"}`
	resp2, _ := http.Post(ts.URL+"/api/workspaces/"+wsID+"/sections", "application/json", strings.NewReader(secBody))
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("section create status = %d, want %d", resp2.StatusCode, http.StatusCreated)
	}
	var sec map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&sec)
	resp2.Body.Close()
	secID := sec["id"].(string)

	if sec["name"] != "Frontend" {
		t.Errorf("section name = %v, want %q", sec["name"], "Frontend")
	}

	// List sections
	resp3, _ := http.Get(ts.URL + "/api/workspaces/" + wsID + "/sections")
	defer resp3.Body.Close()
	var secList map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&secList)
	sections := secList["sections"].([]interface{})
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	// Update section
	updateSecBody := `{"name":"Backend","description":"Go API","color":"#10b981"}`
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/workspaces/"+wsID+"/sections/"+secID, strings.NewReader(updateSecBody))
	req.Header.Set("Content-Type", "application/json")
	resp4, _ := http.DefaultClient.Do(req)
	defer resp4.Body.Close()
	var updatedSec map[string]interface{}
	json.NewDecoder(resp4.Body).Decode(&updatedSec)
	if updatedSec["name"] != "Backend" {
		t.Errorf("updated section name = %v, want %q", updatedSec["name"], "Backend")
	}

	// Delete section
	req2, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/workspaces/"+wsID+"/sections/"+secID, nil)
	resp5, _ := http.DefaultClient.Do(req2)
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Errorf("section delete status = %d, want %d", resp5.StatusCode, http.StatusOK)
	}

	// Verify section deleted
	resp6, _ := http.Get(ts.URL + "/api/workspaces/" + wsID + "/sections")
	defer resp6.Body.Close()
	var secList2 map[string]interface{}
	json.NewDecoder(resp6.Body).Decode(&secList2)
	sections2 := secList2["sections"].([]interface{})
	if len(sections2) != 0 {
		t.Errorf("expected 0 sections after delete, got %d", len(sections2))
	}
}

func TestIndexOfSlash(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"abc", -1},
		{"abc/def", 3},
		{"abc/def/ghi", 3},
		{"/", 0},
	}
	for _, tt := range tests {
		got := indexOfSlash(tt.s)
		if got != tt.want {
			t.Errorf("indexOfSlash(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestExtractSectionID(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/sec-1-1", "sec-1-1"},
		{"", ""},
		{"nonslash", ""},
	}
	for _, tt := range tests {
		got := extractSectionID(tt.path)
		if got != tt.want {
			t.Errorf("extractSectionID(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
