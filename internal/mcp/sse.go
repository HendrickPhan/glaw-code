package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// defaultHTTPTimeout is used for notification POST requests that have no
// caller-supplied context deadline.
const defaultHTTPTimeout = 30 * time.Second

// SSEClient implements Transport for HTTP-based MCP servers using the
// Streamable HTTP transport (POST requests with JSON or SSE responses).
type SSEClient struct {
	url        string
	headers    map[string]string
	httpClient *http.Client

	nextID int64
}

// NewSSEClient creates a new HTTP-based MCP transport.
func NewSSEClient(url string, headers map[string]string) (*SSEClient, error) {
	c := &SSEClient{
		url:        strings.TrimSuffix(url, "/"),
		headers:    headers,
		httpClient: &http.Client{},
		nextID:     1,
	}
	return c, nil
}

// SendRequest sends a JSON-RPC request via HTTP POST and reads the response.
// Supports both direct JSON responses and SSE-streamed responses (Streamable HTTP).
func (c *SSEClient) SendRequest(ctx context.Context, method string, params interface{}) (*JSONRPCResponse, error) {
	id := atomic.AddInt64(&c.nextID, 1)

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

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating POST request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Content-Length", strconv.Itoa(len(body)))
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("POST request to %s: %w", c.url, err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if len(respBody) == 0 {
		return nil, fmt.Errorf("empty response from %s (status %d)", c.url, httpResp.StatusCode)
	}

	// Try parsing as direct JSON-RPC response first.
	var resp JSONRPCResponse
	if err := json.Unmarshal(respBody, &resp); err == nil && resp.ID == id {
		if resp.Error != nil {
			return &resp, resp.Error
		}
		return &resp, nil
	}

	// Try parsing as SSE stream (data: lines containing JSON-RPC responses).
	resp, err = parseSSEResponse(respBody, id)
	if err != nil {
		return nil, fmt.Errorf("parsing response from %s: %w", c.url, err)
	}
	if resp.Error != nil {
		return &resp, resp.Error
	}
	return &resp, nil
}

// SendNotification sends a JSON-RPC notification via HTTP POST.
// Notifications have no ID and no response is expected.
func (c *SSEClient) SendNotification(method string, params interface{}) error {
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

	body, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshaling notification: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating notification POST: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	httpReq.Header.Set("Content-Length", strconv.Itoa(len(body)))
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("POST notification to %s: %w", c.url, err)
	}
	httpResp.Body.Close()

	return nil
}

// Close shuts down the HTTP client.
func (c *SSEClient) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// parseSSEResponse extracts a JSON-RPC response from an SSE-formatted body.
func parseSSEResponse(data []byte, targetID int64) (JSONRPCResponse, error) {
	var resp JSONRPCResponse
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var candidate JSONRPCResponse
		if err := json.Unmarshal([]byte(payload), &candidate); err != nil {
			continue
		}
		if candidate.ID == targetID {
			return candidate, nil
		}
	}
	return resp, fmt.Errorf("no response with id %d found in SSE stream", targetID)
}

// Compile-time check that SSEClient implements Transport.
var _ Transport = (*SSEClient)(nil)
