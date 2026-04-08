package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
func (m *mockRuntimeFS) IsYoloMode() bool                    { return m.permMode == "yolo" }
func (m *mockRuntimeFS) ToggleYoloMode() bool {
	if m.permMode == "yolo" {
		m.permMode = "workspace_write"
		return false
	}
	m.permMode = "yolo"
	return true
}
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
func (m *mockRuntimeFS) RevertLastTurn() (int, error)            { return 0, nil }
func (m *mockRuntimeFS) RevertAll() (int, error)                 { return 0, nil }
func (m *mockRuntimeFS) LoadSession(sessionID string) error      { m.sessionID = sessionID; return nil }
func (m *mockRuntimeFS) NewSession()                              { m.sessionID = "sess_new"; m.msgCount = 0 }
func (m *mockRuntimeFS) GetSubAgentSessions() []commands.SubAgentSessionInfo { return nil }
func (m *mockRuntimeFS) ResumeSubAgentSession(agentID string) error {
	m.sessionID = agentID
	return nil
}

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
	if err := os.WriteFile(filepath.Join(sessionsDir, "sess_other.json"), []byte(`{"id":"sess_other"}`), 0o644); err != nil { t.Fatal(err) }
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))

	// List
	result := handleCmd(t, d, "/session list")
	if !strings.Contains(result.Message, "sess_test") {
		t.Errorf("list missing sess_test: %q", result.Message)
	}
	if !strings.Contains(result.Message, "sess_other") {
		t.Errorf("list missing sess_other: %q", result.Message)
	}

	// Delete a non-current session
	result = handleCmd(t, d, "/session delete sess_other")
	if !strings.Contains(result.Message, "deleted") {
		t.Errorf("delete: %q", result.Message)
	}

	// Verify file removed
	if _, err := os.Stat(filepath.Join(sessionsDir, "sess_other.json")); !os.IsNotExist(err) {
		t.Error("session file should be deleted")
	}

	// Cannot delete the current session
	result = handleCmd(t, d, "/session delete sess_test")
	if !strings.Contains(result.Message, "Cannot delete the current session") {
		t.Errorf("should prevent deleting current: %q", result.Message)
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
	// Create a temp dir with skill files to simulate real skill loading
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".glaw", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create sample skill files
	skillFiles := map[string]string{
		"commit.md":    "# Create a git commit\nCreate a git commit with staged changes",
		"review-pr.md": "# Review a PR\nReview a pull request",
		"pdf.md":       "# Read PDF files\nRead and analyze PDF files",
	}
	for name, content := range skillFiles {
		if err := os.WriteFile(filepath.Join(skillsDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Change to the temp dir so .glaw/skills is found
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	d := commands.NewDispatcher(newMockRuntimeFS(tmpDir))
	result := handleCmd(t, d, "/skills")

	// Verify that skills loaded from the temp directory appear
	if !strings.Contains(result.Message, "commit") {
		t.Errorf("skills missing 'commit': %s", result.Message)
	}
	if !strings.Contains(result.Message, "review-pr") {
		t.Errorf("skills missing 'review-pr': %s", result.Message)
	}
	if !strings.Contains(result.Message, "pdf") {
		t.Errorf("skills missing 'pdf': %s", result.Message)
	}
}

func TestE2ECmdAgentsList(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents")

	wantAgents := []string{"general-purpose", "Explore", "Plan", "Verification", "code-reviewer", "security-auditor", "test-writer", "docs-writer", "refactorer"}
	for _, agent := range wantAgents {
		if !strings.Contains(result.Message, agent) {
			t.Errorf("agents missing %q: %s", agent, result.Message)
		}
	}
}

func TestE2ECmdAgentsListSubcommand(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents list")

	wantAgents := []string{"general-purpose", "Explore", "Plan"}
	for _, agent := range wantAgents {
		if !strings.Contains(result.Message, agent) {
			t.Errorf("agents list missing %q: %s", agent, result.Message)
		}
	}
}

func TestE2ECmdAgentsShow(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})

	// Show built-in agent
	result := handleCmd(t, d, "/agents show Explore")
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("show Explore: %q", result.Message)
	}
	if !strings.Contains(result.Message, "builtin") {
		t.Errorf("show should show source: %q", result.Message)
	}

	// Show non-existent agent
	result = handleCmd(t, d, "/agents show nonexistent")
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("show nonexistent: %q", result.Message)
	}
}

func TestE2ECmdAgentsShowNoName(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents show")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("show no name: %q", result.Message)
	}
}

func TestE2ECmdAgentsCreate(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents create my-test-agent --desc \"A test agent\" --tools read_file,bash --model sonnet")

	if !strings.Contains(result.Message, "created successfully") {
		t.Errorf("create: %q", result.Message)
	}
	if !strings.Contains(result.Message, "my-test-agent") {
		t.Errorf("create should mention agent name: %q", result.Message)
	}

	// Verify file was created
	agentFile := filepath.Join(agentsDir, "my-test-agent.md")
	if _, err := os.Stat(agentFile); os.IsNotExist(err) {
		t.Error("agent file should be created")
	}
}

func TestE2ECmdAgentsCreateNoName(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents create")
	if !strings.Contains(result.Message, "Usage") && !strings.Contains(result.Message, "Options") {
		t.Errorf("create no name: %q", result.Message)
	}
}

func TestE2ECmdAgentsDelete(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create an agent file to delete
	agentFile := filepath.Join(agentsDir, "to-delete.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: to-delete\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents delete to-delete")

	if !strings.Contains(result.Message, "deleted") {
		t.Errorf("delete: %q", result.Message)
	}

	// Verify file was removed
	if _, err := os.Stat(agentFile); !os.IsNotExist(err) {
		t.Error("agent file should be deleted")
	}
}

func TestE2ECmdAgentsDeleteBuiltin(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents delete Explore")

	if !strings.Contains(result.Message, "built-in") || !strings.Contains(result.Message, "cannot be deleted") {
		t.Errorf("delete builtin should be prevented: %q", result.Message)
	}
}

func TestE2ECmdAgentsCallNoArgs(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents call")
	if !strings.Contains(result.Message, "Usage") {
		t.Errorf("call no args: %q", result.Message)
	}
}

func TestE2ECmdAgentsNoProvider(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	result := handleCmd(t, d, "/agents")
	if !strings.Contains(result.Message, "No agent provider configured") {
		t.Errorf("agents without provider: %q", result.Message)
	}
}

func TestE2ECmdAgentsUnknownSubcommand(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents foobar")
	if !strings.Contains(result.Message, "Agent Management") {
		t.Errorf("unknown subcommand should show help: %q", result.Message)
	}
}

func TestE2ECmdAgentsCreateWithQuotedDescription(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	d.SetAgentsProvider(&agentAgentsProviderStub{})

	// Test quoted description
	result := handleCmd(t, d, `/agents create quoted-agent --desc "This is a quoted description" --tools read_file,bash --model sonnet`)
	if !strings.Contains(result.Message, "created successfully") {
		t.Errorf("create with quoted desc: %q", result.Message)
	}
	if !strings.Contains(result.Message, "This is a quoted description") {
		t.Errorf("should preserve full description: %q", result.Message)
	}

	// Verify file contents
	data, err := os.ReadFile(filepath.Join(agentsDir, "quoted-agent.md"))
	if err != nil {
		t.Fatalf("agent file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "This is a quoted description") {
		t.Errorf("file should contain full description: %q", content)
	}
	if !strings.Contains(content, "tools: read_file, bash") {
		t.Errorf("file should contain tools: %q", content)
	}
	if !strings.Contains(content, "model: sonnet") {
		t.Errorf("file should contain model: %q", content)
	}
}

func TestE2ECmdAgentsCallWithQuotedPrompt(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})

	// Test /agents call with a quoted prompt (non-blocking by default)
	result := handleCmd(t, d, `/agents call Explore "Search for all error handling patterns"`)
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("call result should mention agent name: %q", result.Message)
	}
	if !strings.Contains(result.Message, "spawned in background") {
		t.Errorf("call result should mention background spawn: %q", result.Message)
	}
	if !strings.Contains(result.Message, "Job ID:") {
		t.Errorf("call result should include job ID: %q", result.Message)
	}
}

func TestE2ECmdAgentsCallWithWait(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})

	// Test /agents call with --wait flag (blocking mode)
	result := handleCmd(t, d, `/agents call Explore "Search for patterns" --wait`)
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("call --wait result should mention agent name: %q", result.Message)
	}
	if !strings.Contains(result.Message, "result:") {
		t.Errorf("call --wait result should include result: %q", result.Message)
	}
}

func TestE2ECmdAgentsEditWorkflow(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create an agent to edit
	agentContent := "---\nname: edit-me\ndescription: Original desc\ntools: read_file\nmodel: inherit\n---\n\nOriginal prompt.\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "edit-me.md"), []byte(agentContent), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	d := commands.NewDispatcher(newMockRuntimeFS(dir))
	d.SetAgentsProvider(&agentAgentsProviderStub{})

	// Verify original agent shows up
	result := handleCmd(t, d, "/agents show edit-me")
	if !strings.Contains(result.Message, "Original desc") {
		t.Errorf("show before edit: %q", result.Message)
	}

	// Edit the agent
	result = handleCmd(t, d, `/agents edit edit-me --desc "Updated description" --model sonnet`)
	if !strings.Contains(result.Message, "updated successfully") {
		t.Errorf("edit: %q", result.Message)
	}

	// Verify updated description in show
	result = handleCmd(t, d, "/agents show edit-me")
	if !strings.Contains(result.Message, "Updated description") {
		t.Errorf("show after edit should have new desc: %q", result.Message)
	}

	// Verify the file on disk has the correct content
	data, err := os.ReadFile(filepath.Join(agentsDir, "edit-me.md"))
	if err != nil {
		t.Fatalf("agent file not found after edit: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Updated description") {
		t.Errorf("file should have new description: %q", content)
	}
	if !strings.Contains(content, "model: sonnet") {
		t.Errorf("file should have model sonnet: %q", content)
	}
}

func TestE2ECmdAgentsLsAlias(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents ls")
	if !strings.Contains(result.Message, "Available Agents") {
		t.Errorf("/agents ls should list agents: %q", result.Message)
	}
}

func TestE2ECmdAgentsShowInfoAlias(t *testing.T) {
	d := commands.NewDispatcher(newMockRuntimeFS(t.TempDir()))
	d.SetAgentsProvider(&agentAgentsProviderStub{})
	result := handleCmd(t, d, "/agents info Explore")
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("/agents info: %q", result.Message)
	}
}

// agentAgentsProviderStub is a test stub for commands.AgentsProvider
// that returns the same built-in agents as the real adapter.
// It also reads/writes agent files from disk for create/delete/get operations.
type agentAgentsProviderStub struct{}

func (a *agentAgentsProviderStub) ListAgents(workspaceRoot string) ([]commands.AgentInfo, error) {
	result := []commands.AgentInfo{
		{Name: "Explore", Description: "Fast agent for exploring codebases", Source: "builtin", Tools: []string{"read_file", "glob_search", "grep_search"}, Model: "inherit"},
		{Name: "Plan", Description: "Software architect agent", Source: "builtin", Tools: []string{"read_file", "glob_search", "grep_search", "todo_write"}, Model: "inherit"},
		{Name: "Verification", Description: "Test runner agent", Source: "builtin", Tools: []string{"read_file", "glob_search", "grep_search", "bash"}, Model: "inherit"},
		{Name: "code-reviewer", Description: "Code review agent", Source: "builtin", Tools: []string{"read_file", "glob_search", "grep_search"}, Model: "inherit"},
		{Name: "security-auditor", Description: "Security audit agent", Source: "builtin", Tools: []string{"read_file", "glob_search", "grep_search"}, Model: "inherit"},
		{Name: "test-writer", Description: "Test writing agent", Source: "builtin", Tools: []string{"read_file", "write_file", "edit_file", "bash"}, Model: "inherit"},
		{Name: "docs-writer", Description: "Documentation agent", Source: "builtin", Tools: []string{"read_file", "write_file", "edit_file"}, Model: "inherit"},
		{Name: "refactorer", Description: "Code refactoring agent", Source: "builtin", Tools: []string{"read_file", "write_file", "edit_file", "bash"}, Model: "inherit"},
		{Name: "general-purpose", Description: "General-purpose agent", Source: "builtin", Tools: []string{"bash", "read_file", "write_file", "edit_file"}, Model: "inherit"},
	}

	// Also load custom agents from disk
	if workspaceRoot != "" {
		agentsDir := filepath.Join(workspaceRoot, ".glaw", "agents")
		if entries, err := os.ReadDir(agentsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".md")
				data, err := os.ReadFile(filepath.Join(agentsDir, e.Name()))
				if err != nil {
					continue
				}
				// Simple parsing of frontmatter for description
				desc := "Custom agent"
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "description:") {
						desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
						break
					}
				}
				result = append(result, commands.AgentInfo{
					Name:        name,
					Description: desc,
					Source:      "project",
				})
			}
		}
	}

	return result, nil
}

func (a *agentAgentsProviderStub) GetAgent(workspaceRoot, name string) (*commands.AgentInfo, error) {
	// Check built-in first
	agents, _ := a.ListAgents(workspaceRoot)
	for i := range agents {
		if agents[i].Name == name {
			return &agents[i], nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", name)
}

func (a *agentAgentsProviderStub) CreateAgent(workspaceRoot, name, description, scope string, tools []string, model, prompt string) error {
	dir := filepath.Join(workspaceRoot, ".glaw", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("name: %s\n", name))
	buf.WriteString(fmt.Sprintf("description: %s\n", description))
	if len(tools) > 0 {
		buf.WriteString(fmt.Sprintf("tools: %s\n", strings.Join(tools, ", ")))
	}
	if model != "" {
		buf.WriteString(fmt.Sprintf("model: %s\n", model))
	}
	buf.WriteString("---\n\n")
	buf.WriteString(prompt)
	buf.WriteString("\n")

	return os.WriteFile(filepath.Join(dir, name+".md"), buf.Bytes(), 0o644)
}

func (a *agentAgentsProviderStub) DeleteAgent(workspaceRoot, name, scope string) error {
	path := filepath.Join(workspaceRoot, ".glaw", "agents", name+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("agent %q not found", name)
	}
	return os.Remove(path)
}

func (a *agentAgentsProviderStub) CallAgent(ctx context.Context, name, prompt string) (string, error) {
	// Stub: return a mock response
	return fmt.Sprintf("Agent %q executed with prompt: %s", name, prompt), nil
}

func (a *agentAgentsProviderStub) CallAgentBackground(ctx context.Context, name, prompt string) (string, error) {
	return fmt.Sprintf("agent-stub-%d", time.Now().UnixMilli()), nil
}

func (a *agentAgentsProviderStub) GetAgentJobStatus(jobID string) (*commands.AgentJobStatus, error) {
	return &commands.AgentJobStatus{
		ID:        jobID,
		AgentType: "general-purpose",
		Status:    "completed",
		Prompt:    "stub prompt",
	}, nil
}

func (a *agentAgentsProviderStub) ListAgentJobs() []*commands.AgentJobStatus {
	return nil
}

func (a *agentAgentsProviderStub) WaitAgentJob(jobID string) (string, error) {
	return fmt.Sprintf("Stub result for job %s", jobID), nil
}

func (a *agentAgentsProviderStub) CancelAgentJob(jobID string) error {
	return nil
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
	// Now uses in-REPL switching instead of restart message
	if !strings.Contains(result.Message, "Resumed session") {
		t.Errorf("should confirm session resume: %q", result.Message)
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
