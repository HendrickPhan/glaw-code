package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/commands"
)

// --- Mock Runtime for command tests ---

type mockRuntimeFS struct {
	model        string
	permMode     string
	sessionID    string
	msgCount     int
	usage        commands.UsageInfo
	workspaceDir string
	gitOutput    string
	gitError     error
	configMap    map[string]interface{}
}

func newMockRuntimeFS(dir string) *mockRuntimeFS {
	return &mockRuntimeFS{
		model:        "claude-sonnet-4-6",
		permMode:     "workspace_write",
		sessionID:    "sess_test",
		msgCount:     5,
		usage:        commands.UsageInfo{InputTokens: 100, OutputTokens: 50, TotalCostUSD: 0.05},
		workspaceDir: dir,
		gitOutput:    "fake diff output",
		configMap:    map[string]interface{}{"model": "claude-sonnet-4-6"},
	}
}

func (m *mockRuntimeFS) GetModel() string                    { return m.model }
func (m *mockRuntimeFS) SetModel(s string)                  { m.model = s }
func (m *mockRuntimeFS) GetPermissionMode() string           { return m.permMode }
func (m *mockRuntimeFS) SetPermissionMode(s string)          { m.permMode = s }
func (m *mockRuntimeFS) GetMessageCount() int                { return m.msgCount }
func (m *mockRuntimeFS) GetSessionID() string                { return m.sessionID }
func (m *mockRuntimeFS) GetUsageInfo() commands.UsageInfo    { return m.usage }
func (m *mockRuntimeFS) CompactSession() error               { return nil }
func (m *mockRuntimeFS) ClearSession()                       { m.msgCount = 0 }
func (m *mockRuntimeFS) GetWorkspaceRoot() string            { return m.workspaceDir }
func (m *mockRuntimeFS) GetAllSettings() map[string]interface{} { return m.configMap }
func (m *mockRuntimeFS) SetConfigValue(key, value string) error {
	m.configMap[key] = value
	return nil
}
func (m *mockRuntimeFS) RunGitCommand(args ...string) (string, error) {
	if len(args) > 0 && args[0] == "diff" && m.gitOutput == "" {
		return "", nil
	}
	return m.gitOutput, m.gitError
}
func (m *mockRuntimeFS) GetLSPStatus() []commands.LSPServerStatus { return nil }
func (m *mockRuntimeFS) RevertLastTurn() (int, error)            { return 0, nil }
func (m *mockRuntimeFS) RevertAll() (int, error)                 { return 0, nil }

func handleCmd(t *testing.T, d *commands.Dispatcher, input string) *commands.Result {
	t.Helper()
	result, err := d.Handle(context.Background(), input)
	if err != nil {
		t.Fatalf("Handle(%q) error: %v", input, err)
	}
	return result
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// --- Core Commands ---

func TestE2ECmdHelpListsAllCategories(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/help")

	wantCategories := []string{"Core", "Workspace", "Session", "Git", "Automation"}
	for _, cat := range wantCategories {
		if !strings.Contains(result.Message, cat) {
			t.Errorf("help output missing category %q", cat)
		}
	}
	wantCmds := []string{"/help", "/status", "/model", "/commit", "/skills"}
	for _, cmd := range wantCmds {
		if !strings.Contains(result.Message, cmd) {
			t.Errorf("help output missing command %q", cmd)
		}
	}
	if result.Action != "continue" {
		t.Errorf("Action = %q", result.Action)
	}
}

func TestE2ECmdStatusShowsAllFields(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/status")

	checks := []string{"claude-sonnet-4-6", "sess_test", "5", "workspace_write"}
	for _, want := range checks {
		if !strings.Contains(result.Message, want) {
			t.Errorf("status missing %q: %q", want, result.Message)
		}
	}
}

func TestE2ECmdModelSetAndGet(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	d := commands.NewDispatcher(rt)

	_ = handleCmd(t, d, "/model grok-3")
	if rt.model != "grok-3" {
		t.Errorf("model = %q, want 'grok-3'", rt.model)
	}

	result := handleCmd(t, d, "/model")
	if !strings.Contains(result.Message, "grok-3") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestE2ECmdPermissionsAlias(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	d := commands.NewDispatcher(rt)

	_ = handleCmd(t, d, "/perm read_only")
	if rt.permMode != "read_only" {
		t.Errorf("permMode = %q", rt.permMode)
	}

	result := handleCmd(t, d, "/permissions")
	if !strings.Contains(result.Message, "read_only") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestE2ECmdClearAndCost(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	d := commands.NewDispatcher(rt)

	_ = handleCmd(t, d, "/cost")
	_ = handleCmd(t, d, "/clear")
	if rt.msgCount != 0 {
		t.Errorf("msgCount = %d after clear", rt.msgCount)
	}

	result := handleCmd(t, d, "/cost")
	if !strings.Contains(result.Message, "100") || !strings.Contains(result.Message, "0.05") {
		t.Errorf("cost after clear: %q", result.Message)
	}
}

func TestE2ECmdCompact(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/compact")
	if !strings.Contains(result.Message, "compacted") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestE2ECmdVersionAlias(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/v")
	if !strings.Contains(result.Message, "v1.0.0") {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestE2ECmdQuitAction(t *testing.T) {
	// quit/exit are not in Specs, so the Dispatcher treats them as unknown commands.
	// They are handled directly in the REPL loop, not through the Dispatcher.
	// This test verifies they're recognized as unknown by the Dispatcher,
	// since the REPL intercepts them before reaching the Dispatcher.
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	for _, input := range []string{"/quit", "/exit"} {
		result := handleCmd(t, d, input)
		// Dispatcher doesn't have quit/exit in Specs, so they get "Unknown command"
		if result.Action == "quit" {
			// This would be unexpected but ok
			continue
		}
		if !strings.Contains(result.Message, "Unknown command") {
			t.Errorf("Handle(%q) should return unknown: Action=%q Message=%q", input, result.Action, result.Message)
		}
	}
}

func TestE2ECmdConfigShowAndSet(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	d := commands.NewDispatcher(rt)

	// Show config
	result := handleCmd(t, d, "/config")
	if !strings.Contains(result.Message, "model") {
		t.Errorf("config show: %q", result.Message)
	}

	// Set config
	result = handleCmd(t, d, "/config model grok-3")
	if !strings.Contains(result.Message, "Set") {
		t.Errorf("config set: %q", result.Message)
	}
}

// --- Unknown Command Handling ---

func TestE2ECmdUnknownWithSuggestion(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/hlep")
	if !strings.Contains(result.Message, "Did you mean") {
		t.Errorf("Message = %q, should suggest alternatives", result.Message)
	}
}

func TestE2ECmdUnknownNoSuggestion(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/xyzqwerty")
	if !strings.Contains(result.Message, "Unknown command") {
		t.Errorf("Message = %q", result.Message)
	}
}

// --- FS-dependent Commands ---

func TestE2ECmdInitCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/init")
	if !strings.Contains(result.Message, "Initialized") {
		t.Errorf("Message = %q", result.Message)
	}

	// Verify directory structure
	dirs := []string{".glaw", ".glaw/sessions", ".glaw/agents", ".glaw/skills", ".glaw/plugins", ".glaw/memory", ".glaw/exports"}
	for _, d := range dirs {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("directory %q not created", d)
		}
	}

	// Verify settings.json
	data, err := os.ReadFile(".glaw/settings.json")
	if err != nil {
		t.Fatalf("settings.json not found: %v", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("invalid settings.json: %v", err)
	}
	if settings["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v", settings["model"])
	}
}

func TestE2ECmdMemoryWorkflow(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".glaw", "memory"), 0o755); err != nil { t.Fatal(err) }
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))

	// Add
	result := handleCmd(t, d, "/memory add testname This is test content")
	if !strings.Contains(result.Message, "saved") {
		t.Errorf("add: %q", result.Message)
	}

	// Read
	result = handleCmd(t, d, "/memory read testname")
	if result.Message != "This is test content" {
		t.Errorf("read: %q", result.Message)
	}

	// List
	result = handleCmd(t, d, "/memory")
	if !strings.Contains(result.Message, "testname.md") {
		t.Errorf("list: %q", result.Message)
	}

	// Delete
	result = handleCmd(t, d, "/memory delete testname")
	if !strings.Contains(result.Message, "deleted") {
		t.Errorf("delete: %q", result.Message)
	}

	// Verify deleted
	result = handleCmd(t, d, "/memory read testname")
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("after delete: %q", result.Message)
	}
}

func TestE2ECmdMemoryListEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".glaw", "memory"), 0o755); err != nil { t.Fatal(err) }
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/memory")
	if !strings.Contains(result.Message, "No memory") {
		t.Errorf("empty list: %q", result.Message)
	}
}

func TestE2ECmdMemoryNoDir(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/memory")
	if !strings.Contains(result.Message, "No memory") {
		t.Errorf("no dir: %q", result.Message)
	}
}

func TestE2ECmdSessionWorkflow(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, ".glaw", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(sessionsDir, "sess_test.json"), []byte(`{"id":"sess_test"}`), 0o644); err != nil { t.Fatal(err) }
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))

	// List
	result := handleCmd(t, d, "/session list")
	if !strings.Contains(result.Message, "sess_test") {
		t.Errorf("list: %q", result.Message)
	}

	// Delete
	result = handleCmd(t, d, "/session delete sess_test")
	if !strings.Contains(result.Message, "deleted") {
		t.Errorf("delete: %q", result.Message)
	}

	// Verify file removed
	if _, err := os.Stat(filepath.Join(sessionsDir, "sess_test.json")); !os.IsNotExist(err) {
		t.Error("session file should be deleted")
	}
}

func TestE2ECmdSessionNoArgs(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/session")
	if !strings.Contains(result.Message, "sess_test") || !strings.Contains(result.Message, "5") {
		t.Errorf("session info: %q", result.Message)
	}
}

func TestE2ECmdExportCreatesFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/export")
	if !strings.Contains(result.Message, "exported") {
		t.Errorf("export: %q", result.Message)
	}

	// Find the export file
	exportsDir := filepath.Join(dir, ".glaw", "exports")
	entries, err := os.ReadDir(exportsDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no export files: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(exportsDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	var export map[string]interface{}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatalf("parse export: %v", err)
	}

	if export["session_id"] != "sess_test" {
		t.Errorf("session_id = %v", export["session_id"])
	}
	if export["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v", export["model"])
	}
	if _, ok := export["exported_at"]; !ok {
		t.Error("missing exported_at")
	}
}

func TestE2ECmdExportWithFilename(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/export my-export.json")
	if !strings.Contains(result.Message, "my-export.json") {
		t.Errorf("export with filename: %q", result.Message)
	}
}

// --- Diff ---

func TestE2ECmdDiffWithOutput(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	rt.gitOutput = "fake diff output"
	d := commands.NewDispatcher(rt)

	result := handleCmd(t, d, "/diff")
	if !strings.Contains(result.Message, "fake diff") {
		t.Errorf("diff: %q", result.Message)
	}
}

func TestE2ECmdDiffNoChanges(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	rt.gitOutput = ""
	d := commands.NewDispatcher(rt)

	result := handleCmd(t, d, "/diff")
	if !strings.Contains(result.Message, "No pending changes") {
		t.Errorf("diff no changes: %q", result.Message)
	}
}

// --- Skills and Agents ---

func TestE2ECmdSkillsList(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/skills")

	wantSkills := []string{"commit", "review-pr", "pdf", "context7-mcp", "claude-api", "simplify"}
	for _, skill := range wantSkills {
		if !strings.Contains(result.Message, skill) {
			t.Errorf("skills missing %q: %s", skill, result.Message)
		}
	}
}

func TestE2ECmdAgentsList(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/agents")

	wantAgents := []string{"general-purpose", "Explore", "Plan"}
	for _, agent := range wantAgents {
		if !strings.Contains(result.Message, agent) {
			t.Errorf("agents missing %q: %s", agent, result.Message)
		}
	}
}

// --- Bughunter ---

func TestE2ECmdBughunter(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/bughunter main.go")
	if !strings.Contains(result.Message, "Bughunter mode activated") {
		t.Errorf("bughunter: %q", result.Message)
	}
	if !strings.Contains(result.Message, "main.go") {
		t.Errorf("bughunter should mention target: %q", result.Message)
	}
}

func TestE2ECmdBughunterNoTarget(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/bughunter")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("bughunter no target: %q", result.Message)
	}
}

// --- Debug Tool Call ---

func TestE2ECmdDebugToolCall(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))

	result := handleCmd(t, d, "/debug-tool-call on")
	if !strings.Contains(result.Message, "enabled") {
		t.Errorf("debug on: %q", result.Message)
	}

	result = handleCmd(t, d, "/debug-tool-call off")
	if !strings.Contains(result.Message, "disabled") {
		t.Errorf("debug off: %q", result.Message)
	}

	result = handleCmd(t, d, "/debug-tool-call")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("debug no arg: %q", result.Message)
	}
}

// --- Ultraplan ---

func TestE2ECmdUltraplan(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/ultraplan Build a REST API")
	if !strings.Contains(result.Message, "Ultraplan mode activated") {
		t.Errorf("ultraplan: %q", result.Message)
	}
	if !strings.Contains(result.Message, "Build a REST API") {
		t.Errorf("ultraplan should include task: %q", result.Message)
	}
}

// --- Teleport ---

func TestE2ECmdTeleportNoArg(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/teleport")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("teleport no arg: %q", result.Message)
	}
}

func TestE2ECmdTeleportWithArg(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/teleport localhost:8080")
	if !strings.Contains(result.Message, "localhost:8080") {
		t.Errorf("teleport: %q", result.Message)
	}
}

// --- Resume ---

func TestE2ECmdResumeListSessions(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, ".glaw", "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(sessionsDir, "abc123.json"), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
		chdir(t, dir)

		d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/resume")
	if !strings.Contains(result.Message, "abc123") {
		t.Errorf("resume list: %q", result.Message)
	}
}

func TestE2ECmdResumeWithID(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/resume my-session-id")
	if !strings.Contains(result.Message, "my-session-id") {
		t.Errorf("resume with id: %q", result.Message)
	}
	if !strings.Contains(result.Message, "glaw-code --session") {
		t.Errorf("should show restart command: %q", result.Message)
	}
}

// --- Plugin ---

func TestE2ECmdPluginNoArgs(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/plugin")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("plugin no args: %q", result.Message)
	}
}

func TestE2ECmdPluginListEmpty(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/plugin list")
	if !strings.Contains(result.Message, "No plugins") {
		t.Errorf("plugin list empty: %q", result.Message)
	}
}

func TestE2ECmdPluginEnableDisable(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))

	result := handleCmd(t, d, "/plugin enable myplugin")
	if !strings.Contains(result.Message, "enabled") {
		t.Errorf("plugin enable: %q", result.Message)
	}

	result = handleCmd(t, d, "/plugin disable myplugin")
	if !strings.Contains(result.Message, "disabled") {
		t.Errorf("plugin disable: %q", result.Message)
	}
}

func TestE2ECmdPluginInstallWithManifest(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Create a manifest file
	manifestPath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(`{"name":"test-plugin","version":"1.0.0"}`), 0o644); err != nil {
			t.Fatal(err)
		}

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	result := handleCmd(t, d, "/plugin install "+manifestPath)
	if !strings.Contains(result.Message, "installed") {
		t.Errorf("plugin install: %q", result.Message)
	}

	// Verify it was installed
	installedManifest := filepath.Join(dir, ".glaw", "plugins", "test-plugin", "manifest.json")
	if _, err := os.Stat(installedManifest); os.IsNotExist(err) {
		t.Error("plugin manifest should be installed")
	}
}

// --- Branch ---

func TestE2ECmdBranchList(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	rt.gitOutput = "* main\n  feature-branch"
	d := commands.NewDispatcher(rt)

	result := handleCmd(t, d, "/branch")
	if !strings.Contains(result.Message, "main") {
		t.Errorf("branch list: %q", result.Message)
	}
}

func TestE2ECmdBranchCreate(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	rt.gitOutput = "Switched to branch 'feature'"
	d := commands.NewDispatcher(rt)

	result := handleCmd(t, d, "/branch create feature")
	if !strings.Contains(result.Message, "Created branch") {
		t.Errorf("branch create: %q", result.Message)
	}
}

func TestE2ECmdBranchNoName(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/branch create")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("branch no name: %q", result.Message)
	}
}

// --- Worktree ---

func TestE2ECmdWorktreeList(t *testing.T) {
	rt := newMockRuntimeFS(t.TempDir())
	rt.gitOutput = "/main  abc123  [main]"
	d := commands.NewDispatcher(rt)

	result := handleCmd(t, d, "/worktree")
	if !strings.Contains(result.Message, "main") {
		t.Errorf("worktree list: %q", result.Message)
	}
}

func TestE2ECmdWorktreeNoName(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/worktree create")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("worktree no name: %q", result.Message)
	}
}

// --- Parse All Specs ---

func TestE2ECmdParseAllSpecs(t *testing.T) {
	for _, spec := range commands.Specs {
		parsed, _ := commands.Parse("/" + spec.Name)
		if parsed == nil {
			t.Errorf("Parse(%q) returned nil", "/"+spec.Name)
			continue
		}
		if parsed.Spec.Name != spec.Name {
			t.Errorf("Parse(%q).Name = %q, want %q", "/"+spec.Name, parsed.Spec.Name, spec.Name)
		}
	}
}

func TestE2ECmdParseAliases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/h", "help"},
		{"/?", "help"},
		{"/st", "status"},
		{"/v", "version"},
		{"/perm", "permissions"},
		{"/cpp", "commit-push-pr"},
	}
	for _, tt := range tests {
		parsed, _ := commands.Parse(tt.input)
		if parsed == nil {
			t.Errorf("Parse(%q) returned nil", tt.input)
			continue
		}
		if parsed.Spec.Name != tt.want {
			t.Errorf("Parse(%q).Name = %q, want %q", tt.input, parsed.Spec.Name, tt.want)
		}
	}
}
