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
	ModelAlias string // optional model name override (e.g., stripping "ollama:" prefix)
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
	Model       string       `json:"model"`
	Messages    []oaiMessage `json:"messages"`
	Tools       []oaiTool    `json:"tools,omitempty"`
	Stream      bool         `json:"stream"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float64      `json:"temperature,omitempty"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`                  // nullable when tool_calls present
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`   // for role:"tool"
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiToolCallFunc `json:"function"`
}

type oaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string  `json:"type"`
	Function oaiFunc `json:"function"`
}

type oaiFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

func (c *OpenAICompatClient) convertRequest(req Request) oaiRequest {
	var msgs []oaiMessage

	// System message first
	if req.System != "" {
		sysContent := req.System
		msgs = append(msgs, oaiMessage{Role: "system", Content: &sysContent})
	}

	for _, m := range req.Messages {
		// Check if this message contains tool_result blocks
		hasToolResult := false
		for _, b := range m.Content {
			if b.Type == ContentToolResult {
				hasToolResult = true
				break
			}
		}

		if hasToolResult {
			// Each tool_result becomes its own "tool" role message
			for _, b := range m.Content {
				if b.Type == ContentToolResult {
					content := b.Content
					msgs = append(msgs, oaiMessage{
						Role:       "tool",
						Content:    &content,
						ToolCallID: b.ToolUseID,
					})
				}
			}
			continue
		}

		// For assistant messages: collect text + tool_use blocks
		if m.Role == RoleAssistant {
			var textParts []string
			var toolCalls []oaiToolCall
			for _, b := range m.Content {
				switch b.Type {
				case ContentText:
					textParts = append(textParts, b.Text)
				case ContentToolUse:
					toolCalls = append(toolCalls, oaiToolCall{
						ID:   b.ID,
						Type: "function",
						Function: oaiToolCallFunc{
							Name:      b.Name,
							Arguments: string(b.Input),
						},
					})
				}
			}
			content := strings.Join(textParts, "")
			msg := oaiMessage{Role: "assistant", Content: &content}
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
			msgs = append(msgs, msg)
			continue
		}

		// Default: user text message
		var textParts []string
		for _, b := range m.Content {
			if b.Type == ContentText {
				textParts = append(textParts, b.Text)
			}
		}
		content := strings.Join(textParts, "")
		msgs = append(msgs, oaiMessage{Role: string(m.Role), Content: &content})
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

	modelName := req.Model
	if c.config.ModelAlias != "" {
		modelName = c.config.ModelAlias
	}

	return oaiRequest{
		Model:       modelName,
		Messages:    msgs,
		Tools:       tools,
		Stream:      false,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
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
			InputTokens:  oaiResp.Usage.PromptTokens,
			OutputTokens: oaiResp.Usage.CompletionTokens,
		},
	}

	if len(oaiResp.Choices) > 0 {
		choice := oaiResp.Choices[0]

		var blocks []ContentBlock

		// Text content (may be nil/empty when tool_calls present)
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			blocks = append(blocks, NewTextBlock(*choice.Message.Content))
		}

		// Tool calls
		for _, tc := range choice.Message.ToolCalls {
			blocks = append(blocks, NewToolUseBlock(
				tc.ID,
				tc.Function.Name,
				json.RawMessage(tc.Function.Arguments),
			))
		}

		if len(blocks) == 0 {
			blocks = []ContentBlock{NewTextBlock("")}
		}
		resp.Content = blocks

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
	parser := NewSSEParser()
	buf := make([]byte, 4096)

	// Accumulate tool call arguments by index
	type toolCallAcc struct {
		ID   string
		Name string
		Args strings.Builder
	}
	toolCallAccum := make(map[int]*toolCallAcc)

	var response Response

	ch <- StreamEvent{Type: EventMessageStart, Content: nil}

	for {
		n, err := r.Read(buf)
		if n > 0 {
			events := parser.PushChunk(buf[:n])
			for _, event := range events {
				data := strings.TrimSpace(event.Data)
				if data == "[DONE]" {
					// Finalize accumulated tool calls into response content
					for i := 0; i < len(toolCallAccum); i++ {
						acc, ok := toolCallAccum[i]
						if !ok {
							continue
						}
						response.Content = append(response.Content, NewToolUseBlock(
							acc.ID,
							acc.Name,
							json.RawMessage(acc.Args.String()),
						))
					}
					if len(response.Content) == 0 {
						response.Content = []ContentBlock{}
					}
					ch <- StreamEvent{Type: EventMessageStop, Content: &response}
					ch <- StreamEvent{Type: EventDone}
					return
				}

				var chunk struct {
					Choices []struct {
						Delta struct {
							Role      string `json:"role"`
							Content   *string `json:"content"`
							ToolCalls []struct {
								Index    int    `json:"index"`
								ID       string `json:"id"`
								Type     string `json:"type"`
								Function struct {
									Name      string `json:"name"`
									Arguments string `json:"arguments"`
								} `json:"function"`
							} `json:"tool_calls"`
						} `json:"delta"`
						FinishReason *string `json:"finish_reason"`
					} `json:"choices"`
				}
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					continue
				}
				if len(chunk.Choices) == 0 {
					continue
				}

				delta := chunk.Choices[0].Delta
				finishReason := chunk.Choices[0].FinishReason

				// Text delta
				if delta.Content != nil && *delta.Content != "" {
					ch <- StreamEvent{Type: EventContentBlockDelta, Content: *delta.Content}
				}

				// Tool call deltas — accumulate by index
				for _, tc := range delta.ToolCalls {
					acc, ok := toolCallAccum[tc.Index]
					if !ok {
						acc = &toolCallAcc{}
						toolCallAccum[tc.Index] = acc
					}
					if tc.ID != "" {
						acc.ID = tc.ID
					}
					if tc.Function.Name != "" {
						acc.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.Args.WriteString(tc.Function.Arguments)
					}
				}

				// Finish reason
				if finishReason != nil {
					switch *finishReason {
					case "stop":
						response.StopReason = StopEndTurn
					case "tool_calls":
						response.StopReason = StopToolUse
					case "length":
						response.StopReason = StopMaxTokens
					}
					ch <- StreamEvent{Type: EventMessageDelta, Content: response.StopReason}
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

	// Finalize on unexpected stream end
	for i := 0; i < len(toolCallAccum); i++ {
		acc, ok := toolCallAccum[i]
		if !ok {
			continue
		}
		response.Content = append(response.Content, NewToolUseBlock(
			acc.ID,
			acc.Name,
			json.RawMessage(acc.Args.String()),
		))
	}
	if len(response.Content) == 0 {
		response.Content = []ContentBlock{}
	}
	ch <- StreamEvent{Type: EventMessageStop, Content: &response}
	ch <- StreamEvent{Type: EventDone}
}

// ProviderType identifies the LLM provider.
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGemini    ProviderType = "gemini"
	ProviderOllama    ProviderType = "ollama"
	ProviderXAI       ProviderType = "xai"
	ProviderOpenRouter ProviderType = "openrouter"
)

// ProviderInfo holds resolved provider configuration.
type ProviderInfo struct {
	Type    ProviderType
	APIKey  string
	BaseURL string
}

// DetectProvider determines the provider from model name and env vars.
func DetectProvider(model string) ProviderInfo {
	lower := strings.ToLower(model)

	switch {
	case strings.HasPrefix(lower, "grok"):
		return ProviderInfo{Type: ProviderXAI}

	case strings.HasPrefix(lower, "gpt-"),
		strings.HasPrefix(lower, "o3"),
		strings.HasPrefix(lower, "o4-"),
		strings.HasPrefix(lower, "chatgpt-"):
		return ProviderInfo{Type: ProviderOpenAI}

	case strings.HasPrefix(lower, "gemini-"):
		return ProviderInfo{Type: ProviderGemini}

	case strings.HasPrefix(lower, "ollama:"):
		return ProviderInfo{Type: ProviderOllama}

	case strings.HasPrefix(lower, "openrouter:"):
		return ProviderInfo{Type: ProviderOpenRouter}

	default:
		// Check if OLLAMA_BASE_URL is set with a non-prefixed model
		if os.Getenv("OLLAMA_BASE_URL") != "" || os.Getenv("OLLAMA_HOST") != "" {
			return ProviderInfo{Type: ProviderOllama}
		}
		return ProviderInfo{Type: ProviderAnthropic}
	}
}

// Resolve fills in APIKey and BaseURL from environment variables.
func (p ProviderInfo) Resolve(model string) (ProviderInfo, error) {
	switch p.Type {
	case ProviderXAI:
		p.APIKey = os.Getenv("XAI_API_KEY")
		if p.APIKey == "" {
			return p, NewMissingCredentialsError("XAI_API_KEY not set")
		}
		p.BaseURL = os.Getenv("XAI_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = "https://api.x.ai/v1"
		}

	case ProviderOpenAI:
		p.APIKey = os.Getenv("OPENAI_API_KEY")
		if p.APIKey == "" {
			return p, NewMissingCredentialsError("OPENAI_API_KEY not set")
		}
		p.BaseURL = os.Getenv("OPENAI_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = "https://api.openai.com/v1"
		}

	case ProviderGemini:
		p.APIKey = os.Getenv("GEMINI_API_KEY")
		if p.APIKey == "" {
			p.APIKey = os.Getenv("GOOGLE_API_KEY")
		}
		if p.APIKey == "" {
			return p, NewMissingCredentialsError("GEMINI_API_KEY or GOOGLE_API_KEY not set")
		}
		p.BaseURL = os.Getenv("GEMINI_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		}

	case ProviderOllama:
		p.APIKey = "ollama" // dummy; Ollama doesn't require auth
		p.BaseURL = os.Getenv("OLLAMA_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = os.Getenv("OLLAMA_HOST")
			if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http") {
				p.BaseURL = "http://" + p.BaseURL
			}
		}
		if p.BaseURL == "" {
			p.BaseURL = "http://localhost:11434/v1"
		} else if !strings.HasSuffix(p.BaseURL, "/v1") {
			p.BaseURL = strings.TrimRight(p.BaseURL, "/") + "/v1"
		}

	case ProviderAnthropic:
		p.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		if p.APIKey == "" {
			p.APIKey = os.Getenv("ANTHROPIC_AUTH_TOKEN")
		}
		if p.APIKey == "" {
			p.APIKey = os.Getenv("GLAW_API_KEY")
		}
		if p.APIKey == "" {
			return p, NewMissingCredentialsError("ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN, or GLAW_API_KEY not set")
		}
		p.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = "https://api.anthropic.com"
		}

		case ProviderOpenRouter:
		p.APIKey = os.Getenv("OPENROUTER_API_KEY")
		if p.APIKey == "" {
			return p, NewMissingCredentialsError("OPENROUTER_API_KEY not set")
		}
		p.BaseURL = os.Getenv("OPENROUTER_BASE_URL")
		if p.BaseURL == "" {
			p.BaseURL = "https://openrouter.ai/api/v1"
		}

	}

	return p, nil
}

// NewProviderClient creates the appropriate client based on model name.
func NewProviderClient(model string) (ProviderClient, error) {
	info := DetectProvider(model)
	resolved, err := info.Resolve(model)
	if err != nil {
		return nil, err
	}

	// Strip "ollama:" and "openrouter:" prefix from model name for the actual API call
	actualModel := model
	if info.Type == ProviderOllama && strings.HasPrefix(strings.ToLower(model), "ollama:") {
		actualModel = model[7:]
	}
	if info.Type == ProviderOpenRouter && strings.HasPrefix(strings.ToLower(model), "openrouter:") {
		actualModel = model[11:]
	}

	switch info.Type {
	case ProviderAnthropic:
		return NewAnthropicClient(ClientConfig{
			APIKey:  resolved.APIKey,
			BaseURL: resolved.BaseURL,
		}), nil

	default:
		// OpenAI, Gemini, Ollama, xAI all use OpenAICompatClient
		return NewOpenAICompatClient(ClientConfig{
			APIKey:     resolved.APIKey,
			BaseURL:    resolved.BaseURL,
			ModelAlias: actualModel,
		}), nil
	}
}

func backoffDuration(attempt int) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
