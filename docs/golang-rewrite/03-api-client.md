# API Client — Functions, Flows, and Logic

## Package: `internal/api/`

---

## Provider Architecture

The system supports multiple LLM providers through a unified interface:

### ProviderClient (sum type)
```go
type ProviderClient interface {
    SendMessage(ctx context.Context, req ApiRequest) (*ApiResponse, error)
    StreamMessage(ctx context.Context, req ApiRequest) (<-chan StreamEvent, error)
}
```

**Provider variants:**
- `ClawApiClient` — Anthropic API (`POST /v1/messages`)
- `XaiClient` — xAI/Grok API (`POST /chat/completions`, OpenAI-compatible)
- `OpenAICompatClient` — Generic OpenAI-compatible endpoint

### Provider Routing

**Function:** `NewProviderClient(model string) (ProviderClient, error)`

Routing logic:
```
IF model starts with "grok":
    require XAI_API_KEY env var
    read XAI_BASE_URL env var (default: "https://api.x.ai/v1")
    return XaiClient
ELSE:
    require ANTHROPIC_API_KEY env var (or proxy token)
    read ANTHROPIC_BASE_URL env var (default: "https://api.anthropic.com")
    return ClawApiClient
```

### Auth Sources
- `ApiKey` — explicit key passed at construction
- `EnvVar` — read from environment variable
- `ProxyToken` — from `~/.claw/credentials` OAuth token

---

## ClawApiClient — Anthropic API

### SendMessage (non-streaming)

```
POST {base_url}/v1/messages
Headers:
  x-api-key: {api_key}
  Authorization: Bearer {proxy_token} (if using OAuth)
  content-type: application/json
  anthropic-version: 2023-06-01
Body: ApiRequest JSON

Response: ApiResponse JSON
```

**Logic:**
1. Build HTTP request with proper headers
2. Send request
3. Parse JSON response into ApiResponse
4. Extract `request-id` from `x-request-id` response header
5. Return ApiResponse with RequestID populated

### StreamMessage (SSE streaming)

```
POST {base_url}/v1/messages
Headers: (same as above)
Body: ApiRequest JSON with stream: true

Response: SSE event stream
```

**SSE Event Types (in order):**
1. `message_start` — contains message ID, model, usage
2. `content_block_start` — contains block index, type (text or tool_use)
3. `content_block_delta` — contains text delta or input_json_delta
4. `content_block_stop` — marks end of content block
5. `message_delta` — contains stop_reason, usage delta
6. `message_stop` — marks end of message

**Parsing logic:**
1. Read SSE stream line by line
2. Parse `event:` and `data:` fields
3. For each event type, extract relevant fields from JSON data
4. Accumulate text deltas into content blocks
5. Accumulate tool_use input_json_delta fragments into complete tool input
6. Build final ApiResponse when message_stop received

### Retry Logic

```
Retry configuration: max_retries (default 2)
Retryable errors: HTTP 429 (rate limit), HTTP 503 (overloaded), HTTP 529 (overloaded)

For each retry:
  Wait with exponential backoff
  Resend the same request
  If all retries exhausted: return RetriesExhausted error with attempts count
```

---

## XaiClient / OpenAICompatClient

### SendMessage (non-streaming)

```
POST {base_url}/chat/completions
Headers:
  Authorization: Bearer {api_key}
  content-type: application/json
Body: OpenAI-compatible request JSON

Response: OpenAI-compatible response JSON
```

**Request mapping:**
- `model` → model string directly
- `messages` → converted to OpenAI format: system message first, then user/assistant
- `tools` → converted to function-calling format: `{"type":"function","function":{...}}`
- `tool_choice` → "auto" | "required" | "none" or specific function

**Response parsing:**
- Extract text from `choices[0].message.content`
- Extract tool calls from `choices[0].message.tool_calls[]`
- Map to unified ApiResponse format
- Extract usage from `usage` field

### StreamMessage (SSE streaming)

**SSE format:** Standard `data: {...}` lines terminated by `data: [DONE]`

**Event mapping:**
1. Text delta: `choices[0].delta.content` → Text ContentBlockDelta
2. Tool call delta: `choices[0].delta.tool_calls[i].function.arguments` → input_json_delta
3. Finish reason: `choices[0].finish_reason` → "tool_calls" maps to StopReason "tool_use"

**Multiple simultaneous tool calls:** OpenAI format uses indexed tool_calls array. The parser must track multiple tool_use blocks by index and emit separate ContentBlockStart/Delta events for each.

---

## SSE Parser

### IncrementalSseParser

Stateful parser that handles partial chunks:

```go
type IncrementalSseParser struct {
    buffer    strings.Builder
    eventName string
    dataLines []string
    id        string
    retry     *int
}
```

**Algorithm:**
```
PushChunk(chunk string) []SseEvent:
  Append chunk to buffer
  WHILE buffer contains "\n\n" (event delimiter):
    Extract lines before delimiter
    FOR each line:
      IF starts with ":": skip (comment)
      IF starts with "event: ": set event name
      IF starts with "data: ": append data line
      IF starts with "id: ": set id
      IF starts with "retry: ": parse retry int
      ELSE: treat as continuation (append to previous field)
    IF any data lines collected:
      Build SseEvent with event name, joined data, id, retry
      Reset parser state for next event
  Keep remaining incomplete data in buffer
```

---

## Error Types

```go
type ApiError struct {
    Type    ApiErrorType
    Message string
    Status  int     // HTTP status code
    Retries int     // for RetriesExhausted
}

type ApiErrorType int
// HttpError, JsonError, RateLimit, Authentication,
// InvalidRequest, ServerError, Timeout, SseError,
// RetriesExhausted, MissingCredentials
```
