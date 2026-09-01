package entity

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	api "github.com/hieu-glaw/glaw-code/internal/api"
	mcp "github.com/hieu-glaw/glaw-code/internal/modules/mcp/infrastructure/transport"
	permentity "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/entity"
	permservice "github.com/hieu-glaw/glaw-code/internal/modules/permission/domain/service"
	sessionentity "github.com/hieu-glaw/glaw-code/internal/modules/session/domain/entity"
)

// =====================================================================
// CompositeToolExecutor Tests
// =====================================================================

type mockToolExec struct {
	specs   []api.ToolDefinition
	results map[string]*ToolOutput
}

func newMockToolExec(specs []api.ToolDefinition, results map[string]*ToolOutput) *mockToolExec {
	return &mockToolExec{specs: specs, results: results}
}

func (m *mockToolExec) ExecuteTool(_ context.Context, name string, _ json.RawMessage) (*ToolOutput, error) {
	if r, ok := m.results[name]; ok {
		return r, nil
	}
	return &ToolOutput{Content: fmt.Sprintf("Unknown tool: %s", name), IsError: true}, nil
}

func (m *mockToolExec) GetToolSpecs() []api.ToolDefinition { return m.specs }

func TestNewCompositeToolExecutorNilBuiltin(t *testing.T) {
	comp := NewCompositeToolExecutor(nil, nil)
	if comp == nil {
		t.Fatal("expected non-nil CompositeToolExecutor")
	}
}

func TestCompositeToolExecutorGetToolSpecsBuiltinOnly(t *testing.T) {
	specs := []api.ToolDefinition{
		{Name: "bash", Description: "run commands"},
		{Name: "read_file", Description: "read a file"},
	}
	builtin := newMockToolExec(specs, nil)
	comp := NewCompositeToolExecutor(builtin, nil)

	result := comp.GetToolSpecs()
	if len(result) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(result))
	}
	if result[0].Name != "bash" {
		t.Errorf("spec[0].Name = %q, want %q", result[0].Name, "bash")
	}
}

func TestCompositeToolExecutorGetToolSpecsMergesMCP(t *testing.T) {
	builtinSpecs := []api.ToolDefinition{{Name: "bash", Description: "run commands"}}
	builtin := newMockToolExec(builtinSpecs, nil)
	mgr := mcp.NewManager() // empty MCP manager, no tools

	comp := NewCompositeToolExecutor(builtin, mgr)
	result := comp.GetToolSpecs()

	if len(result) < 1 {
		t.Fatalf("expected at least 1 spec, got %d", len(result))
	}
}

func TestCompositeToolExecutorExecuteKnownBuiltinTool(t *testing.T) {
	builtin := newMockToolExec(nil, map[string]*ToolOutput{
		"bash": {Content: "hello world", IsError: false},
	})
	comp := NewCompositeToolExecutor(builtin, nil)

	out, err := comp.ExecuteTool(context.Background(), "bash", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "hello world" {
		t.Errorf("Content = %q, want %q", out.Content, "hello world")
	}
}

func TestCompositeToolExecutorExecuteUnknownTool(t *testing.T) {
	builtin := newMockToolExec(nil, nil) // returns "Unknown tool: X"
	comp := NewCompositeToolExecutor(builtin, nil)

	out, err := comp.ExecuteTool(context.Background(), "nonexistent_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// Should return the unknown tool error from builtin
	if !out.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestIsUnknownTool(t *testing.T) {
	tests := []struct {
		content string
		name    string
		want    bool
	}{
		{"Unknown tool: bash", "bash", true},
		{"Unknown tool: read_file", "read_file", true},
		{"Some other error", "bash", false},
		{"", "bash", false},
		{"Unknown tool: bash", "read_file", false},
	}
	for _, tt := range tests {
		got := isUnknownTool(tt.content, tt.name)
		if got != tt.want {
			t.Errorf("isUnknownTool(%q, %q) = %v, want %v", tt.content, tt.name, got, tt.want)
		}
	}
}

// =====================================================================
// ConversationRuntime State Management Tests
// =====================================================================

func newTestRuntime() *ConversationRuntime {
	return NewConversationRuntime(
		nil,
		DefaultConfig(),
		sessionentity.NewSession(),
		permservice.NewPermissionManager(permentity.PermWorkspaceWrite, "/tmp"),
		nil,
	)
}

func TestRuntimeIsRunningInitiallyFalse(t *testing.T) {
	rt := newTestRuntime()
	if rt.IsRunning() {
		t.Error("IsRunning should be false initially")
	}
}

func TestRuntimeSetRunningThenIdle(t *testing.T) {
	rt := newTestRuntime()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt.SetRunning(cancel)
	if !rt.IsRunning() {
		t.Error("IsRunning should be true after SetRunning")
	}

	rt.SetIdle()
	if rt.IsRunning() {
		t.Error("IsRunning should be false after SetIdle")
	}
}

func TestRuntimeCancelAction(t *testing.T) {
	rt := newTestRuntime()
	_, cancel := context.WithCancel(context.Background())

	rt.SetRunning(cancel)
	cancelled := rt.CancelAction()
	if !cancelled {
		t.Error("CancelAction should return true when action was running")
	}
	if rt.IsRunning() {
		t.Error("IsRunning should be false after CancelAction")
	}

	// Cancel again should return false
	cancelled = rt.CancelAction()
	if cancelled {
		t.Error("CancelAction should return false when no action running")
	}
}

func TestIsActionCancelled(t *testing.T) {
	err := &ActionCancelledError{}
	if !IsActionCancelled(err) {
		t.Error("IsActionCancelled should return true for ActionCancelledError")
	}
	if IsActionCancelled(fmt.Errorf("other error")) {
		t.Error("IsActionCancelled should return false for other errors")
	}
}

func TestActionCancelledErrorMessage(t *testing.T) {
	err := &ActionCancelledError{}
	if err.Error() != "action cancelled by user" {
		t.Errorf("Error() = %q, want %q", err.Error(), "action cancelled by user")
	}
}

func TestRuntimeIsYoloMode(t *testing.T) {
	rt := newTestRuntime()
	if rt.IsYoloMode() {
		t.Error("should not be yolo initially")
	}
}

func TestRuntimeToggleYoloMode(t *testing.T) {
	rt := newTestRuntime()
	enabled := rt.ToggleYoloMode()
	if !enabled {
		t.Error("first toggle should enable yolo")
	}
	if !rt.IsYoloMode() {
		t.Error("should be yolo after toggle")
	}
	disabled := rt.ToggleYoloMode()
	if disabled {
		t.Error("second toggle should disable yolo")
	}
	if rt.IsYoloMode() {
		t.Error("should not be yolo after second toggle")
	}
}

func TestRuntimeSetModelWithClientFactory(t *testing.T) {
	rt := newTestRuntime()
	called := false
	rt.ClientFactory = func(model string) (api.ProviderClient, error) {
		called = true
		return nil, nil
	}
	rt.SetModel("new-model")
	if !called {
		t.Error("ClientFactory should have been called")
	}
	if rt.GetModel() != "new-model" {
		t.Errorf("Model = %q, want %q", rt.GetModel(), "new-model")
	}
}

// =====================================================================
// ConversationRuntime Session Management Tests
// =====================================================================

func TestRuntimeNewSessionResetsUsage(t *testing.T) {
	rt := newTestRuntime()
	oldID := rt.GetSessionID()
	rt.NewSession()
	if rt.GetSessionID() == oldID {
		t.Error("session ID should change after NewSession")
	}
}

func TestRuntimeGetWorkspaceRoot(t *testing.T) {
	rt := newTestRuntime()
	root := rt.GetWorkspaceRoot()
	if root != "/tmp" {
		t.Errorf("GetWorkspaceRoot = %q, want %q", root, "/tmp")
	}
}

func TestRuntimeGetWorkspaceRootNilPerms(t *testing.T) {
	rt := NewConversationRuntime(nil, DefaultConfig(), sessionentity.NewSession(), nil, nil)
	root := rt.GetWorkspaceRoot()
	if root != "" {
		t.Errorf("GetWorkspaceRoot with nil perms = %q, want empty", root)
	}
}

func TestRuntimeGetAllSettings(t *testing.T) {
	rt := newTestRuntime()
	settings := rt.GetAllSettings()
	if settings == nil {
		t.Fatal("GetAllSettings should not return nil")
	}
	if model, ok := settings["model"]; !ok || model == nil {
		t.Error("settings should contain model")
	}
}

func TestRuntimeGetSubAgentSessionsNil(t *testing.T) {
	rt := newTestRuntime()
	sessions := rt.GetSubAgentSessions()
	if sessions != nil {
		t.Error("should be nil when no SubAgentMgr")
	}
}

func TestRuntimeResumeSubAgentSessionNilMgr(t *testing.T) {
	rt := newTestRuntime()
	err := rt.ResumeSubAgentSession("agent-123")
	if err == nil {
		t.Error("expected error when SubAgentMgr is nil")
	}
}

// =====================================================================
// BuildSystemPrompt / BuildToolDefinitions Tests
// =====================================================================

func TestRuntimeBuildSystemPromptCustom(t *testing.T) {
	rt := newTestRuntime()
	rt.SystemPrompt = "custom prompt"
	got := rt.BuildSystemPrompt()
	if got != "custom prompt" {
		t.Errorf("BuildSystemPrompt = %q, want %q", got, "custom prompt")
	}
}

func TestRuntimeBuildToolDefinitionsNil(t *testing.T) {
	rt := newTestRuntime() // ToolExecutor is nil
	defs := rt.BuildToolDefinitions()
	if defs != nil {
		t.Error("BuildToolDefinitions should return nil when no ToolExecutor")
	}
}

func TestRuntimeBuildToolDefinitions(t *testing.T) {
	mock := newMockToolExec([]api.ToolDefinition{{Name: "test_tool"}}, nil)
	rt := NewConversationRuntime(nil, DefaultConfig(), sessionentity.NewSession(),
		permservice.NewPermissionManager(permentity.PermWorkspaceWrite, "/tmp"), mock)
	defs := rt.BuildToolDefinitions()
	if len(defs) != 1 || defs[0].Name != "test_tool" {
		t.Errorf("BuildToolDefinitions = %v, want [{test_tool}]", defs)
	}
}

// =====================================================================
// LoadInstructionFiles Tests
// =====================================================================

func TestLoadInstructionFilesEmpty(t *testing.T) {
	dir := t.TempDir()
	files := LoadInstructionFiles(dir)
	// No instruction files exist
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestLoadInstructionFilesGLAWmd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "GLAW.md"), []byte("# Rules\nBe helpful"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := LoadInstructionFiles(dir)
	if len(files) == 0 {
		t.Fatal("expected at least one file")
	}
	if _, ok := files["GLAW.md"]; !ok {
		t.Error("expected GLAW.md to be loaded")
	}
	if !strings.Contains(files["GLAW.md"], "Be helpful") {
		t.Errorf("content = %q, should contain 'Be helpful'", files["GLAW.md"])
	}
}

func TestLoadInstructionFilesGlawDir(t *testing.T) {
	dir := t.TempDir()
	glawDir := filepath.Join(dir, ".glaw")
	if err := os.MkdirAll(glawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(glawDir, "GLAW.md"), []byte("project instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := LoadInstructionFiles(dir)
	if _, ok := files[".glaw/GLAW.md"]; !ok {
		t.Error("expected .glaw/GLAW.md to be loaded")
	}
}

func TestLoadInstructionFilesInstructionsDir(t *testing.T) {
	dir := t.TempDir()
	instrDir := filepath.Join(dir, ".glaw", "instructions")
	if err := os.MkdirAll(instrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instrDir, "testing.md"), []byte("test rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-md file should be ignored
	if err := os.WriteFile(filepath.Join(instrDir, "ignore.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := LoadInstructionFiles(dir)
	if _, ok := files[filepath.Join(".glaw", "instructions", "testing.md")]; !ok {
		t.Errorf("expected testing.md in instructions, got keys: %v", files)
	}
	if len(files) > 1 {
		t.Error("non-md files should be ignored")
	}
}

func TestLoadInstructionFilesCLAWBackwardCompat(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAW.md"), []byte("legacy instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := LoadInstructionFiles(dir)
	if _, ok := files["CLAW.md"]; !ok {
		t.Error("expected CLAW.md to be loaded for backward compat")
	}
}

// =====================================================================
// SnapshottingExecutor.GetToolSpecs Tests
// =====================================================================

func TestSnapshottingExecutorGetToolSpecs(t *testing.T) {
	specs := []api.ToolDefinition{{Name: "bash"}, {Name: "read_file"}}
	inner := newMockToolExec(specs, nil)
	snap := NewSnapshottingExecutor(inner)

	result := snap.GetToolSpecs()
	if len(result) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(result))
	}
	if result[0].Name != "bash" {
		t.Errorf("spec[0].Name = %q", result[0].Name)
	}
}

// =====================================================================
// Render Helper Tests
// =====================================================================

func TestRenderToolHeader(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
		contains string
	}{
		{"bash with command", "bash", `{"command":"ls -la"}`, "ls -la"},
		{"write_file with path", "write_file", `{"path":"/tmp/test.txt"}`, "/tmp/test.txt"},
		{"read_file with path", "read_file", `{"path":"main.go"}`, "main.go"},
		{"unknown tool", "custom_tool", `{}`, "custom_tool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderToolHeader(tt.toolName, json.RawMessage(tt.input))
			if !strings.Contains(result, tt.contains) {
				t.Errorf("renderToolHeader(%q) = %q, should contain %q", tt.toolName, result, tt.contains)
			}
		})
	}
}

func TestRenderToolHeaderIcons(t *testing.T) {
	tests := []struct {
		toolName string
		icon     string
	}{
		{"bash", "$"},
		{"write_file", "✎"},
		{"edit_file", "✎"},
		{"read_file", "📄"},
		{"glob_search", "📄"},
		{"grep_search", "📄"},
	}
	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := renderToolHeader(tt.toolName, json.RawMessage(`{}`))
			if !strings.Contains(result, tt.icon) {
				t.Errorf("renderToolHeader(%q) = %q, should contain icon %q", tt.toolName, result, tt.icon)
			}
		})
	}
}

func TestRenderToolDone(t *testing.T) {
	tests := []struct {
		name    string
		isError bool
		want    string
	}{
		{"success", false, "✓"},
		{"error", true, "✗"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderToolDone("bash", "output", tt.isError, 100*time.Millisecond)
			if !strings.Contains(result, tt.want) {
				t.Errorf("renderToolDone(isError=%v) = %q, should contain %q", tt.isError, result, tt.want)
			}
		})
	}
}

func TestRenderToolDoneTruncatesLongOutput(t *testing.T) {
	longOutput := strings.Repeat("x", 200)
	result := renderToolDone("bash", longOutput, false, 50*time.Millisecond)
	if strings.Contains(result, strings.Repeat("x", 200)) {
		t.Error("long output should be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("truncated output should contain ...")
	}
}

func TestToolDisplayInfo(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    string
		want     string
	}{
		{"bash short", "bash", `{"command":"ls"}`, "ls"},
		{"bash long truncates", "bash", fmt.Sprintf(`{"command":"%s"}`, strings.Repeat("x", 100)), "..."},
		{"write_file", "write_file", `{"path":"/tmp/test.txt"}`, "/tmp/test.txt"},
		{"glob_search", "glob_search", `{"pattern":"**/*.go"}`, "**/*.go"},
		{"grep_search", "grep_search", `{"pattern":"TODO"}`, "TODO"},
		{"unknown tool", "unknown", `{}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolDisplayInfo(tt.toolName, json.RawMessage(tt.input))
			if !strings.Contains(got, tt.want) {
				t.Errorf("toolDisplayInfo(%q) = %q, should contain %q", tt.toolName, got, tt.want)
			}
		})
	}
}

// =====================================================================
// Concurrency Tests
// =====================================================================

func TestRuntimeConcurrentStateChanges(t *testing.T) {
	rt := newTestRuntime()
	var wg sync.WaitGroup

	// Concurrently toggle running state
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, cancel := context.WithCancel(context.Background())
			rt.SetRunning(cancel)
			rt.CancelAction()
			rt.SetIdle()
		}()
	}
	wg.Wait()
}

func TestUsageTrackerConcurrentRecord(t *testing.T) {
	tr := NewUsageTracker()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Record(api.Usage{InputTokens: 10, OutputTokens: 5})
		}()
	}
	wg.Wait()
	if tr.Cumulative.InputTokens != 1000 {
		t.Errorf("Cumulative.InputTokens = %d, want 1000", tr.Cumulative.InputTokens)
	}
	if tr.Turns != 100 {
		t.Errorf("Turns = %d, want 100", tr.Turns)
	}
}

// =====================================================================
// Save/Load Session Tests
// =====================================================================

func TestSaveAndLoadSessionFromConversationEntity(t *testing.T) {
	dir := t.TempDir()
	s := sessionentity.NewSession()
	s.AddUserMessageFromText("test message")

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
	if loaded.MessageCount() != 1 {
		t.Errorf("MessageCount = %d, want 1", loaded.MessageCount())
	}
}
