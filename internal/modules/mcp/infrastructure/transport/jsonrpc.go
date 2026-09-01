package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// JSON-RPC types for MCP protocol.

type JSONRPCID struct {
	Num      int64
	Str      string
	IsString bool
}

func (id JSONRPCID) MarshalJSON() ([]byte, error) {
	if id.IsString {
		return json.Marshal(id.Str)
	}
	return json.Marshal(id.Num)
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// MCP Protocol Types

type MCPInitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    MCPCapabilities `json:"capabilities"`
	ClientInfo      MCPClientInfo   `json:"clientInfo"`
}

type MCPCapabilities struct{}

type MCPClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type MCPInitializeResult struct {
	ProtocolVersion string                `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo         `json:"serverInfo"`
}

type MCPServerCapabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type MCPListToolsResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type MCPToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type MCPToolCallResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPListResourcesResult struct {
	Resources []MCPResource `json:"resources"`
}

type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType,omitempty"`
}

type MCPReadResourceParams struct {
	URI string `json:"uri"`
}

type MCPReadResourceResult struct {
	Contents []MCPResourceContents `json:"contents"`
}

type MCPResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// StdioProcess manages a child process communicating via JSON-RPC over stdio.
type StdioProcess struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	mu         sync.Mutex
	nextID     int64
	pending    map[int64]chan *JSONRPCResponse
	cancelFunc context.CancelFunc
}

// NewStdioProcess starts a child process for JSON-RPC communication.
// The process lifetime is independent of the caller's context deadline:
// handshake timeouts must not kill the long-lived server process. The
// process is terminated by Close, or exits on its own.
func NewStdioProcess(ctx context.Context, command string, args []string, env map[string]string) (*StdioProcess, error) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(ctx, command, args...)

	// Set environment
	cmd.Env = append(cmd.Environ(), envToSlice(env)...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating stdout pipe: %w", err)
	}
	// Forward the child's stderr to our stderr so server-side startup
	// errors are visible instead of silently discarded.
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("starting process %s: %w", command, err)
	}

	p := &StdioProcess{
		cmd:        cmd,
		stdin:      stdinPipe,
		stdout:     bufio.NewReaderSize(stdoutPipe, 64*1024),
		nextID:     1,
		pending:    make(map[int64]chan *JSONRPCResponse),
		cancelFunc: cancel,
	}

	// Start background reader
	go p.readLoop()

	return p, nil
}

// SendRequest sends a JSON-RPC request and waits for the response.
func (p *StdioProcess) SendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	p.mu.Lock()
	id := p.nextID
	p.nextID++
	ch := make(chan *JSONRPCResponse, 1)
	p.pending[id] = ch
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		delete(p.pending, id)
		p.mu.Unlock()
	}()

	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshaling params: %w", err)
		}
		paramsJSON = b
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  paramsJSON,
	}

	if err := p.writeMessage(req); err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp, resp.Error
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendNotification sends a JSON-RPC notification (no ID, no response expected).
func (p *StdioProcess) SendNotification(method string, params interface{}) error {
	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsJSON = b
	}

	notif := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}
	return p.writeMessage(notif)
}

// closeGracePeriod is how long Close waits for the server to exit on its own
// after stdin is closed, before force-killing it.
const closeGracePeriod = time.Second

// Close shuts down the process: it first closes stdin (a well-behaved
// server exits on EOF), then force-kills if it lingers. The forced kill is
// expected and is not reported as an error.
func (p *StdioProcess) Close() error {
	_ = p.stdin.Close()

	waitCh := make(chan error, 1)
	go func() { waitCh <- p.cmd.Wait() }()

	select {
	case err := <-waitCh:
		// ExitError means the server exited non-zero on its own; the
		// connection is closed either way, so it is not a Close failure.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	case <-time.After(closeGracePeriod):
		p.cancelFunc() // kill
		<-waitCh
		return nil
	}
}

// writeMessage writes a single JSON-RPC message. The MCP stdio transport is
// newline-delimited JSON (one message per line) — unlike LSP, there are no
// Content-Length headers.
func (p *StdioProcess) writeMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := p.stdin.Write(data); err != nil {
		return err
	}
	_, err = p.stdin.Write([]byte("\n"))
	return err
}

func (p *StdioProcess) readLoop() {
	for {
		resp, err := p.readMessage()
		if err != nil {
			p.failAllPending(err)
			return
		}
		if resp.ID != 0 {
			p.mu.Lock()
			ch, ok := p.pending[resp.ID]
			p.mu.Unlock()
			if ok {
				ch <- resp
			}
		}
	}
}

// failAllPending unblocks all in-flight requests when the connection dies,
// so callers never hang waiting on a dead server.
func (p *StdioProcess) failAllPending(cause error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, ch := range p.pending {
		ch <- &JSONRPCResponse{
			ID:    id,
			Error: &JSONRPCError{Code: -32000, Message: fmt.Sprintf("MCP server connection closed: %v", cause)},
		}
		delete(p.pending, id)
	}
}

// readMessage reads a single newline-delimited JSON-RPC message. Non-JSON
// lines (server log noise on stdout) are skipped.
func (p *StdioProcess) readMessage() (*JSONRPCResponse, error) {
	for {
		line, err := p.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue
		}
		return &resp, nil
	}
}

func envToSlice(env map[string]string) []string {
	var result []string
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
