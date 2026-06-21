package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// =====================================================================
// Backward Compatibility Re-export Tests
// These tests verify that the runtime package correctly re-exports
// types from the new module structure.
// =====================================================================

func TestNewSessionReExport(t *testing.T) {
	s := NewSession()
	if s == nil {
		t.Fatal("NewSession should not return nil")
	}
	if s.ID == "" {
		t.Error("Session ID should not be empty")
	}
}

func TestSessionMethods(t *testing.T) {
	s := NewSession()
	s.AddUserMessageFromText("hello")
	if s.MessageCount() != 1 {
		t.Errorf("MessageCount = %d, want 1", s.MessageCount())
	}

	s.AddAssistantMessage([]api.ContentBlock{api.NewTextBlock("world")}, nil)
	if s.MessageCount() != 2 {
		t.Errorf("MessageCount = %d, want 2", s.MessageCount())
	}
}

func TestPermissionModeConstants(t *testing.T) {
	if PermReadOnly != "read_only" {
		t.Errorf("PermReadOnly = %q", PermReadOnly)
	}
	if PermWorkspaceWrite != "workspace_write" {
		t.Errorf("PermWorkspaceWrite = %q", PermWorkspaceWrite)
	}
	if PermDangerFullAccess != "danger_full_access" {
		t.Errorf("PermDangerFullAccess = %q", PermDangerFullAccess)
	}
	if PermYolo != "yolo" {
		t.Errorf("PermYolo = %q", PermYolo)
	}
}

func TestNewPermissionManagerReExport(t *testing.T) {
	pm := NewPermissionManager(PermWorkspaceWrite, "/tmp")
	if pm == nil {
		t.Fatal("NewPermissionManager should not return nil")
	}
	if !pm.Check(PermReadFile) {
		t.Error("should allow read in workspace_write mode")
	}
}

func TestNewEnhancedPermissionManagerReExport(t *testing.T) {
	pm := NewEnhancedPermissionManager(PermWorkspaceWrite, "/tmp")
	if pm == nil {
		t.Fatal("NewEnhancedPermissionManager should not return nil")
	}
}

func TestDefaultConfigReExport(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig should not return nil")
	}
	if cfg.Model == "" {
		t.Error("Model should not be empty")
	}
}

func TestNewConversationRuntimeReExport(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), NewSession(),
		NewPermissionManager(PermReadOnly, "/tmp"), nil)
	if rt == nil {
		t.Fatal("NewConversationRuntime should not return nil")
	}
}

func TestNewUsageTrackerReExport(t *testing.T) {
	ut := NewUsageTracker()
	if ut == nil {
		t.Fatal("NewUsageTracker should not return nil")
	}
	ut.Record(api.Usage{InputTokens: 100, OutputTokens: 50})
	if ut.LatestTurn.InputTokens != 100 {
		t.Errorf("LatestTurn.InputTokens = %d, want 100", ut.LatestTurn.InputTokens)
	}
}

func TestPricingForModelReExport(t *testing.T) {
	p := PricingForModel("claude-sonnet-4-6")
	if p.InputCostPerMillion != 3.0 {
		t.Errorf("InputCostPerMillion = %v, want 3.0", p.InputCostPerMillion)
	}
}

func TestFormatUSDReExport(t *testing.T) {
	got := FormatUSD(1.5)
	if got != "$1.5000" {
		t.Errorf("FormatUSD(1.5) = %q, want %q", got, "$1.5000")
	}
}

func TestDetectSandboxStatusReExport(t *testing.T) {
	status := DetectSandboxStatus()
	if status.OS == "" {
		t.Error("OS should not be empty")
	}
}

func TestOSNameReExport(t *testing.T) {
	name := OSName()
	if name == "" {
		t.Error("OSName should not be empty")
	}
}

func TestSaveAndLoadSessionReExport(t *testing.T) {
	dir := t.TempDir()
	s := NewSession()
	s.AddUserMessageFromText("test")

	path, err := SaveSession(s, dir)
	if err != nil {
		t.Fatalf("SaveSession error: %v", err)
	}

	loaded, err := LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession error: %v", err)
	}
	if loaded.ID != s.ID {
		t.Errorf("ID mismatch: %q != %q", loaded.ID, s.ID)
	}
}

// =====================================================================
// BuiltinToolExecutor Tests
// =====================================================================

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
