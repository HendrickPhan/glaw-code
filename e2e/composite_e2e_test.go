package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hieu-glaw/glaw-code/internal/mcp"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// --- CompositeToolExecutor E2E Tests ---

func TestE2ECompositeBuiltinToolSucceeds(t *testing.T) {
	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, nil)

	out, err := composite.ExecuteTool(context.Background(), "bash", json.RawMessage(`{"command":"echo hi"}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if out.IsError {
		t.Errorf("IsError = true, Content = %q", out.Content)
	}
	if !strings.Contains(out.Content, "hi") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2ECompositeUnknownToolFallsBackToMCP(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "test",
		Tools: []mcp.MCPTool{
			{Name: "custom_search", Description: "Custom search", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"custom_search": func(args map[string]interface{}) string {
				q, _ := args["query"].(string)
				return "result for: " + q
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test": {Transport: "http", URL: mock.URL()},
	})

	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, mgr)

	out, err := composite.ExecuteTool(ctx, "custom_search", json.RawMessage(`{"query":"test"}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if out.IsError {
		t.Errorf("IsError = true, Content = %q", out.Content)
	}
	if !strings.Contains(out.Content, "result for: test") {
		t.Errorf("Content = %q", out.Content)
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeMCPFallbackWithToolResult(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "test",
		Tools: []mcp.MCPTool{
			{Name: "translate", Description: "Translate text", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"translate": func(args map[string]interface{}) string {
				text, _ := args["text"].(string)
				lang, _ := args["lang"].(string)
				return "translated(" + lang + "): " + text
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test": {Transport: "http", URL: mock.URL()},
	})

	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, mgr)

	out, err := composite.ExecuteTool(ctx, "translate", json.RawMessage(`{"text":"hello","lang":"fr"}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if out.Content != "translated(fr): hello" {
		t.Errorf("Content = %q", out.Content)
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeBuiltinTakesPriority(t *testing.T) {
	// Create MCP server with a "read_file" tool (same name as builtin)
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "conflict",
		Tools: []mcp.MCPTool{
			{Name: "read_file", Description: "MCP read", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"read_file": func(args map[string]interface{}) string { return "FROM MCP" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"conflict": {Transport: "http", URL: mock.URL()},
	})

	dir := t.TempDir()
	builtin := runtime.NewBuiltinToolExecutor(dir)
	composite := runtime.NewCompositeToolExecutor(builtin, mgr)

	// Write a file first via builtin write_file
	writeOut, err := builtin.ExecuteTool(ctx, "write_file", json.RawMessage(`{"path":"test.txt","content":"builtin content"}`))
	if err != nil || writeOut.IsError {
		t.Fatalf("write failed: %v / %s", err, writeOut.Content)
	}

	out, err := composite.ExecuteTool(ctx, "read_file", json.RawMessage(`{"path":"test.txt"}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	// Should use builtin, not MCP
	if out.Content == "FROM MCP" {
		t.Error("should use builtin, not MCP, for read_file")
	}
	if !strings.Contains(out.Content, "builtin content") {
		t.Errorf("Content = %q, should contain 'builtin content'", out.Content)
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeNilMCPManager(t *testing.T) {
	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, nil)

	out, err := composite.ExecuteTool(context.Background(), "unknown_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if !out.IsError {
		t.Error("IsError should be true for unknown tool with nil MCP")
	}
	if !strings.Contains(out.Content, "Unknown tool") {
		t.Errorf("Content = %q", out.Content)
	}
}

func TestE2ECompositeNilBuiltin(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "test",
		Tools: []mcp.MCPTool{
			{Name: "mcp_tool", Description: "MCP tool", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"mcp_tool": func(args map[string]interface{}) string { return "mcp result" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test": {Transport: "http", URL: mock.URL()},
	})

	// nil builtin -> noopToolExecutor -> always "Unknown tool" -> falls back to MCP
	composite := runtime.NewCompositeToolExecutor(nil, mgr)

	out, err := composite.ExecuteTool(ctx, "mcp_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if out.Content != "mcp result" {
		t.Errorf("Content = %q, want 'mcp result'", out.Content)
	}

	// Unknown tool with nil builtin + MCP that doesn't know it either
	out2, err := composite.ExecuteTool(ctx, "no_such_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if !out2.IsError {
		t.Error("IsError should be true for unknown tool")
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeGetToolSpecs(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "test",
		Tools: []mcp.MCPTool{
			{Name: "mcp_tool_a", Description: "MCP tool A", InputSchema: map[string]interface{}{"type": "object"}},
			{Name: "mcp_tool_b", Description: "MCP tool B", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"mcp_tool_a": func(args map[string]interface{}) string { return "a" },
			"mcp_tool_b": func(args map[string]interface{}) string { return "b" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test": {Transport: "http", URL: mock.URL()},
	})

	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, mgr)

	specs := composite.GetToolSpecs()
	// BuiltinToolExecutor has 4 tools: bash, read_file, write_file, edit_file
	builtinCount := 0
	for _, s := range specs {
		switch s.Name {
		case "bash", "read_file", "write_file", "edit_file":
			builtinCount++
		}
	}
	if builtinCount != 4 {
		t.Errorf("found %d builtin specs, want 4", builtinCount)
	}

	// Plus 2 MCP tools
	mcpCount := 0
	for _, s := range specs {
		if s.Name == "mcp_tool_a" || s.Name == "mcp_tool_b" {
			mcpCount++
		}
	}
	if mcpCount != 2 {
		t.Errorf("found %d MCP specs, want 2", mcpCount)
	}

	// Each spec should have valid fields
	for _, s := range specs {
		if s.Name == "" {
			t.Error("spec has empty Name")
		}
		if !json.Valid(s.InputSchema) {
			t.Errorf("spec %q has invalid InputSchema", s.Name)
		}
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeGetToolSpecsWithNilMCP(t *testing.T) {
	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, nil)

	specs := composite.GetToolSpecs()
	if len(specs) != 4 {
		t.Errorf("specs count = %d, want 4 (builtin only)", len(specs))
	}
}

func TestE2ECompositeFullWorkflow(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "analyzer",
		Tools: []mcp.MCPTool{
			{Name: "analyze", Description: "Analyze content", InputSchema: map[string]interface{}{"type": "object"}},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"analyze": func(args map[string]interface{}) string {
				text, _ := args["text"].(string)
				return "Analysis: " + text + " (length: analysis complete)"
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"analyzer": {Transport: "http", URL: mock.URL()},
	})

	dir := t.TempDir()
	builtin := runtime.NewBuiltinToolExecutor(dir)
	composite := runtime.NewCompositeToolExecutor(builtin, mgr)

	// Write a file via builtin write_file
	writeOut, err := composite.ExecuteTool(ctx, "write_file", json.RawMessage(`{"path":"data.txt","content":"important data"}`))
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if writeOut.IsError {
		t.Fatalf("write failed: %s", writeOut.Content)
	}

	// Read it back via builtin read_file
	readOut, err := composite.ExecuteTool(ctx, "read_file", json.RawMessage(`{"path":"data.txt"}`))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if readOut.Content != "important data" {
		t.Errorf("Content = %q", readOut.Content)
	}

	// Analyze via MCP tool
	analyzeOut, err := composite.ExecuteTool(ctx, "analyze", json.RawMessage(`{"text":"important data"}`))
	if err != nil {
		t.Fatalf("analyze error: %v", err)
	}
	if !strings.Contains(analyzeOut.Content, "Analysis: important data") {
		t.Errorf("Content = %q", analyzeOut.Content)
	}

	_ = mgr.Shutdown()
}

func TestE2ECompositeMCPErrorReturnsBuiltinError(t *testing.T) {
	// When both builtin and MCP don't know the tool, the original builtin error is returned
	builtin := runtime.NewBuiltinToolExecutor(t.TempDir())
	composite := runtime.NewCompositeToolExecutor(builtin, nil) // nil MCP, so fallback also fails

	out, err := composite.ExecuteTool(context.Background(), "totally_unknown", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("ExecuteTool error: %v", err)
	}
	if !out.IsError {
		t.Error("IsError should be true")
	}
	if !strings.Contains(out.Content, "Unknown tool") {
		t.Errorf("Content = %q, should contain 'Unknown tool'", out.Content)
	}
}
