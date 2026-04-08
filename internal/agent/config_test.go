package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Config Parsing Tests ---

func TestParseSubAgentConfigBasic(t *testing.T) {
	data := []byte(`---
name: test-agent
description: A test agent for unit tests
tools: Read, Write, Bash
model: sonnet
---
You are a test agent. Do test things.
`)

	config, err := ParseSubAgentConfig(data, "test-agent.md")
	if err != nil {
		t.Fatalf("ParseSubAgentConfig error: %v", err)
	}

	if config.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", config.Name, "test-agent")
	}
	if config.Description != "A test agent for unit tests" {
		t.Errorf("Description = %q", config.Description)
	}
	if config.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", config.Model, "sonnet")
	}
	if config.Prompt != "You are a test agent. Do test things." {
		t.Errorf("Prompt = %q", config.Prompt)
	}

	// Check tools are converted to glaw names
	wantTools := []string{"read_file", "write_file", "bash"}
	if len(config.Tools) != len(wantTools) {
		t.Fatalf("Tools = %v, want %v", config.Tools, wantTools)
	}
	for i, want := range wantTools {
		if config.Tools[i] != want {
			t.Errorf("Tools[%d] = %q, want %q", i, config.Tools[i], want)
		}
	}
}

func TestParseSubAgentConfigNoTools(t *testing.T) {
	data := []byte(`---
name: minimal-agent
description: Minimal agent with no tools specified
---
Just do stuff.
`)

	config, err := ParseSubAgentConfig(data, "minimal-agent.md")
	if err != nil {
		t.Fatalf("ParseSubAgentConfig error: %v", err)
	}

	if len(config.Tools) != 0 {
		t.Errorf("Tools should be empty, got %v", config.Tools)
	}
}

func TestParseSubAgentConfigInheritName(t *testing.T) {
	data := []byte(`---
description: Agent without explicit name
---
Some prompt.
`)

	config, err := ParseSubAgentConfig(data, "my-custom-agent.md")
	if err != nil {
		t.Fatalf("ParseSubAgentConfig error: %v", err)
	}

	if config.Name != "my-custom-agent" {
		t.Errorf("Name = %q, want %q (inferred from filename)", config.Name, "my-custom-agent")
	}
}

func TestParseSubAgentConfigNoFrontmatter(t *testing.T) {
	data := []byte(`This is just a prompt with no frontmatter at all.`)

	config, err := ParseSubAgentConfig(data, "simple.md")
	if err != nil {
		t.Fatalf("ParseSubAgentConfig error: %v", err)
	}

	if config.Name != "simple" {
		t.Errorf("Name = %q, want %q", config.Name, "simple")
	}
	if config.Prompt != "This is just a prompt with no frontmatter at all." {
		t.Errorf("Prompt = %q", config.Prompt)
	}
}

func TestParseSubAgentConfigClaudeToolNames(t *testing.T) {
	data := []byte(`---
name: claude-tools
description: Test Claude Code tool name mapping
tools: Read, Write, Edit, View, Bash, Grep, Glob, WebFetch, WebSearch, TodoWrite
---
Test prompt.
`)

	config, err := ParseSubAgentConfig(data, "claude-tools.md")
	if err != nil {
		t.Fatalf("ParseSubAgentConfig error: %v", err)
	}

	expectedTools := map[string]bool{
		"read_file":   false, // Read and View both map to read_file
		"write_file":  false,
		"edit_file":   false,
		"bash":        false,
		"grep_search": false,
		"glob_search": false,
		"web_fetch":   false,
		"web_search":  false,
		"todo_write":  false,
	}

	for _, tool := range config.Tools {
		if _, ok := expectedTools[tool]; ok {
			expectedTools[tool] = true
		}
	}

	for tool, found := range expectedTools {
		if !found {
			t.Errorf("Expected tool %q not found in parsed tools %v", tool, config.Tools)
		}
	}
}

// --- ResolvedTools Tests ---

func TestResolvedToolsInherit(t *testing.T) {
	config := &SubAgentConfig{
		Name:        "inherit-test",
		Description: "Test",
		Tools:       nil, // empty = inherit all
	}

	allTools := []string{"bash", "read_file", "write_file", "edit_file"}
	resolved := config.ResolvedTools(allTools)

	if len(resolved) != len(allTools) {
		t.Errorf("ResolvedTools() = %v, want %v (inherit all)", resolved, allTools)
	}
}

func TestResolvedToolsFiltered(t *testing.T) {
	config := &SubAgentConfig{
		Name:        "filtered-test",
		Description: "Test",
		Tools:       []string{"read_file", "grep_search"},
	}

	allTools := []string{"bash", "read_file", "write_file", "edit_file", "grep_search"}
	resolved := config.ResolvedTools(allTools)

	if len(resolved) != 2 {
		t.Fatalf("ResolvedTools() = %v, want 2 tools", resolved)
	}
	if resolved[0] != "read_file" || resolved[1] != "grep_search" {
		t.Errorf("ResolvedTools() = %v, want [read_file, grep_search]", resolved)
	}
}

// --- ResolvedModel Tests ---

func TestResolvedModel(t *testing.T) {
	tests := []struct {
		configModel  string
		parentModel  string
		want         string
	}{
		{"", "claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"inherit", "claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"sonnet", "claude-opus-4", "claude-sonnet-4-6"},
		{"opus", "claude-sonnet-4-6", "claude-opus-4"},
		{"haiku", "claude-sonnet-4-6", "claude-haiku-4"},
		{"custom-model", "claude-sonnet-4-6", "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.configModel, func(t *testing.T) {
			config := &SubAgentConfig{Model: tt.configModel}
			got := config.ResolvedModel(tt.parentModel)
			if got != tt.want {
				t.Errorf("ResolvedModel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Directory Loading Tests ---

func TestLoadSubAgentsFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create agent files
	agent1 := `---
name: agent-one
description: First test agent
tools: Read, Bash
model: sonnet
---
Agent one prompt.
`
	agent2 := `---
name: agent-two
description: Second test agent
tools: Read
---
Agent two prompt.
`

	if err := os.WriteFile(filepath.Join(dir, "agent-one.md"), []byte(agent1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-two.md"), []byte(agent2), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-.md file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not an agent"), 0o644); err != nil {
		t.Fatal(err)
	}

	configs, errs := LoadSubAgentsFromDir(dir, "project")
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}
	if len(configs) != 2 {
		t.Fatalf("Expected 2 configs, got %d", len(configs))
	}

	// Should be sorted by name
	if configs[0].Name != "agent-one" {
		t.Errorf("configs[0].Name = %q", configs[0].Name)
	}
	if configs[1].Name != "agent-two" {
		t.Errorf("configs[1].Name = %q", configs[1].Name)
	}

	// Check level
	for _, c := range configs {
		if c.Level != "project" {
			t.Errorf("Level = %q, want %q", c.Level, "project")
		}
	}
}

func TestLoadSubAgentsFromDirEmpty(t *testing.T) {
	dir := t.TempDir()
	configs, errs := LoadSubAgentsFromDir(dir, "user")
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}
	if len(configs) != 0 {
		t.Fatalf("Expected 0 configs, got %d", len(configs))
	}
}

func TestLoadSubAgentsFromDirNotExist(t *testing.T) {
	configs, errs := LoadSubAgentsFromDir("/nonexistent/path", "user")
	if len(errs) > 0 {
		t.Fatalf("Unexpected errors: %v", errs)
	}
	if len(configs) != 0 {
		t.Fatalf("Expected 0 configs for nonexistent dir, got %d", len(configs))
	}
}

// --- LoadAllSubAgents Tests ---

func TestLoadAllSubAgents(t *testing.T) {
	workspaceRoot := t.TempDir()

	// Create project-level agents
	agentsDir := filepath.Join(workspaceRoot, ".glaw", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	projectAgent := `---
name: project-agent
description: Project-level agent
---
Project agent prompt.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "project-agent.md"), []byte(projectAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	configs, err := LoadAllSubAgents(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadAllSubAgents error: %v", err)
	}

	found := false
	for _, c := range configs {
		if c.Name == "project-agent" {
			found = true
			if c.Level != "project" {
				t.Errorf("Level = %q, want %q", c.Level, "project")
			}
			break
		}
	}
	if !found {
		t.Error("project-agent not found in loaded configs")
	}
}

func TestLoadAllSubAgentsProjectOverridesUser(t *testing.T) {
	workspaceRoot := t.TempDir()

	// Create user-level agent
	home := t.TempDir()
	t.Setenv("HOME", home)
	userAgentsDir := filepath.Join(home, ".glaw", "agents")
	if err := os.MkdirAll(userAgentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	userAgent := `---
name: shared-agent
description: User-level version
---
User agent prompt.
`
	if err := os.WriteFile(filepath.Join(userAgentsDir, "shared-agent.md"), []byte(userAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create project-level agent with same name (should override)
	projAgentsDir := filepath.Join(workspaceRoot, ".glaw", "agents")
	if err := os.MkdirAll(projAgentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projAgent := `---
name: shared-agent
description: Project-level version
---
Project agent prompt.
`
	if err := os.WriteFile(filepath.Join(projAgentsDir, "shared-agent.md"), []byte(projAgent), 0o644); err != nil {
		t.Fatal(err)
	}

	configs, err := LoadAllSubAgents(workspaceRoot)
	if err != nil {
		t.Fatalf("LoadAllSubAgents error: %v", err)
	}

	// Should only have one shared-agent (project takes precedence)
	count := 0
	for _, c := range configs {
		if c.Name == "shared-agent" {
			count++
			if c.Level != "project" {
				t.Errorf("Level = %q, want %q (project should override)", c.Level, "project")
			}
			if c.Description != "Project-level version" {
				t.Errorf("Description = %q, should be project-level", c.Description)
			}
		}
	}
	if count != 1 {
		t.Errorf("Found %d shared-agent configs, want 1", count)
	}
}

// --- CreateSubAgentFile Tests ---

func TestCreateSubAgentFile(t *testing.T) {
	dir := t.TempDir()

	config := &SubAgentConfig{
		Name:        "my-agent",
		Description: "My custom agent",
		Tools:       []string{"read_file", "bash"},
		Model:       "sonnet",
		Prompt:      "You are my custom agent.",
	}

	if err := CreateSubAgentFile(dir, config); err != nil {
		t.Fatalf("CreateSubAgentFile error: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "my-agent.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	content := string(data)
	if !containsString(content, "name: my-agent") {
		t.Error("File should contain 'name: my-agent'")
	}
	if !containsString(content, "description: My custom agent") {
		t.Error("File should contain description")
	}
	if !containsString(content, "tools: read_file, bash") {
		t.Error("File should contain tools")
	}
	if !containsString(content, "model: sonnet") {
		t.Error("File should contain model")
	}
	if !containsString(content, "You are my custom agent.") {
		t.Error("File should contain prompt")
	}
}

func TestEnsureAgentsDir(t *testing.T) {
	workspaceRoot := t.TempDir()

	dir, err := EnsureAgentsDir(workspaceRoot)
	if err != nil {
		t.Fatalf("EnsureAgentsDir error: %v", err)
	}

	expectedDir := filepath.Join(workspaceRoot, ".glaw", "agents")
	if dir != expectedDir {
		t.Errorf("dir = %q, want %q", dir, expectedDir)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}

	// Call again - should not error
	dir2, err := EnsureAgentsDir(workspaceRoot)
	if err != nil {
		t.Fatalf("Second EnsureAgentsDir error: %v", err)
	}
	if dir2 != dir {
		t.Errorf("dir2 = %q, want %q", dir2, dir)
	}
}

// --- Builtin Agents Tests ---

func TestBuiltinSubAgentsExist(t *testing.T) {
	expectedAgents := []string{
		"Explore", "Plan", "Verification",
		"code-reviewer", "security-auditor",
		"test-writer", "docs-writer",
		"refactorer", "general-purpose",
	}

	names := BuiltinSubAgentNames()
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range expectedAgents {
		if !nameSet[expected] {
			t.Errorf("Missing built-in agent: %q", expected)
		}
	}
}

func TestGetBuiltinSubAgent(t *testing.T) {
	config := GetBuiltinSubAgent("Explore")
	if config == nil {
		t.Fatal("GetBuiltinSubAgent(Explore) returned nil")
		return
	}
	if config.Name != "Explore" {
		t.Errorf("Name = %q", config.Name)
	}
	if len(config.Tools) == 0 {
		t.Error("Explore agent should have tools")
	}
	if config.Prompt == "" {
		t.Error("Explore agent should have a prompt")
	}
}

func TestGetBuiltinSubAgentNotFound(t *testing.T) {
	config := GetBuiltinSubAgent("nonexistent")
	if config != nil {
		t.Error("GetBuiltinSubAgent should return nil for nonexistent agent")
	}
}

func TestBuiltinAgentToolPermissions(t *testing.T) {
	tests := []struct {
		name          string
		hasBash       bool
		hasRead       bool
		hasWrite      bool
		hasWebFetch   bool
	}{
		{"Explore", false, true, false, true},
		{"Plan", false, true, false, true},
		{"Verification", true, true, false, true},
		{"code-reviewer", false, true, false, true},
		{"security-auditor", false, true, false, true},
		{"test-writer", true, true, true, false},
		{"docs-writer", true, true, true, true},
		{"refactorer", true, true, true, false},
		{"general-purpose", true, true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetBuiltinSubAgent(tt.name)
			if config == nil {
				t.Fatalf("Agent %q not found", tt.name)
				return
			}

			has := func(name string) bool {
				for _, t := range config.Tools {
					if t == name {
						return true
					}
				}
				return false
			}

			if got := has("bash"); got != tt.hasBash {
				t.Errorf("has bash = %v, want %v", got, tt.hasBash)
			}
			if got := has("read_file"); got != tt.hasRead {
				t.Errorf("has read_file = %v, want %v", got, tt.hasRead)
			}
			if got := has("write_file"); got != tt.hasWrite {
				t.Errorf("has write_file = %v, want %v", got, tt.hasWrite)
			}
			if got := has("web_fetch"); got != tt.hasWebFetch {
				t.Errorf("has web_fetch = %v, want %v", got, tt.hasWebFetch)
			}
		})
	}
}

// --- AllAvailableAgents Tests ---

func TestAllAvailableAgents(t *testing.T) {
	// Clear any custom configs
	SetCustomConfigs(nil)

	agents := AllAvailableAgents()
	if len(agents) < 9 {
		t.Errorf("AllAvailableAgents returned %d agents, want at least 9", len(agents))
	}
}

// Helper

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
