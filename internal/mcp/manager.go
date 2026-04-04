package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ServerConfig defines how to connect to an MCP server.
type ServerConfig struct {
	Transport string            `json:"transport"` // "stdio" | "sse" | "http" | "websocket"
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Env       map[string]string `json:"env"`
}

// Transport is the interface for sending JSON-RPC messages to an MCP server.
type Transport interface {
	SendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error)
	SendNotification(method string, params interface{}) error
	Close() error
}

// ManagedTool represents a tool discovered from an MCP server.
type ManagedTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	ServerName  string
}

// Manager manages multiple MCP server connections.
type Manager struct {
	mu      sync.RWMutex
	servers map[string]Transport
	tools   map[string]*ManagedTool // tool_name -> tool
	routes  map[string]string       // tool_name -> server_name
	configs map[string]ServerConfig
}

// NewManager creates a new MCP server manager.
func NewManager() *Manager {
	return &Manager{
		servers: make(map[string]Transport),
		tools:   make(map[string]*ManagedTool),
		routes:  make(map[string]string),
		configs: make(map[string]ServerConfig),
	}
}

// InitializeAll connects to all configured MCP servers and discovers tools.
func (m *Manager) InitializeAll(ctx context.Context, configs map[string]ServerConfig) error {
	m.mu.Lock()
	m.configs = configs
	m.mu.Unlock()

	for name, config := range configs {
		switch config.Transport {
		case "stdio":
			if err := m.connectStdio(ctx, name, config); err != nil {
				fmt.Printf("MCP: failed to connect to server %q: %v\n", name, err)
			}
		case "http", "sse":
			if err := m.connectHTTP(ctx, name, config); err != nil {
				fmt.Printf("MCP: failed to connect to server %q: %v\n", name, err)
			}
		default:
			fmt.Printf("MCP: unsupported transport %q for server %q\n", config.Transport, name)
		}
	}

	return nil
}

// connectStdio connects to a single MCP server via stdio and discovers its tools.
func (m *Manager) connectStdio(ctx context.Context, name string, config ServerConfig) error {
	proc, err := NewStdioProcess(ctx, config.Command, config.Args, config.Env)
	if err != nil {
		return fmt.Errorf("starting MCP server %s: %w", name, err)
	}
	defer func() {
		// If registration fails, close the process.
		// On success the transport is stored in m.servers and not closed here.
		m.mu.RLock()
		_, stored := m.servers[name]
		m.mu.RUnlock()
		if !stored {
			proc.Close()
		}
	}()

	return m.initializeAndRegister(ctx, name, proc)
}

// connectHTTP connects to a single MCP server via HTTP/SSE and discovers its tools.
func (m *Manager) connectHTTP(ctx context.Context, name string, config ServerConfig) error {
	client, err := NewSSEClient(config.URL, config.Headers)
	if err != nil {
		return fmt.Errorf("creating SSE client for %s: %w", name, err)
	}
	defer func() {
		m.mu.RLock()
		_, stored := m.servers[name]
		m.mu.RUnlock()
		if !stored {
			client.Close()
		}
	}()

	return m.initializeAndRegister(ctx, name, client)
}

// initializeAndRegister performs the MCP handshake and tool discovery on a transport.
func (m *Manager) initializeAndRegister(ctx context.Context, name string, t Transport) error {
	// Send initialize request
	initParams := MCPInitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    MCPCapabilities{},
		ClientInfo:      MCPClientInfo{Name: "glaw-code", Version: "1.0.0"},
	}

	resp, err := t.SendRequest(ctx, "initialize", initParams)
	if err != nil {
		return fmt.Errorf("initializing MCP server %s: %w", name, err)
	}

	// Parse initialize result
	var initResult MCPInitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		return fmt.Errorf("parsing initialize result from %s: %w", name, err)
	}

	// Send initialized notification
	if err := t.SendNotification("notifications/initialized", nil); err != nil {
		return fmt.Errorf("sending initialized notification to %s: %w", name, err)
	}

	// Discover tools
	toolsResp, err := t.SendRequest(ctx, "tools/list", struct{}{})
	if err != nil {
		return fmt.Errorf("listing tools from %s: %w", name, err)
	}

	var toolsResult MCPListToolsResult
	if err := json.Unmarshal(toolsResp.Result, &toolsResult); err != nil {
		return fmt.Errorf("parsing tools list from %s: %w", name, err)
	}

	// Register tools
	m.mu.Lock()
	m.servers[name] = t
	for _, tool := range toolsResult.Tools {
		m.tools[tool.Name] = &ManagedTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
			ServerName:  name,
		}
		m.routes[tool.Name] = name
	}
	m.mu.Unlock()

	return nil
}

// CallTool executes a tool on the appropriate MCP server.
func (m *Manager) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPToolCallResult, error) {
	m.mu.RLock()
	serverName, ok := m.routes[toolName]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("unknown MCP tool: %s", toolName)
	}
	t, ok := m.servers[serverName]
	if !ok {
		m.mu.RUnlock()
		return nil, fmt.Errorf("MCP server not connected: %s", serverName)
	}
	m.mu.RUnlock()

	params := MCPToolCallParams{
		Name:      toolName,
		Arguments: arguments,
	}

	resp, err := t.SendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("calling MCP tool %s: %w", toolName, err)
	}

	var result MCPToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parsing tool result: %w", err)
	}

	return &result, nil
}

// GetTools returns all discovered MCP tools.
func (m *Manager) GetTools() []*ManagedTool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]*ManagedTool, 0, len(m.tools))
	for _, t := range m.tools {
		tools = append(tools, t)
	}
	return tools
}

// HasTool checks if a tool is available.
func (m *Manager) HasTool(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.tools[name]
	return ok
}

// Shutdown closes all MCP server connections.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, t := range m.servers {
		if err := t.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing MCP server %s: %w", name, err)
		}
		delete(m.servers, name)
	}
	return firstErr
}

// ServerNames returns the names of all configured servers.
func (m *Manager) ServerNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// ToolCount returns the number of discovered tools.
func (m *Manager) ToolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tools)
}
