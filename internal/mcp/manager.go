// Package mcp re-exports types from internal/modules/mcp/infrastructure/transport.
// The canonical implementation lives in the modules structure.
package mcp

import transport "github.com/hieu-glaw/glaw-code/internal/modules/mcp/infrastructure/transport"

// Re-export all types
type ServerConfig = transport.ServerConfig
type Transport = transport.Transport
type ManagedTool = transport.ManagedTool
type Manager = transport.Manager

// Re-export JSON-RPC types
type JSONRPCID = transport.JSONRPCID
type JSONRPCRequest = transport.JSONRPCRequest
type JSONRPCResponse = transport.JSONRPCResponse
type JSONRPCError = transport.JSONRPCError

// Re-export MCP types
type MCPInitializeParams = transport.MCPInitializeParams
type MCPCapabilities = transport.MCPCapabilities
type MCPClientInfo = transport.MCPClientInfo
type MCPInitializeResult = transport.MCPInitializeResult
type MCPServerCapabilities = transport.MCPServerCapabilities
type MCPServerInfo = transport.MCPServerInfo
type MCPListToolsResult = transport.MCPListToolsResult
type MCPTool = transport.MCPTool
type MCPToolCallParams = transport.MCPToolCallParams
type MCPToolCallResult = transport.MCPToolCallResult
type MCPContent = transport.MCPContent
type MCPListResourcesResult = transport.MCPListResourcesResult
type MCPResource = transport.MCPResource
type MCPReadResourceParams = transport.MCPReadResourceParams
type MCPReadResourceResult = transport.MCPReadResourceResult
type MCPResourceContents = transport.MCPResourceContents
type StdioProcess = transport.StdioProcess
type SSEClient = transport.SSEClient

// Re-export functions
var NewManager = transport.NewManager
var NewStdioProcess = transport.NewStdioProcess
var NewSSEClient = transport.NewSSEClient
