package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Error represents an LSP-related error.
type Error struct {
	Message string
	Code    int
}

func (e *Error) Error() string {
	return fmt.Sprintf("lsp error: %s", e.Message)
}

// jsonrpcRequest represents a JSON-RPC request.
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// jsonrpcNotification is a JSON-RPC notification (no ID).
type jsonrpcNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// Client manages a connection to a language server.
type Client struct {
	config        ServerConfig
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        *bufio.Reader
	nextID        atomic.Int64
	pending       map[int64]chan json.RawMessage
	diagnostics   map[string][]Diagnostic
	openDocuments map[string]bool
	mu            sync.RWMutex
	cancel        context.CancelFunc
}

// NewClient creates a new LSP client (does not connect yet).
func NewClient(config ServerConfig) *Client {
	return &Client{
		config:        config,
		pending:       make(map[int64]chan json.RawMessage),
		diagnostics:   make(map[string][]Diagnostic),
		openDocuments: make(map[string]bool),
	}
}

// Connect starts the language server process and sends initialize.
func (c *Client) Connect(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.cmd = exec.CommandContext(ctx, c.config.Command, c.config.Args...)
	if c.config.Env != nil {
		env := os.Environ()
		for k, v := range c.config.Env {
			env = append(env, k+"="+v)
		}
		c.cmd.Env = env
	}

	stdinPipe, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}
	c.stdin = stdinPipe

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReaderSize(stdoutPipe, 64*1024)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("starting LSP server %s: %w", c.config.Name, err)
	}

	// Start background reader
	go c.readLoop()

	// Send initialize request
	workspaceURI := "file://" + c.config.WorkspaceRoot
	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   workspaceURI,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"definition":  map[string]interface{}{"dynamicRegistration": false},
				"references":  map[string]interface{}{"dynamicRegistration": false},
				"publishDiagnostics": map[string]interface{}{"relatedInformation": true},
			},
		},
	}

	if c.config.InitializationOptions != nil {
		params["initializationOptions"] = json.RawMessage(c.config.InitializationOptions)
	}

	_, err = c.sendRequest(ctx, "initialize", params)
	if err != nil {
		c.Close()
		return fmt.Errorf("initializing LSP server %s: %w", c.config.Name, err)
	}

	// Send initialized notification
	if err := c.sendNotification("initialized", map[string]interface{}{}); err != nil {
		c.Close()
		return fmt.Errorf("sending initialized to %s: %w", c.config.Name, err)
	}

	return nil
}

// OpenDocument opens a file in the language server.
func (c *Client) OpenDocument(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file %s: %w", filePath, err)
	}

	ext := filepath.Ext(filePath)
	lang := "unknown"
	if c.config.ExtensionToLanguage != nil {
		if l, ok := c.config.ExtensionToLanguage[NormalizeExtension(ext)]; ok {
			lang = l
		}
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": lang,
			"version":    0,
			"text":       string(content),
		},
	}

	if err := c.sendNotification("textDocument/didOpen", params); err != nil {
		return err
	}

	c.mu.Lock()
	c.openDocuments[filePath] = true
	c.mu.Unlock()
	return nil
}

// ChangeDocument updates a file's content in the language server.
func (c *Client) ChangeDocument(ctx context.Context, filePath string, content string) error {
	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":     uri,
			"version": 1,
		},
		"contentChanges": []map[string]interface{}{
			{"text": content},
		},
	}
	return c.sendNotification("textDocument/didChange", params)
}

// CloseDocument closes a file in the language server.
func (c *Client) CloseDocument(ctx context.Context, filePath string) error {
	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}

	if err := c.sendNotification("textDocument/didClose", params); err != nil {
		return err
	}

	c.mu.Lock()
	delete(c.openDocuments, filePath)
	delete(c.diagnostics, filePath)
	c.mu.Unlock()
	return nil
}

// SaveDocument notifies the server that a file was saved.
func (c *Client) SaveDocument(ctx context.Context, filePath string) error {
	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}
	return c.sendNotification("textDocument/didSave", params)
}

// GoToDefinition finds where a symbol is defined.
func (c *Client) GoToDefinition(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return nil, err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result, filePath)
}

// FindReferences finds all references to a symbol.
func (c *Client) FindReferences(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return nil, err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
		"context":      map[string]interface{}{"includeDeclaration": true},
	}

	result, err := c.sendRequest(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result, filePath)
}

// Hover returns hover information for a symbol at the given position.
func (c *Client) Hover(ctx context.Context, filePath string, line, character int) (string, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return "", err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/hover", params)
	if err != nil {
		return "", err
	}

	if len(result) == 0 || string(result) == "null" {
		return "", nil
	}

	var hover struct {
		Contents struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return "", nil
	}

	return hover.Contents.Value, nil
}

// DocumentSymbol returns symbols within a document.
func (c *Client) DocumentSymbol(ctx context.Context, filePath string) ([]SymbolLocation, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return nil, err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	}

	result, err := c.sendRequest(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	return parseDocumentSymbols(result, filePath)
}

// WorkspaceSymbol searches for symbols across the workspace.
func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolLocation, error) {
	params := map[string]interface{}{
		"query": query,
	}

	result, err := c.sendRequest(ctx, "workspace/symbol", params)
	if err != nil {
		return nil, err
	}

	return parseWorkspaceSymbols(result)
}

// GoToImplementation finds implementations of a symbol.
func (c *Client) GoToImplementation(ctx context.Context, filePath string, line, character int) ([]SymbolLocation, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return nil, err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/implementation", params)
	if err != nil {
		return nil, err
	}

	return parseLocations(result, filePath)
}

// CallHierarchyItem represents an item in a call hierarchy.
type CallHierarchyItem struct {
	Name       string         `json:"name"`
	Kind       int            `json:"kind"`
	URI        string         `json:"uri"`
	Range      Range          `json:"range"`
	Selection  Range          `json:"selectionRange"`
}

// PrepareCallHierarchy prepares call hierarchy items for a position.
func (c *Client) PrepareCallHierarchy(ctx context.Context, filePath string, line, character int) ([]CallHierarchyItem, error) {
	if err := c.ensureDocumentOpen(ctx, filePath); err != nil {
		return nil, err
	}

	uri := "file://" + filePath
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": line, "character": character},
	}

	result, err := c.sendRequest(ctx, "textDocument/prepareCallHierarchy", params)
	if err != nil {
		return nil, err
	}

	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var items []CallHierarchyItem
	if err := json.Unmarshal(result, &items); err != nil {
		return nil, nil
	}

	return items, nil
}

// IncomingCalls returns callers of the given call hierarchy item.
func (c *Client) IncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyItem, error) {
	params := map[string]interface{}{
		"item": map[string]interface{}{
			"name":            item.Name,
			"kind":            item.Kind,
			"uri":             item.URI,
			"range":           item.Range,
			"selectionRange":  item.Selection,
		},
	}

	result, err := c.sendRequest(ctx, "callHierarchy/incomingCalls", params)
	if err != nil {
		return nil, err
	}

	return parseCallHierarchyCalls(result)
}

// OutgoingCalls returns callees of the given call hierarchy item.
func (c *Client) OutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyItem, error) {
	params := map[string]interface{}{
		"item": map[string]interface{}{
			"name":            item.Name,
			"kind":            item.Kind,
			"uri":             item.URI,
			"range":           item.Range,
			"selectionRange":  item.Selection,
		},
	}

	result, err := c.sendRequest(ctx, "callHierarchy/outgoingCalls", params)
	if err != nil {
		return nil, err
	}

	return parseCallHierarchyCalls(result)
}

// DiagnosticsSnapshot returns current diagnostics for all files.
func (c *Client) DiagnosticsSnapshot() map[string][]Diagnostic {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]Diagnostic, len(c.diagnostics))
	for k, v := range c.diagnostics {
		result[k] = v
	}
	return result
}

// Close shuts down the language server.
func (c *Client) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.stdin != nil {
		// Send shutdown request (best effort)
		ctx, cancel := context.WithTimeout(context.Background(), 5000)
		defer cancel()
		_, _ = c.sendRequest(ctx, "shutdown", nil)
		_ = c.sendNotification("exit", nil)
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return nil
}

func (c *Client) ensureDocumentOpen(ctx context.Context, filePath string) error {
	c.mu.RLock()
	open := c.openDocuments[filePath]
	c.mu.RUnlock()
	if open {
		return nil
	}
	return c.OpenDocument(ctx, filePath)
}

func (c *Client) sendRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan json.RawMessage, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.writeMessage(req); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) sendNotification(method string, params interface{}) error {
	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(notif)
}

func (c *Client) writeMessage(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	_, err = c.stdin.Write(data)
	return err
}

func (c *Client) readLoop() {
	for {
		var contentLength int
		for {
			line, err := c.stdout.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "Content-Length:") {
				clStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				contentLength, _ = strconv.Atoi(clStr)
			}
		}

		if contentLength == 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.stdout, body); err != nil {
			return
		}

		// Parse response
		var raw struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			continue
		}

		if raw.ID != 0 {
			// Response to a request
			c.mu.Lock()
			ch, ok := c.pending[raw.ID]
			c.mu.Unlock()
			if ok {
				ch <- raw.Result
			}
		} else if raw.Method == "textDocument/publishDiagnostics" {
			// Diagnostic notification
			var params struct {
				URI         string       `json:"uri"`
				Diagnostics []Diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(raw.Params, &params); err == nil {
				path := strings.TrimPrefix(params.URI, "file://")
				c.mu.Lock()
				c.diagnostics[path] = params.Diagnostics
				c.mu.Unlock()
			}
		}
	}
}

// parseLocations extracts symbol locations from an LSP response.
func parseLocations(result json.RawMessage, basePath string) ([]SymbolLocation, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	// Try array of locations first
	var locations []struct {
		URI   string `json:"uri"`
		Range Range  `json:"range"`
	}

	// Try as direct array
	if err := json.Unmarshal(result, &locations); err != nil {
		// Try as single location
		var loc struct {
			URI   string `json:"uri"`
			Range Range  `json:"range"`
		}
		if err := json.Unmarshal(result, &loc); err != nil {
			return nil, nil
		}
		locations = []struct {
			URI   string `json:"uri"`
			Range Range  `json:"range"`
		}{loc}
	}

	var result2 []SymbolLocation
	for _, loc := range locations {
		path := strings.TrimPrefix(loc.URI, "file://")
		result2 = append(result2, SymbolLocation{
			Path: path,
			Line: loc.Range.Start.Line + 1,
			Col:  loc.Range.Start.Character + 1,
		})
	}

	return dedupeLocations(result2), nil
}

// parseDocumentSymbols extracts symbol locations from a textDocument/documentSymbol response.
func parseDocumentSymbols(result json.RawMessage, basePath string) ([]SymbolLocation, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	// Document symbols can be either DocumentSymbol[] or SymbolInformation[]
	// Try SymbolInformation[] first (flat list with locations)
	var symInfos []struct {
		Name string `json:"name"`
		Kind int    `json:"kind"`
		Location struct {
			URI   string `json:"uri"`
			Range Range  `json:"range"`
		} `json:"location"`
	}
	if err := json.Unmarshal(result, &symInfos); err == nil && len(symInfos) > 0 {
		var locs []SymbolLocation
		for _, si := range symInfos {
			path := strings.TrimPrefix(si.Location.URI, "file://")
			locs = append(locs, SymbolLocation{
				Path: path,
				Line: si.Location.Range.Start.Line + 1,
				Col:  si.Location.Range.Start.Character + 1,
			})
		}
		return locs, nil
	}

	// Try DocumentSymbol[] (nested with range but no location URI)
	var docSymbols []struct {
		Name  string `json:"name"`
		Kind  int    `json:"kind"`
		Range Range  `json:"range"`
	}
	if err := json.Unmarshal(result, &docSymbols); err == nil {
		var locs []SymbolLocation
		for _, ds := range docSymbols {
			locs = append(locs, SymbolLocation{
				Path: basePath,
				Line: ds.Range.Start.Line + 1,
				Col:  ds.Range.Start.Character + 1,
			})
		}
		return locs, nil
	}

	return nil, nil
}

// parseWorkspaceSymbols extracts symbol locations from a workspace/symbol response.
func parseWorkspaceSymbols(result json.RawMessage) ([]SymbolLocation, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	var symbols []struct {
		Name     string `json:"name"`
		Kind     int    `json:"kind"`
		Location struct {
			URI   string `json:"uri"`
			Range Range  `json:"range"`
		} `json:"location"`
	}
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, nil
	}

	var locs []SymbolLocation
	for _, s := range symbols {
		path := strings.TrimPrefix(s.Location.URI, "file://")
		locs = append(locs, SymbolLocation{
			Path: path,
			Line: s.Location.Range.Start.Line + 1,
			Col:  s.Location.Range.Start.Character + 1,
		})
	}

	return locs, nil
}

// parseCallHierarchyCalls extracts call hierarchy items from incoming/outgoing call responses.
func parseCallHierarchyCalls(result json.RawMessage) ([]CallHierarchyItem, error) {
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}

	// Incoming/outgoing calls have the structure: [{ from/to: CallHierarchyItem }]
	var rawCalls []json.RawMessage
	if err := json.Unmarshal(result, &rawCalls); err != nil {
		return nil, nil
	}

	var items []CallHierarchyItem
	for _, raw := range rawCalls {
		// Try "from" key (incoming calls) then "to" key (outgoing calls)
		var wrapper struct {
			From CallHierarchyItem `json:"from"`
			To   CallHierarchyItem `json:"to"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			continue
		}
		if wrapper.From.Name != "" {
			items = append(items, wrapper.From)
		} else if wrapper.To.Name != "" {
			items = append(items, wrapper.To)
		}
	}

	return items, nil
}

func dedupeLocations(locs []SymbolLocation) []SymbolLocation {
	seen := make(map[string]bool)
	var result []SymbolLocation
	for _, loc := range locs {
		key := fmt.Sprintf("%s:%d:%d", loc.Path, loc.Line, loc.Col)
		if !seen[key] {
			seen[key] = true
			result = append(result, loc)
		}
	}
	return result
}
