package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBashSuccess(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out.Content)
	}
}

func TestBashFailure(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"exit 1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for failing command")
	}
}

func TestBashTimeout(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"sleep 10","timeout":100}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected timeout error")
	}
	if !strings.Contains(out.Content, "timed out") {
		t.Errorf("output = %q, want 'timed out'", out.Content)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "read_file", json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if out.Content != "hello world" {
		t.Errorf("Content = %q, want %q", out.Content, "hello world")
	}
}

func TestReadFileNotFound(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "read_file", json.RawMessage(`{"path":"nonexistent.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for missing file")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	out, err := r.ExecuteTool(context.Background(), "write_file", json.RawMessage(`{"path":"sub/dir/test.txt","content":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, err := os.ReadFile(filepath.Join(dir, "sub", "dir", "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("file content = %q, want %q", string(data), "hello")
	}
}

func TestWriteFileOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("old"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "write_file", json.RawMessage(`{"path":"test.txt","content":"new"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("content = %q, want %q", string(data), "new")
	}
}

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"world","new_string":"Go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello Go" {
		t.Errorf("content = %q, want %q", string(data), "hello Go")
	}
}

func TestEditFileNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"missing","new_string":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when old_string not found")
	}
}

func TestEditFileMultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("aaa bbb aaa"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "edit_file", json.RawMessage(`{"path":"test.txt","old_string":"aaa","new_string":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error when old_string found multiple times")
	}
}

func TestGlobSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "glob_search", json.RawMessage(`{"pattern":"*.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, "b.go") {
		t.Errorf("output = %q, want a.go and b.go", out.Content)
	}
	if strings.Contains(out.Content, "c.txt") {
		t.Error("should not match c.txt")
	}
}

func TestGlobSearchNoMatch(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "glob_search", json.RawMessage(`{"pattern":"*.xyz"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No files") {
		t.Errorf("output = %q, want 'No files'", out.Content)
	}
}

func TestGrepSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc hello() {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc world() {}\n"), 0o644)

	r := NewRegistry(dir)
	out, err := r.ExecuteTool(context.Background(), "grep_search", json.RawMessage(`{"pattern":"func hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", out.Content)
	}
	if strings.Contains(out.Content, "world") {
		t.Error("should not match world")
	}
}

func TestGrepSearchNoMatch(t *testing.T) {
	r := NewRegistry(t.TempDir())
	os.WriteFile(filepath.Join(t.TempDir(), "test.txt"), []byte("hello"), 0o644)

	out, err := r.ExecuteTool(context.Background(), "grep_search", json.RawMessage(`{"pattern":"xyz123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No matches") {
		t.Errorf("output = %q, want 'No matches'", out.Content)
	}
}

func TestExecuteToolUnknown(t *testing.T) {
	r := NewRegistry(t.TempDir())
	out, err := r.ExecuteTool(context.Background(), "nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestGetToolSpecs(t *testing.T) {
	r := NewRegistry(t.TempDir())
	specs := r.GetToolSpecs()
	if len(specs) != 22 {
		t.Errorf("expected 22 tool specs, got %d", len(specs))
	}

	names := make(map[string]bool)
	for _, s := range specs {
		names[s.Name] = true
	}
	for _, name := range []string{"bash", "read_file", "write_file", "edit_file", "glob_search", "grep_search", "web_fetch", "web_search", "todo_write", "tool_search", "notebook_edit", "sleep", "send_user_message", "config", "lsp_go_to_definition", "lsp_find_references", "lsp_hover", "lsp_document_symbol", "lsp_workspace_symbol", "lsp_go_to_implementation", "lsp_incoming_calls", "lsp_outgoing_calls"} {
		if !names[name] {
			t.Errorf("missing tool spec: %s", name)
		}
	}
}

func TestWebFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from server"))
	}))
	defer ts.Close()

	r := NewRegistry(t.TempDir())
	input, _ := json.Marshal(map[string]string{"url": ts.URL})
	out, err := r.ExecuteTool(context.Background(), "web_fetch", input)
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("unexpected error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "Hello from server") {
		t.Errorf("output = %q", out.Content)
	}
}
