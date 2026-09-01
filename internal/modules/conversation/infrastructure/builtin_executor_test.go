package infrastructure

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewBuiltinToolExecutor(t *testing.T) {
	dir := t.TempDir()
	exec := NewBuiltinToolExecutor(dir)
	if exec == nil {
		t.Fatal("NewBuiltinToolExecutor should not return nil")
	}
}

func TestBuiltinToolExecutorGetToolSpecs(t *testing.T) {
	exec := NewBuiltinToolExecutor(t.TempDir())
	specs := exec.GetToolSpecs()
	if len(specs) != 4 {
		t.Fatalf("expected 4 tool specs, got %d", len(specs))
	}
	names := make(map[string]bool)
	for _, s := range specs {
		names[s.Name] = true
	}
	for _, name := range []string{"bash", "read_file", "write_file", "edit_file"} {
		if !names[name] {
			t.Errorf("missing tool spec: %s", name)
		}
	}
}

func TestBuiltinToolExecutorWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	exec := NewBuiltinToolExecutor(dir)

	// Write
	writeOut, err := exec.ExecuteTool(nil, "write_file", json.RawMessage(
		`{"path":"test.txt","content":"hello world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if writeOut.IsError {
		t.Fatalf("write failed: %s", writeOut.Content)
	}

	// Read
	readOut, err := exec.ExecuteTool(nil, "read_file", json.RawMessage(
		`{"path":"test.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if readOut.Content != "hello world" {
		t.Errorf("read content = %q, want %q", readOut.Content, "hello world")
	}
}

func TestBuiltinToolExecutorEditFile(t *testing.T) {
	dir := t.TempDir()
	exec := NewBuiltinToolExecutor(dir)

	// Write initial content
	f := filepath.Join(dir, "edit.txt")
	os.WriteFile(f, []byte("hello world"), 0o644)

	// Edit
	out, err := exec.ExecuteTool(nil, "edit_file", json.RawMessage(
		`{"path":"edit.txt","old_string":"hello","new_string":"goodbye"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("edit failed: %s", out.Content)
	}

	// Verify
	data, _ := os.ReadFile(f)
	if string(data) != "goodbye world" {
		t.Errorf("content = %q, want %q", string(data), "goodbye world")
	}
}

func TestBuiltinToolExecutorUnknownTool(t *testing.T) {
	exec := NewBuiltinToolExecutor(t.TempDir())
	out, err := exec.ExecuteTool(nil, "nonexistent", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("unknown tool should be an error")
	}
}

func TestBuiltinToolExecutorReadNonexistent(t *testing.T) {
	exec := NewBuiltinToolExecutor(t.TempDir())
	out, err := exec.ExecuteTool(nil, "read_file", json.RawMessage(
		`{"path":"nonexistent.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("reading nonexistent file should be an error")
	}
}

func TestBuiltinToolExecutorEditNoMatch(t *testing.T) {
	dir := t.TempDir()
	exec := NewBuiltinToolExecutor(dir)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644)

	out, err := exec.ExecuteTool(nil, "edit_file", json.RawMessage(
		`{"path":"test.txt","old_string":"xyz","new_string":"abc"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Error("edit with non-matching old_string should be an error")
	}
}

func TestBuiltinToolExecutorWriteCreatesDirs(t *testing.T) {
	dir := t.TempDir()
	exec := NewBuiltinToolExecutor(dir)

	out, err := exec.ExecuteTool(nil, "write_file", json.RawMessage(
		`{"path":"sub/dir/test.txt","content":"nested"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("write with nested dirs failed: %s", out.Content)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "sub", "dir", "test.txt"))
	if string(data) != "nested" {
		t.Errorf("content = %q, want %q", string(data), "nested")
	}
}

func TestBuiltinToolExecutorBash(t *testing.T) {
	exec := NewBuiltinToolExecutor(t.TempDir())
	out, err := exec.ExecuteTool(context.Background(), "bash", json.RawMessage(
		`{"command":"echo hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError {
		t.Fatalf("bash failed: %s", out.Content)
	}
	// Output may include trailing newline
	if out.Content != "hello\n" && out.Content != "hello" {
		t.Errorf("bash output = %q, want %q", out.Content, "hello")
	}
}
