# MCP (Model Context Protocol) Integration

## Package: `internal/runtime/mcp/`

---

## Overview

MCP allows external tool servers to be discovered and called through a standard JSON-RPC protocol. The system manages multiple MCP server connections and routes tool calls to the appropriate server.

---

## MCP Server Configuration

### MCPServerConfig
```go
type MCPServerConfig struct {
    Transport string            // "stdio" | "sse" | "http" | "websocket"
    Command   string            // for stdio: command to start server
    Args      []string          // for stdio: command arguments
    URL       string            // for sse/http/websocket: server URL
    Env       map[string]string // additional environment variables
}
```

### MCP Client Bootstrap

```go
type McpClientBootstrap struct {
    Transport  McpClientTransport
    Auth       McpClientAuth
    ServerName string
}
```

**Transport types:**
```go
type McpClientTransport struct {
    Type    string // "stdio" | "sse" | "http" | "websocket" | "sdk" | "managed_proxy"
    Command string   // for stdio
    Args    []string // for stdio
    URL     string   // for network transports
}
```

**Auth types:**
```go
type McpClientAuth struct {
    Type string // "none" | "oauth"
}
```

---

## JSON-RPC Protocol (over stdio)

### Framing

Messages are framed using Content-Length headers (same as LSP):

```
Content-Length: {length}\r\n\r\n{json_body}
```

### JSON-RPC Types

```go
type JsonRpcId struct {
    Type   string // "number" | "string" | "null"
    Number int64
    String string
}

type JsonRpcRequest struct {
    JsonRpc string          `json:"jsonrpc"` // "2.0"
    ID      *JsonRpcId      `json:"id"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params"`
}

type JsonRpcResponse struct {
    JsonRpc string          `json:"jsonrpc"` // "2.0"
    ID      *JsonRpcId      `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *JsonRpcError   `json:"error,omitempty"`
}

type JsonRpcError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}
```

---

## MCP Protocol Messages

### Initialize

**Request:**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "claw-code", "version": "1.0.0"}
    }
}
```

**Response:**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
        "protocolVersion": "2024-11-05",
        "capabilities": {"tools": {}},
        "serverInfo": {"name": "...", "version": "..."}
    }
}
```

### List Tools

**Request:**
```json
{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
}
```

**Response:**
```json
{
    "jsonrpc": "2.0",
    "id": 2,
    "result": {
        "tools": [
            {
                "name": "tool_name",
                "description": "...",
                "inputSchema": { ... }
            }
        ]
    }
}
```

### Call Tool

**Request:**
```json
{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
        "name": "tool_name",
        "arguments": { ... }
    }
}
```

**Response:**
```json
{
    "jsonrpc": "2.0",
    "id": 3,
    "result": {
        "content": [
            {"type": "text", "text": "result text"}
        ]
    }
}
```

### List Resources

**Request:**
```json
{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "resources/list",
    "params": {}
}
```

### Read Resource

**Request:**
```json
{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "resources/read",
    "params": {
        "uri": "resource://path"
    }
}
```

### Shutdown

**Request:**
```json
{
    "jsonrpc": "2.0",
    "method": "notifications/cancelled",
    "params": { "requestId": 1, "reason": "shutdown" }
}
```

---

## McpServerManager

```go
type McpServerManager struct {
    Servers   map[string]*McpStdioProcess
    Tools     map[string]*ManagedMcpTool  // tool_name -> tool
    ToolRoute map[string]string           // tool_name -> server_name
}
```

### Initialize All Servers

```
init_all(configs map[string]MCPServerConfig):
  FOR EACH (name, config) in configs:
    IF config.Transport == "stdio":
      process = start_stdio_server(config.Command, config.Args, config.Env)
      send initialize request
      send initialized notification
      discover tools via tools/list
      FOR EACH discovered tool:
        Register in Tools map
        Register in ToolRoute (tool_name -> server_name)
    ELSE:
      Record as unsupported (sse/http/websocket not yet implemented)
```

### Start Stdio Server

```go
type McpStdioProcess struct {
    Cmd           *exec.Cmd
    Stdin         io.WriteCloser
    Stdout        *bufio.Reader
    NextRequestID int64
    Pending       map[int64]chan JsonRpcResponse
}
```

**Start logic:**
```
1. Create exec.Cmd with command + args
2. Set environment (OS env + config env)
3. Create stdin/stdout pipes
4. Start process
5. Start background goroutine to read responses:
   LOOP:
     a. Read Content-Length header
     b. Read JSON body
     c. Parse as JsonRpcResponse
     d. IF response has ID and Pending channel exists:
          Send response to channel
        ELSE:
          Handle notification (e.g. diagnostics update)
```

### Execute MCP Tool

```
call_tool(tool_name, arguments):
  1. Look up server_name from ToolRoute[tool_name]
  2. Get McpStdioProcess for that server
  3. Build tools/call request with arguments
  4. Send request and wait for response via pending channel
  5. Extract content from response
  6. Return content as tool result
```

---

## Tool Integration with Global Registry

MCP tools are merged into the GlobalToolRegistry:

```
1. McpServerManager discovers all MCP tools during initialization
2. FOR EACH MCP tool:
   - Create ToolSpec from McpTool (name, description, inputSchema)
   - Set RequiredPermission = "danger_full_access" (MCP tools are external)
   - Register in GlobalToolRegistry.mcpTools
3. When tool execution is requested:
   - GlobalToolRegistry checks builtin first, then MCP, then plugins
   - IF MCP tool: delegate to McpServerManager.call_tool()
```

---

## Error Handling

```go
type McpServerManagerError struct {
    Type    McpErrorType
    Message string
}

// Error types:
// Io           - process start/read/write failure
// JsonRpc      - JSON-RPC protocol error (non-zero error code)
// InvalidResponse - unexpected response structure
// UnknownTool  - tool not found in any MCP server
// UnknownServer - server name not found
```
