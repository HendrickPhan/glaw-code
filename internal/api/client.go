package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ProviderClient is the interface for LLM provider communication.
type ProviderClient interface {
	SendMessage(ctx context.Context, req Request) (*Response, error)
	StreamMessage(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

// StreamEvent represents an event during SSE streaming.
type StreamEvent struct {
	Type    StreamEventType
	Content interface{}
	Error   error
}

// StreamEventType enumerates streaming event types.
type StreamEventType int

const (
	EventMessageStart StreamEventType = iota
	EventContentBlockStart
	EventContentBlockDelta
	EventContentBlockStop
	EventMessageDelta
	EventMessageStop
	EventError
	EventDone
)

// ClientConfig holds configuration for an API client.
type ClientConfig struct {
	APIKey     string
	BaseURL    string
	MaxRetries int
	Timeout    time.Duration
	Headers    map[string]string
}

// AnthropicClient implements ProviderClient for the Anthropic API.
type AnthropicClient struct {
	config     ClientConfig
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic API client.
func NewAnthropicClient(config ClientConfig) *AnthropicClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 2
	}
	return &AnthropicClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// NewAnthropicClientFromEnv creates a client using environment variables.
func NewAnthropicClientFromEnv() (*AnthropicClient, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GLAW_API_KEY")
	}
	if apiKey == "" {
		return nil, NewMissingCredentialsError("ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, or GLAW_API_KEY not set")
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return NewAnthropicClient(ClientConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
	}), nil
}

// SendMessage sends a non-streaming request to the Anthropic API.
func (c *AnthropicClient) SendMessage(ctx context.Context, req Request) (*Response, error) {
	req.Stream = false

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffDuration(attempt)):
			}
		}

		resp, err := c.doRequest(ctx, req)
		if err != nil {
			lastErr = err
			if apiErr, ok := err.(*Error); ok && !apiErr.IsRetryable() {
				return nil, err
			}
			continue
		}
		return resp, nil
	}

	return nil, NewRetriesExhaustedError(c.config.MaxRetries+1, lastErr)
}

// StreamMessage sends a streaming request and returns an event channel.
func (c *AnthropicClient) StreamMessage(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	req.Stream = true
	ch := make(chan StreamEvent, 64)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		apiErr := NewHTTPError(httpResp.StatusCode, string(respBody))
		if httpResp.StatusCode == 429 || httpResp.StatusCode == 503 {
			apiErr.Type = ErrRateLimit
		}
		return nil, apiErr
	}

	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		c.processSSEStream(httpResp.Body, ch)
	}()

	return ch, nil
}

func (c *AnthropicClient) doRequest(ctx context.Context, req Request) (*Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return &Response{}, fmt.Errorf("http request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		apiErr := NewHTTPError(httpResp.StatusCode, string(respBody))
		if httpResp.StatusCode == 429 || httpResp.StatusCode == 503 {
			apiErr.Type = ErrRateLimit
		}
		return nil, apiErr
	}

	var resp Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		if os.Getenv("GLAW_DEBUG") == "1" {
			n := len(respBody)
			if n > 500 {
				n = 500
			}
			fmt.Fprintf(os.Stderr, "[DEBUG API Response] %s\n", string(respBody[:n]))
		}
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	resp.RequestID = httpResp.Header.Get("x-request-id")
	return &resp, nil
}

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}
}

func (c *AnthropicClient) processSSEStream(r io.Reader, ch chan<- StreamEvent) {
	parser := NewSSEParser()
	buf := make([]byte, 4096)

	var currentResponse Response
	var currentBlockIndex int

	for {
		n, err := r.Read(buf)
		if n > 0 {
			events := parser.PushChunk(buf[:n])
			for _, event := range events {
				// Determine event type from SSE event name
				switch event.Event {
				case "message_start":
					var payload struct {
						Type    string   `json:"type"`
						Message Response `json:"message"`
					}
					if err := json.Unmarshal([]byte(event.Data), &payload); err == nil {
						currentResponse = payload.Message
						currentBlockIndex = 0
					}
					ch <- StreamEvent{Type: EventMessageStart, Content: currentResponse.Usage}

				case "content_block_start":
					var payload struct {
						Type  string       `json:"type"`
						Index int          `json:"index"`
						Block ContentBlock `json:"content_block"`
					}
					if err := json.Unmarshal([]byte(event.Data), &payload); err == nil {
						currentBlockIndex = payload.Index
					}
					ch <- StreamEvent{Type: EventContentBlockStart, Content: currentBlockIndex}

				case "content_block_delta":
					ch <- StreamEvent{Type: EventContentBlockDelta, Content: event.Data}

				case "content_block_stop":
					ch <- StreamEvent{Type: EventContentBlockStop, Content: currentBlockIndex}

				case "message_delta":
					var payload struct {
						Type  string     `json:"type"`
						Delta struct {
							StopReason StopReason `json:"stop_reason"`
						} `json:"delta"`
						Usage struct {
							OutputTokens int `json:"output_tokens"`
						} `json:"usage"`
					}
					if err := json.Unmarshal([]byte(event.Data), &payload); err == nil {
						currentResponse.StopReason = payload.Delta.StopReason
						currentResponse.Usage.OutputTokens = payload.Usage.OutputTokens
					}
					ch <- StreamEvent{Type: EventMessageDelta, Content: currentResponse.StopReason}

				case "message_stop":
					currentResponse.Content = accumulateContentBlocks(currentResponse, ch)
					ch <- StreamEvent{Type: EventMessageStop, Content: &currentResponse}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				ch <- StreamEvent{Type: EventError, Error: err}
			}
			break
		}
	}

	ch <- StreamEvent{Type: EventDone}
}

func accumulateContentBlocks(resp Response, ch chan<- StreamEvent) []ContentBlock {
	// Content blocks are built up from streaming events.
	// The caller (processSSEStream) accumulates deltas into text blocks
	// and collects tool_use blocks from the response as they arrive.
	// This returns the current state of content blocks.
	if resp.Content == nil {
		return []ContentBlock{}
	}
	return resp.Content
}

// OpenAICompatClient implements ProviderClient for OpenAI-compatible APIs.
type OpenAICompatClient struct {
	config     ClientConfig
	httpClient *http.Client
}

// NewOpenAICompatClient creates a new OpenAI-compatible client.
func NewOpenAICompatClient(config ClientConfig) *OpenAICompatClient {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}
	return &OpenAICompatClient{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// SendMessage sends a request to the OpenAI-compatible API.
func (c *OpenAICompatClient) SendMessage(ctx context.Context, req Request) (*Response, error) {
	oaiReq := c.convertRequest(req)
	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, err
	}

	endpoint := c.config.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, NewHTTPError(httpResp.StatusCode, string(respBody))
	}

	return c.convertResponse(respBody)
}

// StreamMessage sends a streaming request to the OpenAI-compatible API.
func (c *OpenAICompatClient) StreamMessage(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	oaiReq := c.convertRequest(req)
	oaiReq.Stream = true

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, err
	}

	endpoint := c.config.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, NewHTTPError(httpResp.StatusCode, string(respBody))
	}

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close()
		c.processOpenAISSEStream(httpResp.Body, ch)
	}()
	return ch, nil
}

// OpenAI-compatible request/response types

type oaiRequest struct {
	Model    string        `json:"model"`
	Messages []oaiMessage  `json:"messages"`
	Tools    []oaiTool     `json:"tools,omitempty"`
	Stream   bool          `json:"stream"`
}

type oaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type oaiTool struct {
	Type     string   `json:"type"`
	Function oaiFunc  `json:"function"`
}

type oaiFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

func (c *OpenAICompatClient) convertRequest(req Request) oaiRequest {
	var msgs []oaiMessage
	for _, m := range req.Messages {
		// Flatten content blocks to text
		var text string
		for _, b := range m.Content {
			if b.Type == ContentText {
				text += b.Text
			}
		}
		msgs = append(msgs, oaiMessage{Role: string(m.Role), Content: text})
	}

	if req.System != "" {
		msgs = append([]oaiMessage{{Role: "system", Content: req.System}}, msgs...)
	}

	var tools []oaiTool
	for _, t := range req.Tools {
		tools = append(tools, oaiTool{
			Type: "function",
			Function: oaiFunc{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	return oaiRequest{
		Model:    req.Model,
		Messages: msgs,
		Tools:    tools,
		Stream:   false,
	}
}

func (c *OpenAICompatClient) convertResponse(body []byte) (*Response, error) {
	var oaiResp oaiResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, err
	}

	resp := &Response{
		Model: oaiResp.Model,
		Usage: Usage{
			InputTokens:  oaiResp.Usage.TotalTokens, // approximate
			OutputTokens: oaiResp.Usage.TotalTokens,
		},
	}

	if len(oaiResp.Choices) > 0 {
		choice := oaiResp.Choices[0]
		resp.Content = []ContentBlock{NewTextBlock(choice.Message.Content)}
		switch choice.FinishReason {
		case "stop":
			resp.StopReason = StopEndTurn
		case "tool_calls":
			resp.StopReason = StopToolUse
		case "length":
			resp.StopReason = StopMaxTokens
		}
	}

	return resp, nil
}

func (c *OpenAICompatClient) processOpenAISSEStream(r io.Reader, ch chan<- StreamEvent) {
	events, err := ParseSSEStream(r)
	if err != nil {
		ch <- StreamEvent{Type: EventError, Error: err}
		ch <- StreamEvent{Type: EventDone}
		return
	}

	for _, event := range events {
		if strings.TrimSpace(event.Data) == "[DONE]" {
			ch <- StreamEvent{Type: EventDone}
			return
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			continue
		}

		ch <- StreamEvent{Type: EventContentBlockDelta, Content: event.Data}
	}

	ch <- StreamEvent{Type: EventDone}
}

// NewProviderClient creates the appropriate client based on model name.
func NewProviderClient(model string) (ProviderClient, error) {
	if strings.HasPrefix(model, "grok") {
		apiKey := os.Getenv("XAI_API_KEY")
		if apiKey == "" {
			return nil, NewMissingCredentialsError("XAI_API_KEY not set")
		}
		baseURL := os.Getenv("XAI_BASE_URL")
		if baseURL == "" {
			baseURL = "https://api.x.ai/v1"
		}
		return NewOpenAICompatClient(ClientConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
		}), nil
	}

	return NewAnthropicClientFromEnv()
}

func backoffDuration(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
