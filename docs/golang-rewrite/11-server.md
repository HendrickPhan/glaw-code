# HTTP Server (REST API + SSE)

## Package: `internal/server/`

---

## Overview

An optional HTTP server that exposes session management via a REST API. Built with axum in Rust; for Go, use `net/http` or a framework like `gin` or `chi`.

---

## Data Types

### Session Store
```go
type SessionStore struct {
    mu       sync.RWMutex
    Sessions map[string]*Session  // session_id -> Session
    NextID   atomic.Uint64
}
```

### Server Session
```go
type Session struct {
    ID          string
    CreatedAt   time.Time
    Conversation *RuntimeSession  // the actual conversation data
    Events      chan SessionEvent // broadcast channel (cap: 64)
}
```

### Session Events
```go
type SessionEvent struct {
    Type      string // "snapshot" | "message"
    SessionID string
    Data      interface{}
}
```

### API Request/Response Types

```go
type CreateSessionResponse struct {
    SessionID string `json:"session_id"`
}

type SessionSummary struct {
    ID           string    `json:"id"`
    CreatedAt    time.Time `json:"created_at"`
    MessageCount int       `json:"message_count"`
}

type ListSessionsResponse struct {
    Sessions []SessionSummary `json:"sessions"`
}

type SessionDetailsResponse struct {
    ID        string          `json:"id"`
    CreatedAt time.Time       `json:"created_at"`
    Session   *RuntimeSession `json:"session"`
}

type SendMessageRequest struct {
    Message string `json:"message"`
}

type ErrorResponse struct {
    Error string `json:"error"`
}
```

---

## Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | /sessions | createSession | Create new conversation session |
| GET | /sessions | listSessions | List all active sessions |
| GET | /sessions/{id} | getSession | Get session details |
| GET | /sessions/{id}/events | streamEvents | SSE stream of session events |
| POST | /sessions/{id}/message | sendMessage | Send message to session |

---

## Handler Logic

### POST /sessions — createSession

```
1. Allocate new session ID: "session-{N}" (atomic increment)
2. Create Session with:
   - ID: allocated ID
   - CreatedAt: now
   - Conversation: new empty RuntimeSession
   - Events: make broadcast channel (capacity 64)
3. Store in SessionStore
4. Return CreateSessionResponse{SessionID: id}
```

### GET /sessions — listSessions

```
1. Read lock SessionStore
2. FOR EACH session:
     Build SessionSummary with ID, CreatedAt, MessageCount
3. Return ListSessionsResponse{Sessions: summaries}
```

### GET /sessions/{id} — getSession

```
1. Look up session by ID in SessionStore
2. IF not found: return 404 ErrorResponse{"Session not found"}
3. Build SessionDetailsResponse with full session data
4. Return 200 with JSON body
```

### GET /sessions/{id}/events — streamEvents (SSE)

```
1. Look up session by ID
2. IF not found: return 404
3. Set headers:
   Content-Type: text/event-stream
   Cache-Control: no-cache
   Connection: keep-alive
4. Subscribe to session's event channel
5. Send initial "snapshot" event:
   event: snapshot
   data: {json of full session state}
6. LOOP:
   SELECT:
   CASE event := <-subscription:
     Write SSE event:
       event: {event.Type}
       data: {json of event.Data}

   CASE <-keepalive (every 15 seconds):
     Write SSE comment:
       : keepalive

   CASE <-request.Done():
     Break loop
7. On broadcast channel lag (client too slow):
   Log warning, continue (don't break)
```

**SSE Format:**
```
event: {event_name}\n
data: {json_payload}\n
\n
```

### POST /sessions/{id}/message — sendMessage

```
1. Parse SendMessageRequest from body
2. Look up session by ID
3. IF not found: return 404
4. Send message to session's ConversationRuntime:
   a. Append user message to conversation
   b. Run agentic turn (send to API, handle tool calls)
   c. Append assistant response to conversation
5. Broadcast SessionEvent to all subscribers:
   Type: "message"
   SessionID: session.ID
   Data: the new messages
6. Return 200 with response data
```

---

## SSE Keep-Alive

Every 15 seconds, send a comment line to prevent connection timeout:

```
: keepalive\n\n
```

This is a standard SSE comment (starts with `:`) that clients ignore.

---

## Error Handling

| Scenario | Status | Response |
|----------|--------|----------|
| Session not found | 404 | `{"error": "Session not found"}` |
| Invalid JSON body | 400 | `{"error": "Invalid request body"}` |
| API error during turn | 500 | `{"error": "{api_error_message}"}` |
| Method not allowed | 405 | `{"error": "Method not allowed"}` |

---

## Session ID Allocation

Thread-safe atomic counter:
```go
func (s *SessionStore) AllocateID() string {
    n := s.NextID.Add(1)
    return fmt.Sprintf("session-%d", n)
}
```

---

## Broadcast Channel Pattern

```go
type Broadcaster struct {
    ch   chan SessionEvent
    subs []chan SessionEvent
    mu   sync.Mutex
}

func (b *Broadcaster) Subscribe() chan SessionEvent {
    ch := make(chan SessionEvent, 64)
    b.mu.Lock()
    b.subs = append(b.subs, ch)
    b.mu.Unlock()
    return ch
}

func (b *Broadcaster) Send(event SessionEvent) {
    b.mu.Lock()
    defer b.mu.Unlock()
    for _, sub := range b.subs {
        select {
        case sub <- event:
        default:
            // Client too slow, drop event (log warning)
        }
    }
}
```
