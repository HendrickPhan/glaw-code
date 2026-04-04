package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hieu-glaw/glaw-code/internal/mcp"
)

// --- Mock MCP Server Infrastructure ---

type mockMCPConfig struct {
	ServerName    string
	ServerVersion string
	Tools         []mcp.MCPTool
	ToolHandlers  map[string]func(args map[string]interface{}) string
	FailInit      bool
	FailToolsList bool
}

type mockMCPServer struct {
	server *httptest.Server
	config mockMCPConfig
	mu     sync.Mutex
	calls  []string
}

func newMockMCPServer(t *testing.T, cfg mockMCPConfig) *mockMCPServer {
	t.Helper()
	ms := &mockMCPServer{config: cfg}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req mcp.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		ms.mu.Lock()
		ms.calls = append(ms.calls, req.Method)
		ms.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		var resp mcp.JSONRPCResponse

		switch req.Method {
		case "initialize":
			if cfg.FailInit {
				resp = mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      idFromReq(req.ID),
					Error:   &mcp.JSONRPCError{Code: -32000, Message: "init failed"},
				}
				break
			}
			result := mcp.MCPInitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities:    mcp.MCPServerCapabilities{Tools: &struct{}{}},
				ServerInfo:      mcp.MCPServerInfo{Name: cfg.ServerName, Version: cfg.ServerVersion},
			}
			resultBytes, _ := json.Marshal(result)
			resp = mcp.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      idFromReq(req.ID),
				Result:  resultBytes,
			}

		case "notifications/initialized":
			w.WriteHeader(http.StatusNoContent)
			return

		case "tools/list":
			if cfg.FailToolsList {
				resp = mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      idFromReq(req.ID),
					Error:   &mcp.JSONRPCError{Code: -32000, Message: "tools/list failed"},
				}
				break
			}
			toolsResult := mcp.MCPListToolsResult{Tools: cfg.Tools}
			resultBytes, _ := json.Marshal(toolsResult)
			resp = mcp.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      idFromReq(req.ID),
				Result:  resultBytes,
			}

		case "tools/call":
			var params mcp.MCPToolCallParams
			if req.Params != nil {
				_ = json.Unmarshal(req.Params, &params)
			}
			handler, ok := cfg.ToolHandlers[params.Name]
			if !ok {
				resultBytes, _ := json.Marshal(mcp.MCPToolCallResult{
					Content: []mcp.MCPContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", params.Name)}},
					IsError: true,
				})
				resp = mcp.JSONRPCResponse{JSONRPC: "2.0", ID: idFromReq(req.ID), Result: resultBytes}
				break
			}
			resultText := handler(params.Arguments)
			resultBytes, _ := json.Marshal(mcp.MCPToolCallResult{
				Content: []mcp.MCPContent{{Type: "text", Text: resultText}},
				IsError: strings.HasPrefix(resultText, "ERROR: "),
			})
			resp = mcp.JSONRPCResponse{JSONRPC: "2.0", ID: idFromReq(req.ID), Result: resultBytes}

		default:
			resp = mcp.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      idFromReq(req.ID),
				Error:   &mcp.JSONRPCError{Code: -32601, Message: "method not found"},
			}
		}

		_ = json.NewEncoder(w).Encode(resp)
	})

	ms.server = httptest.NewServer(handler)
	t.Cleanup(ms.server.Close)
	return ms
}

func (ms *mockMCPServer) URL() string   { return ms.server.URL }
func (ms *mockMCPServer) Close()        { ms.server.Close() }
func (ms *mockMCPServer) Calls() []string {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	cp := make([]string, len(ms.calls))
	copy(cp, ms.calls)
	return cp
}

func idFromReq(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// --- MCP E2E Tests ---

func TestE2EMCPManagerNewEmpty(t *testing.T) {
	mgr := mcp.NewManager()
	if mgr.ToolCount() != 0 {
		t.Errorf("ToolCount = %d, want 0", mgr.ToolCount())
	}
	if mgr.HasTool("anything") {
		t.Error("HasTool should be false on empty manager")
	}
	if tools := mgr.GetTools(); len(tools) != 0 {
		t.Errorf("GetTools returned %d, want 0", len(tools))
	}
	if names := mgr.ServerNames(); len(names) != 0 {
		t.Errorf("ServerNames returned %d, want 0", len(names))
	}
}

func TestE2EMCPInitializeAndDiscoverTools(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName:    "test-server",
		ServerVersion: "1.0.0",
		Tools: []mcp.MCPTool{
			{
				Name:        "get_weather",
				Description: "Get weather for a location",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]string{"type": "string"},
					},
				},
			},
		},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"get_weather": func(args map[string]interface{}) string {
				loc, _ := args["location"].(string)
				return "72F, sunny in " + loc
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	err := mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test-server": {Transport: "http", URL: mock.URL()},
	})
	if err != nil {
		t.Fatalf("InitializeAll error: %v", err)
	}

	if !mgr.HasTool("get_weather") {
		t.Error("HasTool(get_weather) should be true")
	}
	if mgr.ToolCount() != 1 {
		t.Errorf("ToolCount = %d, want 1", mgr.ToolCount())
	}

	tools := mgr.GetTools()
	if len(tools) != 1 {
		t.Fatalf("GetTools count = %d, want 1", len(tools))
	}
	if tools[0].Name != "get_weather" {
		t.Errorf("tool.Name = %q", tools[0].Name)
	}
	if tools[0].ServerName != "test-server" {
		t.Errorf("tool.ServerName = %q", tools[0].ServerName)
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPInitializeMultipleServers(t *testing.T) {
	mock1 := newMockMCPServer(t, mockMCPConfig{
		ServerName: "weather-srv",
		Tools:      []mcp.MCPTool{{Name: "get_weather", Description: "Get weather", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"get_weather": func(args map[string]interface{}) string { return "72F" },
		},
	})
	mock2 := newMockMCPServer(t, mockMCPConfig{
		ServerName: "calc-srv",
		Tools:      []mcp.MCPTool{{Name: "calculate", Description: "Calculate", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"calculate": func(args map[string]interface{}) string { return "42" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"weather-srv": {Transport: "http", URL: mock1.URL()},
		"calc-srv":    {Transport: "http", URL: mock2.URL()},
	})

	if mgr.ToolCount() != 2 {
		t.Errorf("ToolCount = %d, want 2", mgr.ToolCount())
	}
	if !mgr.HasTool("get_weather") || !mgr.HasTool("calculate") {
		t.Error("both tools should be discovered")
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPToolCallSuccess(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "weather",
		Tools:      []mcp.MCPTool{{Name: "get_weather", Description: "Get weather", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"get_weather": func(args map[string]interface{}) string {
				loc, _ := args["location"].(string)
				return "72F, sunny in " + loc
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"weather": {Transport: "http", URL: mock.URL()},
	})

	result, err := mgr.CallTool(ctx, "get_weather", map[string]interface{}{"location": "SF"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	if result.Content[0].Text != "72F, sunny in SF" {
		t.Errorf("Text = %q", result.Content[0].Text)
	}
	if result.IsError {
		t.Error("IsError should be false")
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPToolCallError(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "test",
		Tools:      []mcp.MCPTool{{Name: "fail_tool", Description: "Fails", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"fail_tool": func(args map[string]interface{}) string { return "ERROR: something broke" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"test": {Transport: "http", URL: mock.URL()},
	})

	result, err := mgr.CallTool(ctx, "fail_tool", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if !result.IsError {
		t.Error("IsError should be true")
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPToolCallUnknownTool(t *testing.T) {
	mgr := mcp.NewManager()
	_, err := mgr.CallTool(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown MCP tool") {
		t.Errorf("error = %q, should mention 'unknown MCP tool'", err.Error())
	}
}

func TestE2EMCPProtocolHandshake(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "handshake-test",
		Tools:      []mcp.MCPTool{{Name: "ping", Description: "Ping", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"ping": func(args map[string]interface{}) string { return "pong" },
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"handshake-test": {Transport: "http", URL: mock.URL()},
	})

	calls := mock.Calls()
	if len(calls) < 2 {
		t.Fatalf("expected >= 2 calls, got %d: %v", len(calls), calls)
	}
	if calls[0] != "initialize" {
		t.Errorf("first call = %q, want 'initialize'", calls[0])
	}

	foundToolsList := false
	for _, c := range calls {
		if c == "tools/list" {
			foundToolsList = true
		}
	}
	if !foundToolsList {
		t.Errorf("tools/list not called; calls = %v", calls)
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPShutdown(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName:    "shutdown-test",
		Tools:         []mcp.MCPTool{{Name: "test", Description: "test", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers:  map[string]func(args map[string]interface{}) string{"test": func(args map[string]interface{}) string { return "ok" }},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"shutdown-test": {Transport: "http", URL: mock.URL()},
	})

	if err := mgr.Shutdown(); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestE2EMCPInitializeFailure(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "fail-init",
		FailInit:   true,
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"fail-init": {Transport: "http", URL: mock.URL()},
	})

	if mgr.ToolCount() != 0 {
		t.Errorf("ToolCount = %d after init failure, want 0", mgr.ToolCount())
	}
}

func TestE2EMCPToolsListFailure(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName:    "fail-tools",
		FailToolsList: true,
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"fail-tools": {Transport: "http", URL: mock.URL()},
	})

	if mgr.ToolCount() != 0 {
		t.Errorf("ToolCount = %d after tools/list failure, want 0", mgr.ToolCount())
	}
}

func TestE2EMCPInitializeAllWithHTTPTransport(t *testing.T) {
	mock := newMockMCPServer(t, mockMCPConfig{
		ServerName: "http-test",
		Tools:      []mcp.MCPTool{{Name: "echo", Description: "Echo", InputSchema: map[string]interface{}{"type": "object"}}},
		ToolHandlers: map[string]func(args map[string]interface{}) string{
			"echo": func(args map[string]interface{}) string {
				msg, _ := args["message"].(string)
				return msg
			},
		},
	})

	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"http-server": {Transport: "http", URL: mock.URL()},
	})

	if !mgr.HasTool("echo") {
		t.Error("echo tool should be discovered")
	}

	result, err := mgr.CallTool(ctx, "echo", map[string]interface{}{"message": "hello"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("Text = %q, want 'hello'", result.Content[0].Text)
	}

	_ = mgr.Shutdown()
}

func TestE2EMCPInitializeAllUnsupportedTransport(t *testing.T) {
	ctx := context.Background()
	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"ws-server": {Transport: "websocket", URL: "ws://localhost:9999"},
	})
	if mgr.ToolCount() != 0 {
		t.Errorf("ToolCount = %d for unsupported transport, want 0", mgr.ToolCount())
	}
}

func TestE2EMCPToolCallTimeout(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	mgr := mcp.NewManager()
	_ = mgr.InitializeAll(ctx, map[string]mcp.ServerConfig{
		"slow": {Transport: "http", URL: slowServer.URL},
	})

	if mgr.ToolCount() != 0 {
		t.Errorf("ToolCount = %d after timeout, want 0", mgr.ToolCount())
	}
}
