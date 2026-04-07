package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/commands"
)

// TestAgentsProviderAdapterListAgents tests listing built-in and custom agents.
func TestAgentsProviderAdapterListAgents(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	agents, err := adapter.ListAgents(dir)
	if err != nil {
		t.Fatalf("ListAgents error: %v", err)
	}

	// Should have all 9 built-in agents
	if len(agents) < 9 {
		t.Errorf("expected at least 9 agents, got %d", len(agents))
	}

	// Verify built-in agents are present
	builtinNames := map[string]bool{}
	for _, a := range agents {
		if a.Source == "builtin" {
			builtinNames[a.Name] = true
		}
	}
	wantBuiltin := []string{"Explore", "Plan", "Verification", "code-reviewer", "security-auditor", "test-writer", "docs-writer", "refactorer", "general-purpose"}
	for _, name := range wantBuiltin {
		if !builtinNames[name] {
			t.Errorf("missing built-in agent %q", name)
		}
	}
}

// TestAgentsProviderAdapterListAgentsWithCustom tests listing with custom agents on disk.
func TestAgentsProviderAdapterListAgentsWithCustom(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a custom agent file
	customAgent := `---
name: my-custom-agent
description: A custom test agent
tools: read_file, bash
model: sonnet
---
You are a custom test agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "my-custom-agent.md"), []byte(customAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewAgentsProviderAdapter(nil)
	agents, err := adapter.ListAgents(dir)
	if err != nil {
		t.Fatalf("ListAgents error: %v", err)
	}

	// Find the custom agent
	found := false
	for _, a := range agents {
		if a.Name == "my-custom-agent" {
			found = true
			if a.Source != "project" {
				t.Errorf("expected source 'project', got %q", a.Source)
			}
			if a.Description != "A custom test agent" {
				t.Errorf("description = %q", a.Description)
			}
			break
		}
	}
	if !found {
		t.Error("custom agent not found in list")
	}
}

// TestAgentsProviderAdapterGetAgent tests retrieving individual agents.
func TestAgentsProviderAdapterGetAgent(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	// Get a built-in agent
	agent, err := adapter.GetAgent(dir, "Explore")
	if err != nil {
		t.Fatalf("GetAgent error: %v", err)
	}
	if agent.Name != "Explore" {
		t.Errorf("Name = %q, want 'Explore'", agent.Name)
	}
	if agent.Source != "builtin" {
		t.Errorf("Source = %q, want 'builtin'", agent.Source)
	}
	if len(agent.Tools) == 0 {
		t.Error("expected Tools to be populated for Explore agent")
	}
	if agent.Prompt == "" {
		t.Error("expected Prompt to be populated for Explore agent")
	}

	// Get a non-existent agent
	agent, err = adapter.GetAgent(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
	if agent != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

// TestAgentsProviderAdapterGetCustomAgent tests retrieving a custom agent.
func TestAgentsProviderAdapterGetCustomAgent(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	customAgent := `---
name: db-reviewer
description: Database code review specialist
tools: read_file, grep_search
model: haiku
---
You review database-related code.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "db-reviewer.md"), []byte(customAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewAgentsProviderAdapter(nil)
	agent, err := adapter.GetAgent(dir, "db-reviewer")
	if err != nil {
		t.Fatalf("GetAgent error: %v", err)
	}
	if agent.Name != "db-reviewer" {
		t.Errorf("Name = %q", agent.Name)
	}
	if agent.Source != "project" {
		t.Errorf("Source = %q", agent.Source)
	}
	if agent.Description != "Database code review specialist" {
		t.Errorf("Description = %q", agent.Description)
	}
	if len(agent.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d: %v", len(agent.Tools), agent.Tools)
	}
	if agent.Model != "haiku" {
		t.Errorf("Model = %q", agent.Model)
	}
	if !strings.Contains(agent.Prompt, "review database-related code") {
		t.Errorf("Prompt = %q", agent.Prompt)
	}
}

// TestAgentsProviderAdapterCreateAgent tests creating agent files.
func TestAgentsProviderAdapterCreateAgent(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	// Create project-scoped agent
	err := adapter.CreateAgent(dir, "api-tester", "API testing agent", "project",
		[]string{"read_file", "bash", "web_fetch"}, "sonnet", "You test APIs thoroughly.")
	if err != nil {
		t.Fatalf("CreateAgent error: %v", err)
	}

	// Verify file exists
	agentFile := filepath.Join(dir, ".glaw", "agents", "api-tester.md")
	data, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("agent file not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "name: api-tester") {
		t.Errorf("file missing name: %q", content)
	}
	if !strings.Contains(content, "description: API testing agent") {
		t.Errorf("file missing description: %q", content)
	}
	if !strings.Contains(content, "tools:") {
		t.Errorf("file missing tools: %q", content)
	}
	if !strings.Contains(content, "model: sonnet") {
		t.Errorf("file missing model: %q", content)
	}
	if !strings.Contains(content, "You test APIs thoroughly.") {
		t.Errorf("file missing prompt: %q", content)
	}

	// Now verify we can load it back
	agent, err := adapter.GetAgent(dir, "api-tester")
	if err != nil {
		t.Fatalf("GetAgent after create error: %v", err)
	}
	if agent.Name != "api-tester" {
		t.Errorf("Name = %q", agent.Name)
	}
	if agent.Source != "project" {
		t.Errorf("Source = %q", agent.Source)
	}
}

// TestAgentsProviderAdapterCreateAgentUserScope tests creating a user-scoped agent.
func TestAgentsProviderAdapterCreateAgentUserScope(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	// Create user-scoped agent (goes to ~/.glaw/agents/)
	// Note: In tests, this will write to the actual home directory,
	// so we use a unique name and clean up.
	agentName := "test-user-scope-agent"
	err := adapter.CreateAgent(dir, agentName, "User scope test", "user",
		nil, "", "Test prompt")
	if err != nil {
		t.Fatalf("CreateAgent error: %v", err)
	}

	// Clean up
	home, _ := os.UserHomeDir()
	agentFile := filepath.Join(home, ".glaw", "agents", agentName+".md")
	defer os.Remove(agentFile)

	// Verify file was created in user dir
	if _, err := os.Stat(agentFile); err != nil {
		t.Errorf("user-scoped agent file not created: %v", err)
	}
}

// TestAgentsProviderAdapterCreateAgentNoName tests validation.
func TestAgentsProviderAdapterCreateAgentNoName(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	err := adapter.CreateAgent(dir, "", "desc", "project", nil, "", "")
	if err == nil {
		t.Error("expected error for empty name")
	}
}

// TestAgentsProviderAdapterDeleteAgent tests deleting agent files.
func TestAgentsProviderAdapterDeleteAgent(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create an agent to delete
	agentFile := filepath.Join(agentsDir, "to-delete.md")
	if err := os.WriteFile(agentFile, []byte("---\nname: to-delete\ndescription: test\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewAgentsProviderAdapter(nil)
	err := adapter.DeleteAgent(dir, "to-delete", "project")
	if err != nil {
		t.Fatalf("DeleteAgent error: %v", err)
	}

	// Verify file removed
	if _, err := os.Stat(agentFile); !os.IsNotExist(err) {
		t.Error("agent file should be deleted")
	}
}

// TestAgentsProviderAdapterDeleteAgentNotFound tests deleting non-existent agent.
func TestAgentsProviderAdapterDeleteAgentNotFound(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	err := adapter.DeleteAgent(dir, "nonexistent", "project")
	if err == nil {
		t.Error("expected error for deleting non-existent agent")
	}
}

// TestAgentsProviderAdapterCallAgentWithoutManager tests that CallAgent fails gracefully.
func TestAgentsProviderAdapterCallAgentWithoutManager(t *testing.T) {
	adapter := NewAgentsProviderAdapter(nil) // nil manager

	_, err := adapter.CallAgent(context.Background(), "Explore", "test prompt")
	if err == nil {
		t.Error("expected error when calling agent without manager")
	}
	if !strings.Contains(err.Error(), "not available") {
		t.Errorf("error = %v", err)
	}
}

// TestAgentsProviderAdapterFullWorkflow tests the complete create -> show -> list -> edit -> delete workflow.
func TestAgentsProviderAdapterFullWorkflow(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	// Step 1: Create a custom agent
	err := adapter.CreateAgent(dir, "workflow-agent", "Workflow test agent",
		"project", []string{"read_file", "bash"}, "sonnet",
		"You are a workflow test agent.")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Step 2: Show it
	agent, err := adapter.GetAgent(dir, "workflow-agent")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if agent.Name != "workflow-agent" {
		t.Errorf("Name = %q", agent.Name)
	}
	if agent.Description != "Workflow test agent" {
		t.Errorf("Description = %q", agent.Description)
	}
	if len(agent.Tools) != 2 {
		t.Errorf("Tools = %v", agent.Tools)
	}

	// Step 3: List agents - should include our custom one
	agents, err := adapter.ListAgents(dir)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "workflow-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("custom agent not in list")
	}

	// Step 4: Delete it
	err = adapter.DeleteAgent(dir, "workflow-agent", "project")
	if err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}

	// Step 5: Verify it's gone
	_, err = adapter.GetAgent(dir, "workflow-agent")
	if err == nil {
		t.Error("agent should be deleted")
	}

	// Step 6: List should no longer include it
	agents, err = adapter.ListAgents(dir)
	if err != nil {
		t.Fatalf("ListAgents after delete: %v", err)
	}
	for _, a := range agents {
		if a.Name == "workflow-agent" {
			t.Error("workflow-agent should not be in list after deletion")
		}
	}
}

// TestAgentsProviderAdapterEditViaDeleteAndRecreate tests the edit pattern.
func TestAgentsProviderAdapterEditViaDeleteAndRecreate(t *testing.T) {
	dir := t.TempDir()
	adapter := NewAgentsProviderAdapter(nil)

	// Create
	err := adapter.CreateAgent(dir, "edit-test", "Original description",
		"project", []string{"read_file"}, "inherit", "Original prompt")
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	// Verify original
	agent, err := adapter.GetAgent(dir, "edit-test")
	if err != nil {
		t.Fatalf("GetAgent original: %v", err)
	}
	if agent.Description != "Original description" {
		t.Errorf("original Description = %q", agent.Description)
	}

	// Edit (delete + recreate with new values)
	err = adapter.DeleteAgent(dir, "edit-test", "project")
	if err != nil {
		t.Fatalf("DeleteAgent for edit: %v", err)
	}
	err = adapter.CreateAgent(dir, "edit-test", "Updated description",
		"project", []string{"read_file", "bash"}, "sonnet", "Updated prompt")
	if err != nil {
		t.Fatalf("CreateAgent for edit: %v", err)
	}

	// Verify updated
	agent, err = adapter.GetAgent(dir, "edit-test")
	if err != nil {
		t.Fatalf("GetAgent updated: %v", err)
	}
	if agent.Description != "Updated description" {
		t.Errorf("updated Description = %q", agent.Description)
	}
	if agent.Model != "sonnet" {
		t.Errorf("updated Model = %q", agent.Model)
	}
	if len(agent.Tools) != 2 {
		t.Errorf("updated Tools = %v", agent.Tools)
	}
}

// TestAgentsProviderAdapterCommandIntegration tests the agents command via the commands.Dispatcher.
func TestAgentsProviderAdapterCommandIntegration(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a custom agent on disk
	customAgent := `---
name: integration-test-agent
description: Integration test agent
tools: read_file, bash
model: sonnet
---
You are an integration test agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "integration-test-agent.md"), []byte(customAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create dispatcher with the real adapter
	rt := &mockCmdRuntime{workspaceRoot: dir}
	d := commands.NewDispatcher(rt)
	adapter := NewAgentsProviderAdapter(nil)
	d.SetAgentsProvider(adapter)

	// Test /agents list
	result, err := d.Handle(context.Background(), "/agents list")
	if err != nil {
		t.Fatalf("/agents list error: %v", err)
	}
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("/agents list missing Explore: %q", result.Message)
	}
	if !strings.Contains(result.Message, "integration-test-agent") {
		t.Errorf("/agents list missing custom agent: %q", result.Message)
	}

	// Test /agents show Explore
	result, err = d.Handle(context.Background(), "/agents show Explore")
	if err != nil {
		t.Fatalf("/agents show Explore error: %v", err)
	}
	if !strings.Contains(result.Message, "Explore") {
		t.Errorf("/agents show Explore: %q", result.Message)
	}
	if !strings.Contains(result.Message, "builtin") {
		t.Errorf("/agents show should show source: %q", result.Message)
	}
	if !strings.Contains(result.Message, "Tools:") {
		t.Errorf("/agents show should show tools: %q", result.Message)
	}

	// Test /agents show custom agent
	result, err = d.Handle(context.Background(), "/agents show integration-test-agent")
	if err != nil {
		t.Fatalf("/agents show custom error: %v", err)
	}
	if !strings.Contains(result.Message, "integration-test-agent") {
		t.Errorf("/agents show custom: %q", result.Message)
	}
	if !strings.Contains(result.Message, "sonnet") {
		t.Errorf("/agents show custom should show model: %q", result.Message)
	}

	// Test /agents show nonexistent
	result, err = d.Handle(context.Background(), "/agents show nonexistent")
	if err != nil {
		t.Fatalf("/agents show nonexistent error: %v", err)
	}
	if !strings.Contains(result.Message, "not found") {
		t.Errorf("/agents show nonexistent: %q", result.Message)
	}

	// Test /agents create
	result, err = d.Handle(context.Background(), "/agents create new-cli-agent --desc \"Created via CLI\" --tools read_file,bash --model haiku")
	if err != nil {
		t.Fatalf("/agents create error: %v", err)
	}
	if !strings.Contains(result.Message, "created successfully") {
		t.Errorf("/agents create: %q", result.Message)
	}

	// Verify file created
	newAgentFile := filepath.Join(agentsDir, "new-cli-agent.md")
	data, err := os.ReadFile(newAgentFile)
	if err != nil {
		t.Fatalf("new agent file not created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "name: new-cli-agent") {
		t.Errorf("new agent file content: %q", content)
	}
	if !strings.Contains(content, "Created via CLI") {
		t.Errorf("new agent missing description: %q", content)
	}

	// Test /agents delete
	result, err = d.Handle(context.Background(), "/agents delete new-cli-agent")
	if err != nil {
		t.Fatalf("/agents delete error: %v", err)
	}
	if !strings.Contains(result.Message, "deleted") {
		t.Errorf("/agents delete: %q", result.Message)
	}
	if _, err := os.Stat(newAgentFile); !os.IsNotExist(err) {
		t.Error("agent file should be deleted after /agents delete")
	}

	// Test /agents delete built-in should fail
	result, err = d.Handle(context.Background(), "/agents delete Explore")
	if err != nil {
		t.Fatalf("/agents delete builtin error: %v", err)
	}
	if !strings.Contains(result.Message, "cannot be deleted") {
		t.Errorf("/agents delete builtin should fail: %q", result.Message)
	}

	// Test /agents (no args) shows list
	result, err = d.Handle(context.Background(), "/agents")
	if err != nil {
		t.Fatalf("/agents error: %v", err)
	}
	if !strings.Contains(result.Message, "Available Agents") {
		t.Errorf("/agents should show list: %q", result.Message)
	}

	// Test unknown subcommand
	result, err = d.Handle(context.Background(), "/agents xyz")
	if err != nil {
		t.Fatalf("/agents xyz error: %v", err)
	}
	if !strings.Contains(result.Message, "Agent Management") {
		t.Errorf("/agents xyz should show help: %q", result.Message)
	}
}

// mockCmdRuntime implements commands.Runtime for integration tests.
type mockCmdRuntime struct {
	model        string
	permMode     string
	sessionID    string
	msgCount     int
	workspaceRoot string
}

func (m *mockCmdRuntime) GetModel() string                         { return m.model }
func (m *mockCmdRuntime) SetModel(s string)                        { m.model = s }
func (m *mockCmdRuntime) GetPermissionMode() string                { return m.permMode }
func (m *mockCmdRuntime) SetPermissionMode(s string)               { m.permMode = s }
func (m *mockCmdRuntime) IsYoloMode() bool                         { return m.permMode == "yolo" }
func (m *mockCmdRuntime) ToggleYoloMode() bool {
	if m.permMode == "yolo" {
		m.permMode = "workspace_write"
		return false
	}
	m.permMode = "yolo"
	return true
}
func (m *mockCmdRuntime) GetMessageCount() int                     { return m.msgCount }
func (m *mockCmdRuntime) GetSessionID() string                     { return m.sessionID }
func (m *mockCmdRuntime) GetUsageInfo() commands.UsageInfo         { return commands.UsageInfo{} }
func (m *mockCmdRuntime) CompactSession() error                    { return nil }
func (m *mockCmdRuntime) ClearSession()                            { m.msgCount = 0 }
func (m *mockCmdRuntime) GetWorkspaceRoot() string                 { return m.workspaceRoot }
func (m *mockCmdRuntime) GetAllSettings() map[string]interface{}   { return map[string]interface{}{} }
func (m *mockCmdRuntime) SetConfigValue(key, value string) error   { return nil }
func (m *mockCmdRuntime) RunGitCommand(args ...string) (string, error) { return "", nil }
func (m *mockCmdRuntime) RevertLastTurn() (int, error)             { return 0, nil }
func (m *mockCmdRuntime) RevertAll() (int, error)                  { return 0, nil }
func (m *mockCmdRuntime) LoadSession(id string) error              { return nil }
func (m *mockCmdRuntime) NewSession()                              {}
func (m *mockCmdRuntime) GetSubAgentSessions() []commands.SubAgentSessionInfo { return nil }
func (m *mockCmdRuntime) ResumeSubAgentSession(id string) error    { return nil }
