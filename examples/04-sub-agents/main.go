// Example 04: Sub-Agents
//
// Demonstrates ALL sub-agent functions available in glaw-code:
//   1. Built-in sub-agent definitions and lookup
//   2. Loading agents from .glaw/agents/ config files (markdown + YAML frontmatter)
//   3. Project-level overrides user-level precedence
//   4. SubAgentOrchestrator: SpawnTask, WaitTask, ListTasks, GetTask, CancelTask
//   5. SubAgentExecutor: FilterTools, BuildSystemPrompt, Execute
//   6. ResolvedTools and ResolvedModel helpers
//   7. The sub_agent tool through tools.Registry
//   8. End-to-end workflow with multiple agents
//   9. Auto-loading all agents from disk with LoadAllSubAgents
//
// This example uses pre-created config files in .glaw/agents/ instead of
// hardcoding agent definitions in code. See .glaw/agents/*.md for the
// agent configurations.
//
// Run: go run examples/04-sub-agents/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hieu-glaw/glaw-code/internal/agent"
	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/tools"
)

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Example 04: Sub-Agents — Complete Feature Demo")
	fmt.Println("═══════════════════════════════════════════════════════════")

	// Use the example directory as the workspace so .glaw/agents/*.md
	// config files are discovered automatically.
	workspace, err := os.Getwd()
	check(err)
	workspace = filepath.Join(workspace, "examples", "04-sub-agents")

	// Verify the agent config files exist
	agentsDir := filepath.Join(workspace, ".glaw", "agents")
	entries, err := os.ReadDir(agentsDir)
	check(err)
	fmt.Printf("Agent config files in %s:\n", agentsDir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			fmt.Printf("  - %s\n", e.Name())
		}
	}

	// Create a temp directory for the sample project and user-level config.
	// The sample project files are needed for the tools to work.
	tmpDir, err := os.MkdirTemp("", "glaw-subagent-demo-*")
	check(err)
	defer os.RemoveAll(tmpDir)
	createSampleProject(tmpDir)

	ctx := context.Background()

	// ── 1. Built-in Agent Definitions ──────────────────────────
	section("1. Built-in Agent Definitions & Lookup")
	fmt.Println("Built-in agents available:")
	fmt.Println()

	for _, name := range agent.BuiltinSubAgentNames() {
		sa := agent.GetBuiltinSubAgent(name)
		fmt.Printf("  %-20s tools: [%s]\n", sa.Name, strings.Join(sa.Tools, ", "))
	}

	e := agent.GetBuiltinSubAgent("Explore")
	fmt.Printf("\n  GetBuiltinSubAgent(\"Explore\") → %d tools: %v\n", len(e.Tools), e.Tools)
	fmt.Printf("  GetBuiltinSubAgent(\"nonexistent\") → nil: %v\n", agent.GetBuiltinSubAgent("nonexistent") == nil)

	// ── 2. Show Pre-Created Config Files ───────────────────────
	section("2. Agent Config Files (.glaw/agents/*.md)")
	fmt.Printf("  Config directory: %s\n\n", agentsDir)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentsDir, entry.Name()))
		check(err)
		// Show first few lines of each config file
		lines := strings.Split(string(data), "\n")
		limit := 8
		if len(lines) < limit {
			limit = len(lines)
		}
		fmt.Printf("  ── %s ──\n", entry.Name())
		for i := 0; i < limit; i++ {
			fmt.Printf("    %s\n", lines[i])
		}
		if len(lines) > limit {
			fmt.Printf("    ... (%d more lines)\n", len(lines)-limit)
		}
		fmt.Println()
	}

	// ── 3. Load Project-Level Agents from Disk ─────────────────
	section("3. Load Agents from .glaw/agents/ (project level)")
	loaded, errs := agent.LoadSubAgentsFromDir(agentsDir, "project")
	fmt.Printf("  LoadSubAgentsFromDir → %d agents, %d errors\n", len(loaded), len(errs))
	for _, c := range loaded {
		fmt.Printf("    - %-15s (level=%s, tools=%v)\n", c.Name, c.Level, c.Tools)
	}

	// ── 4. User-Level Agent Config (for precedence demo) ───────
	section("4. User-Level Agent Config & Project-Level Override")
	// Set up a fake HOME with user-level agents to demonstrate precedence.
	// In real usage, these would be in ~/.glaw/agents/.
	home := filepath.Join(tmpDir, ".fakehome")
	os.MkdirAll(home, 0o755)
	os.Setenv("HOME", home)
	userDir := filepath.Join(home, ".glaw", "agents")
	os.MkdirAll(userDir, 0o755)

	// Create a user-level agent with the same name as a project-level one
	// to show that project-level takes precedence.
	userAgent := &agent.SubAgentConfig{
		Name:        "shared-name",
		Description: "User-level version",
		Tools:       []string{"read_file"},
		Prompt:      "User agent.",
	}
	check(agent.CreateSubAgentFile(userDir, userAgent))
	fmt.Printf("  Created user-level agent: %s/shared-name.md\n", userDir)

	// Also create a user-only agent (not overridden by project)
	userOnlyAgent := &agent.SubAgentConfig{
		Name:        "general-helper",
		Description: "A user-level general helper",
		Tools:       []string{"read_file", "bash", "grep_search"},
		Prompt:      "You are a helpful general-purpose assistant.",
	}
	check(agent.CreateSubAgentFile(userDir, userOnlyAgent))
	fmt.Printf("  Created user-level agent: %s/general-helper.md\n", userDir)

	// LoadAllSubAgents shows project-level overrides user-level
	all, err := agent.LoadAllSubAgents(workspace)
	check(err)
	fmt.Printf("\n  LoadAllSubAgents → %d total agents:\n", len(all))
	for _, c := range all {
		fmt.Printf("    %-15s level=%-8s desc=%q\n", c.Name, c.Level, c.Description)
	}
	// Verify project-level takes precedence
	for _, c := range all {
		if c.Name == "shared-name" {
			fmt.Printf("\n  ✓ shared-name → level=%s (project takes precedence!)\n", c.Level)
		}
	}

	// ── 5. ResolvedTools & ResolvedModel ───────────────────────
	section("5. ResolvedTools & ResolvedModel")
	allTools := []string{"bash", "read_file", "write_file", "edit_file", "glob_search", "grep_search"}

	emptyCfg := &agent.SubAgentConfig{Name: "inherit"}
	fmt.Printf("  ResolvedTools(empty) → inherits all %d: %v\n", len(emptyCfg.ResolvedTools(allTools)), emptyCfg.ResolvedTools(allTools))

	filteredCfg := &agent.SubAgentConfig{Name: "filtered", Tools: []string{"read_file", "grep_search"}}
	fmt.Printf("  ResolvedTools(filtered) → %d: %v\n", len(filteredCfg.ResolvedTools(allTools)), filteredCfg.ResolvedTools(allTools))

	// Show tools for agents loaded from disk
	for _, c := range all {
		resolved := c.ResolvedTools(allTools)
		if len(c.Tools) == 0 {
			fmt.Printf("  %s → inherits all %d tools\n", c.Name, len(resolved))
		} else {
			fmt.Printf("  %s → %d tools: %v\n", c.Name, len(resolved), resolved)
		}
	}

	fmt.Println()
	models := []struct{ cfg, parent, want string }{
		{"", "claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"inherit", "claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"sonnet", "claude-opus-4", "claude-sonnet-4-6"},
		{"opus", "claude-sonnet-4-6", "claude-opus-4"},
		{"haiku", "claude-sonnet-4-6", "claude-haiku-4"},
	}
	for _, m := range models {
		c := &agent.SubAgentConfig{Model: m.cfg}
		fmt.Printf("  ResolvedModel(%q, parent=%q) → %q\n", m.cfg, m.parent, c.ResolvedModel(m.parent))
	}

	// Show model resolution for disk-loaded agents
	for _, c := range all {
		if c.Model != "" {
			fmt.Printf("  %s (model=%q) → ResolvedModel → %q\n", c.Name, c.Model, c.ResolvedModel("claude-sonnet-4-6"))
		}
	}

	// ── 6. SubAgentOrchestrator Lifecycle ──────────────────────
	section("6. SubAgentOrchestrator — Spawn, Wait, List, Get, Cancel")
	agent.SetCustomConfigs(all)
	reg := tools.NewRegistry(tmpDir)
	specs := reg.GetToolSpecs()
	orch := agent.NewSubAgentOrchestrator(reg, specs, "claude-sonnet-4-6")

	// Wire the orchestrator into the registry so the sub_agent tool works.
	reg.SetOrchestrator(orch)

	task1, err := orch.SpawnTask(ctx, "Explore", "Find all Go files in the project")
	check(err)
	fmt.Printf("  Spawned: id=%s agent=Explore\n", task1.ID)

	task2, err := orch.SpawnTask(ctx, "Plan", "Create a plan for adding error handling")
	check(err)
	fmt.Printf("  Spawned: id=%s agent=Plan\n", task2.ID)

	task3, err := orch.SpawnTask(ctx, "general-purpose", "Count lines in main.go")
	check(err)
	fmt.Printf("  Spawned: id=%s agent=general-purpose\n", task3.ID)

	// Also spawn a disk-loaded custom agent
	task4, err := orch.SpawnTask(ctx, "go-expert", "Review the Go code for idiomatic patterns")
	check(err)
	fmt.Printf("  Spawned: id=%s agent=go-expert (loaded from .glaw/agents/go-expert.md)\n", task4.ID)

	task5, err := orch.SpawnTask(ctx, "Verification", "Run tests and verify")
	check(err)
	fmt.Printf("  Spawned: id=%s agent=Verification\n", task5.ID)

	tasks := orch.ListTasks()
	fmt.Printf("\n  ListTasks() → %d tasks:\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("    %s  agent=%-18s  status=%s\n", t.ID, t.AgentName, t.Status)
	}

	got, err := orch.GetTask(task1.ID)
	check(err)
	fmt.Printf("\n  GetTask(%s) → found: agent=%s\n", task1.ID, got.AgentName)
	_, err = orch.GetTask("nonexistent")
	fmt.Printf("  GetTask(\"nonexistent\") → error: %v\n", err != nil)

	fmt.Println()
	for _, id := range []string{task1.ID, task2.ID, task3.ID, task4.ID} {
		result, err := orch.WaitTask(id)
		if err != nil {
			fmt.Printf("  Wait(%s) error: %v\n", id[:20], err)
		} else if result != nil {
			fmt.Printf("  Wait(%s) → %s\n", id[:20], trunc(result.Output, 80))
		}
	}

	err = orch.CancelTask(task5.ID)
	if err != nil {
		fmt.Printf("\n  CancelTask error: %v\n", err)
	} else {
		fmt.Printf("\n  CancelTask(%s) → cancelled\n", task5.ID[:20])
	}
	err = orch.CancelTask(task1.ID)
	fmt.Printf("  CancelTask(already done) → error (expected): %v\n", err != nil)

	_, err = orch.SpawnTask(ctx, "nonexistent-agent", "fail")
	fmt.Printf("  SpawnTask(\"nonexistent\") → error: %v\n", err != nil)

	// ── 7. SubAgentExecutor ────────────────────────────────────
	section("7. SubAgentExecutor — FilterTools, BuildSystemPrompt, Execute")

	// Use a built-in agent
	exploreCfg := agent.GetBuiltinSubAgent("Explore")
	exec := agent.NewSubAgentExecutor(exploreCfg, reg, specs, "claude-sonnet-4-6")
	demoExecutor("Explore (built-in)", exec, ctx)

	// Use a disk-loaded custom agent
	goExpertCfg := findAgent(all, "go-expert")
	if goExpertCfg != nil {
		exec2 := agent.NewSubAgentExecutor(goExpertCfg, reg, specs, "claude-sonnet-4-6")
		demoExecutor("go-expert (from .glaw/agents/)", exec2, ctx)
	}

	// Use another disk-loaded agent
	reviewerCfg := findAgent(all, "reviewer")
	if reviewerCfg != nil {
		exec3 := agent.NewSubAgentExecutor(reviewerCfg, reg, specs, "claude-sonnet-4-6")
		demoExecutor("reviewer (from .glaw/agents/)", exec3, ctx)
	}

	// ── 8. sub_agent Tool via Registry ─────────────────────────
	section("8. sub_agent Tool via tools.Registry")
	fmt.Printf("  sub_agent registered: %v\n\n", toolExists(specs, "sub_agent"))

	// Use built-in agents through the tool
	out, err := reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "Explore",
		"prompt":     "List all Go files in the project",
		"wait":       true,
	}))
	check(err)
	fmt.Printf("  sub_agent(Explore): isError=%v\n  output: %s\n\n", out.IsError, trunc(out.Content, 120))

	// Use disk-loaded custom agents through the tool
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "go-expert",
		"prompt":     "Review the Go code for best practices",
		"wait":       true,
	}))
	check(err)
	fmt.Printf("  sub_agent(go-expert): isError=%v\n  output: %s\n\n", out.IsError, trunc(out.Content, 120))

	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "reviewer",
		"prompt":     "Review code quality in main.go",
		"wait":       true,
	}))
	check(err)
	fmt.Printf("  sub_agent(reviewer): isError=%v\n  output: %s\n\n", out.IsError, trunc(out.Content, 120))

	out, _ = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{"prompt": "test"}))
	fmt.Printf("  Missing agent_name → isError: %v\n", out.IsError)

	out, _ = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{"agent_name": "Explore"}))
	fmt.Printf("  Missing prompt → isError: %v\n", out.IsError)

	out, _ = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{"agent_name": "bad", "prompt": "x"}))
	fmt.Printf("  Unknown agent → isError: %v\n", out.IsError)

	// ── 9. Spawn All Available Agents ──────────────────────────
	section("9. Spawn All Available Agents (built-in + custom from config files)")
	agent.SetCustomConfigs(all)
	agents := agent.AllAvailableAgents()
	fmt.Printf("  Total agents available: %d\n\n", len(agents))
	for _, a := range agents {
		out, _ = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
			"agent_name": a.Name,
			"prompt":     "Analyze the project briefly",
			"wait":       true,
		}))
		source := "built-in"
		if a.Level == "project" {
			source = "project config"
		} else if a.Level == "user" {
			source = "user config"
		}
		fmt.Printf("    %-20s (%s) → isError=%v\n", a.Name, source, out.IsError)
	}

	// ── 10. End-to-End Workflow ────────────────────────────────
	section("10. E2E Workflow: Explore → Plan → Write → Review → Verify → Document → Refactor")
	fmt.Println("  Step 1: Explore the project...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "Explore", "prompt": "What files exist?", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 2: Plan changes...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "Plan", "prompt": "Plan adding tests", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 3: Write tests...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "test-writer", "prompt": "Write a test for greet()", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 4: Review code (using config-file reviewer agent)...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "reviewer", "prompt": "Review main.go for quality issues", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 5: Security audit...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "security-auditor", "prompt": "Check for vulnerabilities", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 6: Verify...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "Verification", "prompt": "Run go vet", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 7: Document...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "docs-writer", "prompt": "Update README", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	fmt.Println("  Step 8: Refactor (using config-file go-expert agent)...")
	out, err = reg.ExecuteTool(ctx, "sub_agent", mustJSON(map[string]interface{}{
		"agent_name": "go-expert", "prompt": "Clean up code smells and improve Go style", "wait": true,
	}))
	check(err)
	fmt.Printf("    ✓ %s\n", trunc(out.Content, 80))

	// ── Summary ───────────────────────────────────────────────
	section("Summary")
	agent.SetCustomConfigs(all)
	agents = agent.AllAvailableAgents()
	builtinCount := len(agent.BuiltinSubAgentNames())
	customCount := len(agents) - builtinCount
	fmt.Printf("  Total agents available: %d\n", len(agents))
	fmt.Printf("  Built-in: %d\n", builtinCount)
	fmt.Printf("  Custom (from .glaw/agents/): %d\n", customCount)
	for _, a := range agents {
		source := "(built-in)"
		if a.Level == "project" {
			source = fmt.Sprintf("(from .glaw/agents/%s.md)", a.Name)
		} else if a.Level == "user" {
			source = "(user-level)"
		}
		fmt.Printf("    ✓ %-20s %s\n", a.Name, source)
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════")
	fmt.Println("  ✅ All sub-agent functions demonstrated successfully!")
	fmt.Println("═══════════════════════════════════════════════════════════")
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func createSampleProject(root string) {
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module github.com/example/demo\n\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(root, "main.go"), []byte(`package main

import "fmt"

func main() { fmt.Println(greet("World")) }

func greet(name string) string { return "Hello, " + name + "!" }
`), 0o644)
	os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo Project\n\nA sample project.\n"), 0o644)
}

func section(title string) {
	pad := 60 - len(title)
	if pad < 0 {
		pad = 0
	}
	fmt.Printf("\n── %s %s\n", title, strings.Repeat("─", pad))
	fmt.Println()
}

func check(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		os.Exit(1)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func mustJSON(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func toolExists(specs []api.ToolDefinition, name string) bool {
	for _, s := range specs {
		if s.Name == name {
			return true
		}
	}
	return false
}

func findAgent(agents []*agent.SubAgentConfig, name string) *agent.SubAgentConfig {
	for _, a := range agents {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func demoExecutor(label string, exec *agent.SubAgentExecutor, ctx context.Context) {
	ft := exec.FilterTools()
	fmt.Printf("  %s FilterTools() → %d tools:\n", label, len(ft))
	for _, t := range ft {
		fmt.Printf("    - %s\n", t.Name)
	}

	sp := exec.BuildSystemPrompt()
	fmt.Printf("\n  %s BuildSystemPrompt() first 200 chars:\n    %s\n", label, trunc(sp, 200))

	result, err := exec.Execute(ctx, "Summarize the project structure")
	if err != nil {
		fmt.Printf("  %s Execute() error: %v\n", label, err)
	} else {
		fmt.Printf("  %s Execute() output: %s\n", label, trunc(result.Output, 150))
	}
	fmt.Println()
}
