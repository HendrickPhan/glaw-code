package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequestMarshal(t *testing.T) {
	id := int64(1)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", parsed["jsonrpc"])
	}
	if parsed["method"] != "initialize" {
		t.Errorf("method = %v, want initialize", parsed["method"])
	}
}

func TestJSONRPCResponseParsing(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"test-server","version":"1.0"}}}`

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("ID = %d, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("Error should be nil, got %v", resp.Error)
	}

	var initResult MCPInitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		t.Fatalf("Unmarshal result error: %v", err)
	}
	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion = %q", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q", initResult.ServerInfo.Name)
	}
}

func TestJSONRPCErrorParsing(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":2,"error":{"code":-32600,"message":"Invalid Request"}}`

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("Code = %d, want -32600", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid Request" {
		t.Errorf("Message = %q", resp.Error.Message)
	}
}

func TestMCPToolCallParams(t *testing.T) {
	params := MCPToolCallParams{
		Name: "get_weather",
		Arguments: map[string]interface{}{
			"location": "San Francisco",
			"unit":     "celsius",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed MCPToolCallParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.Name != "get_weather" {
		t.Errorf("Name = %q", parsed.Name)
	}
}

func TestMCPToolCallResult(t *testing.T) {
	result := MCPToolCallResult{
		Content: []MCPContent{
			{Type: "text", Text: "Weather: 72°F, sunny"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed MCPToolCallResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(parsed.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(parsed.Content))
	}
	if parsed.Content[0].Text != "Weather: 72°F, sunny" {
		t.Errorf("Text = %q", parsed.Content[0].Text)
	}
}

func TestManagerNew(t *testing.T) {
	m := NewManager()
	if m.ToolCount() != 0 {
		t.Errorf("ToolCount = %d, want 0", m.ToolCount())
	}
	if m.HasTool("nonexistent") {
		t.Error("HasTool should be false for nonexistent tool")
	}
	if tools := m.GetTools(); len(tools) != 0 {
		t.Errorf("GetTools length = %d, want 0", len(tools))
	}
}

func TestManagerShutdown(t *testing.T) {
	m := NewManager()
	if err := m.Shutdown(); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}
}

func TestEnvToSlice(t *testing.T) {
	env := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}
	result := envToSlice(env)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	// Order not guaranteed, just check both exist
	found := map[string]bool{}
	for _, v := range result {
		found[v] = true
	}
	if !found["KEY1=value1"] || !found["KEY2=value2"] {
		t.Errorf("missing expected env entries: %v", result)
	}
}
