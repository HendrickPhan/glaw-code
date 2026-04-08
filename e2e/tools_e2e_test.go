package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/runtime"
	"github.com/hieu-glaw/glaw-code/internal/tools"
)

func newTestRegistry(t *testing.T) (*tools.Registry, string) {
	t.Helper()
	dir := t.TempDir()
	reg := tools.NewRegistry(dir)
	return reg, dir
}

func toolInput(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	return b
}

func execTool(t *testing.T, reg *tools.Registry, name string, input interface{}) *runtime.ToolOutput {
	t.Helper()
	out, err := reg.ExecuteTool(context.Background(), name, toolInput(t, input))
	if err != nil {
		t.Fatalf("ExecuteTool(%q) error: %v", name, err)
	}
	return out
}

// --- Registry Specs ---

func TestE2EToolRegistrySpecs(t *testing.T) {
	reg, _ := newTestRegistry(t)
	specs := reg.GetToolSpecs()


	wantNames := map[string]bool{
		"bash": false, "read_file": false, "write_file": false, "edit_file": false,
		"glob_search": false, "grep_search": false, "web_fetch": false,
		"web_search": false, "todo_write": false, "tool_search": false,
		"notebook_edit": false, "sleep": false, "send_user_message": false,
		"config": false, "analyze": false,
		"sub_agent": false, "sub_agent_result": false,
	}
	for _, spec := range specs {
		if _, ok := wantNames[spec.Name]; !ok {
			t.Errorf("unexpected tool: %q", spec.Name)
			continue
		}
		wantNames[spec.Name] = true
		if spec.Description == "" {
			t.Errorf("tool %q has empty description", spec.Name)
		}
		if !json.Valid(spec.InputSchema) {
			t.Errorf("tool %q has invalid InputSchema", spec.Name)
			continue
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Errorf("tool %q InputSchema unmarshal: %v", spec.Name, err)
		} else if schema["type"] != "object" {
			t.Errorf("tool %q InputSchema type = %v, want 'object'", spec.Name, schema["type"])
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Errorf("missing tool: %q", name)
		}
	}
}

// --- Unknown Tool ---

func TestE2EToolUnknown(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "nonexistent", map[string]string{"q": "test"})
	if !out.IsError {
		t.Error("IsError should be true for unknown tool")
	}
	if !strings.Contains(out.Content, "Unknown tool") {
		t.Errorf("Content = %q", out.Content)
	}
}

// --- Bash ---

func TestE2EToolBashEcho(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "bash", map[string]string{"command": "echo hello"})
	if out.IsError {
		t.Errorf("IsError = true, Content = %q", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("Content = %q, want 'hello'", out.Content)
	}
}

func TestE2EToolBashFailure(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "bash", map[string]string{"command": "exit 42"})
	if !out.IsError {
		t.Error("IsError should be true for failing command")
	}
}

func TestE2EToolBashTimeout(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "bash", map[string]interface{}{"command": "sleep 30", "timeout": 100})
	if !out.IsError {
		t.Error("IsError should be true for timeout")
	}
	if !strings.Contains(out.Content, "timed out") {
		t.Errorf("Content = %q, should mention 'timed out'", out.Content)
	}
}

func TestE2EToolBashInvalidJSON(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out, err := reg.ExecuteTool(context.Background(), "bash", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if !out.IsError {
		t.Error("IsError should be true for invalid JSON")
	}
	if !strings.Contains(out.Content, "Invalid input") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolBashEmptyCommand(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "bash", map[string]string{"command": ""})
	if !out.IsError {
		t.Error("IsError should be true for empty command")
	}
}

func TestE2EToolBashCaptureStderr(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "bash", map[string]string{"command": "echo err >&2 && echo out"})
	if out.IsError {
		t.Fatalf("IsError = true, Content = %q", out.Content)
	}
	if !strings.Contains(out.Content, "err") || !strings.Contains(out.Content, "out") {
		t.Errorf("Content = %q, should contain both stderr and stdout", out.Content)
	}
}

// --- File Write/Read ---

func TestE2EToolWriteReadRoundTrip(t *testing.T) {
	reg, _ := newTestRegistry(t)
	writeOut := execTool(t, reg, "write_file", map[string]string{
		"path":    "sub/dir/test.txt",
		"content": "hello world",
	})
	if writeOut.IsError {
		t.Fatalf("write error: %s", writeOut.Content)
	}
	if !strings.Contains(writeOut.Content, "Wrote") {
		t.Errorf("Content = %q", writeOut.Content)
	}

	readOut := execTool(t, reg, "read_file", map[string]string{"path": "sub/dir/test.txt"})
	if readOut.IsError {
		t.Fatalf("read error: %s", readOut.Content)
	}
	if readOut.Content != "hello world" {
		t.Errorf("Content = %q, want 'hello world'", readOut.Content)
	}
}

func TestE2EToolReadFileNotFound(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "read_file", map[string]string{"path": "nonexistent.txt"})
	if !out.IsError {
		t.Error("IsError should be true for missing file")
	}
}

func TestE2EToolWriteFileOverwrite(t *testing.T) {
	reg, _ := newTestRegistry(t)
	execTool(t, reg, "write_file", map[string]string{"path": "file.txt", "content": "first"})
	execTool(t, reg, "write_file", map[string]string{"path": "file.txt", "content": "second"})
	out := execTool(t, reg, "read_file", map[string]string{"path": "file.txt"})
	if out.Content != "second" {
		t.Errorf("Content = %q, want 'second'", out.Content)
	}
}

// --- Edit File ---

func TestE2EToolEditFile(t *testing.T) {
	reg, _ := newTestRegistry(t)
	execTool(t, reg, "write_file", map[string]string{"path": "edit.txt", "content": "foo bar baz"})

	out := execTool(t, reg, "edit_file", map[string]string{
		"path":       "edit.txt",
		"old_string": "bar",
		"new_string": "QUX",
	})
	if out.IsError {
		t.Fatalf("edit error: %s", out.Content)
	}

	readOut := execTool(t, reg, "read_file", map[string]string{"path": "edit.txt"})
	if readOut.Content != "foo QUX baz" {
		t.Errorf("Content = %q, want 'foo QUX baz'", readOut.Content)
	}
}

func TestE2EToolEditFileNotFound(t *testing.T) {
	reg, _ := newTestRegistry(t)
	execTool(t, reg, "write_file", map[string]string{"path": "file.txt", "content": "hello"})

	out := execTool(t, reg, "edit_file", map[string]string{
		"path":       "file.txt",
		"old_string": "missing",
		"new_string": "replacement",
	})
	if !out.IsError {
		t.Error("IsError should be true when old_string not found")
	}
	if !strings.Contains(out.Content, "not found") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolEditFileAmbiguous(t *testing.T) {
	reg, _ := newTestRegistry(t)
	execTool(t, reg, "write_file", map[string]string{"path": "file.txt", "content": "aaa bbb aaa"})

	out := execTool(t, reg, "edit_file", map[string]string{
		"path":       "file.txt",
		"old_string": "aaa",
		"new_string": "zzz",
	})
	if !out.IsError {
		t.Error("IsError should be true for ambiguous match")
	}
	if !strings.Contains(out.Content, "2 times") && !strings.Contains(out.Content, "unique") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolEditFileMissingFile(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "edit_file", map[string]string{
		"path":       "nope.txt",
		"old_string": "a",
		"new_string": "b",
	})
	if !out.IsError {
		t.Error("IsError should be true for missing file")
	}
}

// --- Glob Search ---

func TestE2EToolGlobSearch(t *testing.T) {
	reg, dir := newTestRegistry(t)
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("pkg main"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("pkg main"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644); err != nil {
			t.Fatal(err)
		}

	out := execTool(t, reg, "glob_search", map[string]string{"pattern": "*.go"})
	if out.IsError {
		t.Fatalf("glob error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, "b.go") {
		t.Errorf("Content = %q, should contain a.go and b.go", out.Content)
	}
	if strings.Contains(out.Content, "c.txt") {
		t.Errorf("Content should not contain c.txt: %q", out.Content)
	}
}

func TestE2EToolGlobSearchNoMatch(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "glob_search", map[string]string{"pattern": "*.xyz"})
	if out.IsError {
		t.Fatalf("glob error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No files") {
		t.Errorf("Content = %q", out.Content)
	}
}

// --- Grep Search ---

func TestE2EToolGrepSearch(t *testing.T) {
	reg, dir := newTestRegistry(t)
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("func hello() {}\nfunc world() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte("func other() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := execTool(t, reg, "grep_search", map[string]string{"pattern": "func hello"})
	if out.IsError {
		t.Fatalf("grep error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "hello") {
		t.Errorf("Content = %q, should contain 'hello'", out.Content)
	}
}

func TestE2EToolGrepSearchInSubdirectory(t *testing.T) {
	reg, dir := newTestRegistry(t)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "target.go"), []byte("target_string here\n"), 0o644); err != nil {
			t.Fatal(err)
		}

	out := execTool(t, reg, "grep_search", map[string]string{"pattern": "target_string", "path": "sub"})
	if out.IsError {
		t.Fatalf("grep error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "target_string") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolGrepInvalidRegex(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "grep_search", map[string]string{"pattern": "[invalid"})
	if !out.IsError {
		t.Error("IsError should be true for invalid regex")
	}
	if !strings.Contains(out.Content, "Invalid regex") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolGrepNoMatch(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "grep_search", map[string]string{"pattern": "zzzzzzz"})
	if out.IsError {
		t.Fatalf("grep error: %s", out.Content)
	}
	if !strings.Contains(out.Content, "No matches") {
		t.Errorf("Content = %q", out.Content)
	}
}

// --- Web Fetch ---

func TestE2EToolWebFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello from mock server"))
	}))
	defer ts.Close()

	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "web_fetch", map[string]string{"url": ts.URL})
	if out.IsError {
		t.Fatalf("web_fetch error: %s", out.Content)
	}
	if out.Content != "Hello from mock server" {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2EToolWebFetchTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer ts.Close()

	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "web_fetch", map[string]interface{}{"url": ts.URL, "timeout": 1})
	if !out.IsError {
		t.Error("IsError should be true for timeout")
	}
}

func TestE2EToolWebFetchEmptyURL(t *testing.T) {
	reg, _ := newTestRegistry(t)
	out := execTool(t, reg, "web_fetch", map[string]string{"url": ""})
	if !out.IsError {
		t.Error("IsError should be true for empty URL")
	}
	if !strings.Contains(out.Content, "url is required") {
		t.Errorf("Content = %q", out.Content)
	}
}

// --- Cross-Tool Workflow ---

func TestE2EToolCrossToolWorkflow(t *testing.T) {
	reg, dir := newTestRegistry(t)

	// Step 1: Write a Go source file
	writeOut := execTool(t, reg, "write_file", map[string]string{
		"path":    "main.go",
		"content": "package main\n\nfunc hello() string {\n\treturn \"hello\"\n}\n\nfunc main() {\n\tprintln(hello())\n}\n",
	})
	if writeOut.IsError {
		t.Fatalf("write error: %s", writeOut.Content)
	}

	// Step 2: Bash to count functions
	bashOut := execTool(t, reg, "bash", map[string]string{"command": "grep -c 'func' main.go"})
	if bashOut.IsError {
		t.Fatalf("bash error: %s", bashOut.Content)
	}
	if !strings.Contains(bashOut.Content, "2") {
		t.Errorf("func count = %q, should contain '2'", bashOut.Content)
	}

	// Step 3: Edit the file
	editOut := execTool(t, reg, "edit_file", map[string]string{
		"path":       "main.go",
		"old_string": "return \"hello\"",
		"new_string": "return \"world\"",
	})
	if editOut.IsError {
		t.Fatalf("edit error: %s", editOut.Content)
	}

	// Step 4: Read back and verify
	readOut := execTool(t, reg, "read_file", map[string]string{"path": "main.go"})
	if readOut.IsError {
		t.Fatalf("read error: %s", readOut.Content)
	}
	if !strings.Contains(readOut.Content, "return \"world\"") {
		t.Errorf("file not updated correctly: %q", readOut.Content)
	}
	if strings.Contains(readOut.Content, "return \"hello\"") {
		t.Error("old content should be gone")
	}

	// Step 5: Glob to find the file
	globOut := execTool(t, reg, "glob_search", map[string]string{"pattern": "*.go"})
	if globOut.IsError {
		t.Fatalf("glob error: %s", globOut.Content)
	}
	if !strings.Contains(globOut.Content, "main.go") {
		t.Errorf("glob Content = %q", globOut.Content)
	}

	// Step 6: Grep for the changed content
	grepOut := execTool(t, reg, "grep_search", map[string]string{"pattern": "world"})
	if grepOut.IsError {
		t.Fatalf("grep error: %s", grepOut.Content)
	}
	if !strings.Contains(grepOut.Content, "world") {
		t.Errorf("grep Content = %q", grepOut.Content)
	}

	// Verify file exists on disk
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Errorf("file should exist on disk: %v", err)
	}
}
