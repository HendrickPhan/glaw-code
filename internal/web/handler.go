package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/runtime"
)

// WSMessage is a message sent/received over WebSocket.
type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// WSChatMessage carries a chat message payload.
type WSChatMessage struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

// WSCommandMessage carries a slash-command payload.
type WSCommandMessage struct {
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
}

// WSResponse is a structured response sent to the client.
type WSResponse struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and handles messages.
func (s *WebServer) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("WebSocket client connected: %s", conn.RemoteAddr())

	// Send session list on connect
	sessions := s.store.ListSessions()
	sendWS(conn, WSResponse{Type: "session_list", Data: sessions})

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid message format"}})
			continue
		}

		switch msg.Type {
		case "chat":
			s.handleChat(conn, msg.Data)
		case "command":
			s.handleCommand(conn, msg.Data)
		case "list_sessions":
			sessions := s.store.ListSessions()
			sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
		case "switch_session":
			s.handleSwitchSession(conn, msg.Data)
		case "create_session":
			s.handleCreateSession(conn)
		case "get_history":
			s.handleGetHistory(conn, msg.Data)
		default:
			sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "unknown message type: " + msg.Type}})
		}
	}

	log.Printf("WebSocket client disconnected: %s", conn.RemoteAddr())
}

// handleChat processes a chat message from the WebSocket client.
func (s *WebServer) handleChat(conn *websocket.Conn, data json.RawMessage) {
	var chat WSChatMessage
	if err := json.Unmarshal(data, &chat); err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid chat message"}})
		return
	}

	sessionID := chat.SessionID
	if sessionID == "" {
		// Auto-create session
		sessionID = s.store.CreateSession()
		sendWS(conn, WSResponse{Type: "session_created", Data: map[string]string{"session_id": sessionID}})
	}

	sess, ok := s.store.GetSession(sessionID)
	if !ok {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "session not found"}})
		return
	}

	// Add user message to session
	sess.Conversation.AddUserMessageFromText(chat.Content)

	// Echo user message back
	sendWS(conn, WSResponse{
		Type:      "user_message",
		SessionID: sessionID,
		Data: map[string]interface{}{
			"role":    "user",
			"content": chat.Content,
		},
	})

	// Try to run agent turn if runtime is available
	if s.runtimeFactory != nil {
		go s.runAgentTurn(conn, sessionID, sess)
	} else {
		sendWS(conn, WSResponse{
			Type:      "assistant_message",
			SessionID: sessionID,
			Data: map[string]interface{}{
				"role":    "assistant",
				"content": "No agent runtime configured. Running in local mode.",
			},
		})
		sendWS(conn, WSResponse{Type: "done", SessionID: sessionID})
	}
}

// handleCommand processes a slash-command from the WebSocket client.
func (s *WebServer) handleCommand(conn *websocket.Conn, data json.RawMessage) {
	var cmd WSCommandMessage
	if err := json.Unmarshal(data, &cmd); err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid command message"}})
		return
	}

	switch cmd.Command {
	case "clear":
		sendWS(conn, WSResponse{Type: "session_cleared", SessionID: cmd.SessionID})
	case "help":
		sendWS(conn, WSResponse{Type: "help", Data: map[string]interface{}{
			"commands": []string{"/clear", "/help", "/sessions", "/new"},
		}})
	default:
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "unknown command: " + cmd.Command}})
	}
}

// handleSwitchSession sends the history for a selected session.
func (s *WebServer) handleSwitchSession(conn *websocket.Conn, data json.RawMessage) {
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid switch message"}})
		return
	}

	sess, ok := s.store.GetSession(payload.SessionID)
	if !ok {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "session not found"}})
		return
	}

	// Convert session messages to frontend-friendly format
	messages := convertSessionMessages(sess.Conversation)

	sendWS(conn, WSResponse{
		Type:      "session_switched",
		SessionID: payload.SessionID,
		Data: map[string]interface{}{
			"session_id": payload.SessionID,
			"messages":   messages,
		},
	})
}

// handleCreateSession creates a new session and notifies the client.
func (s *WebServer) handleCreateSession(conn *websocket.Conn) {
	id := s.store.CreateSession()
	sendWS(conn, WSResponse{Type: "session_created", Data: map[string]string{"session_id": id}})

	// Also send updated session list
	sessions := s.store.ListSessions()
	sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
}

// handleGetHistory sends the conversation history for a session.
func (s *WebServer) handleGetHistory(conn *websocket.Conn, data json.RawMessage) {
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid get_history message"}})
		return
	}

	sess, ok := s.store.GetSession(payload.SessionID)
	if !ok {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "session not found"}})
		return
	}

	// Convert session messages to frontend-friendly format
	messages := convertSessionMessages(sess.Conversation)

	sendWS(conn, WSResponse{
		Type:      "history",
		SessionID: payload.SessionID,
		Data: map[string]interface{}{
			"session_id": payload.SessionID,
			"messages":   messages,
		},
	})
}

// runAgentTurn executes an agent turn and streams results over WebSocket.
func (s *WebServer) runAgentTurn(conn *websocket.Conn, sessionID string, sess *WrappedSession) {
	defer func() {
		sendWS(conn, WSResponse{Type: "done", SessionID: sessionID})
	}()

	rt, cleanup, err := s.runtimeFactory(sess.Conversation)
	if err != nil {
		sendWS(conn, WSResponse{Type: "error", SessionID: sessionID, Data: map[string]string{"error": err.Error()}})
		return
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for {
		result, err := rt.Turn(ctx)
		if err != nil {
			sendWS(conn, WSResponse{Type: "error", SessionID: sessionID, Data: map[string]string{"error": err.Error()}})
			return
		}

		// Stream text content back
		for _, block := range result.Response.Content {
			if block.Type == "text" && block.Text != "" {
				sendWS(conn, WSResponse{
					Type:      "assistant_message",
					SessionID: sessionID,
					Data: map[string]interface{}{
						"role":    "assistant",
						"content": block.Text,
					},
				})
			}
			if block.Type == "tool_use" {
				sendWS(conn, WSResponse{
					Type:      "tool_use",
					SessionID: sessionID,
					Data: map[string]interface{}{
						"id":   block.ID,
						"name": block.Name,
						"input": string(block.Input),
					},
				})
			}
		}

		// Execute tool calls and continue
		if result.StopReason == "tool_use" {
			for _, tc := range result.ToolCalls {
				output, err := rt.ToolExecutor.ExecuteTool(ctx, tc.Name, tc.Input)
				if err != nil {
					sess.Conversation.AddToolResult(tc.ID, err.Error(), true)
					sendWS(conn, WSResponse{
						Type:      "tool_result",
						SessionID: sessionID,
						Data: map[string]interface{}{
							"id":      tc.ID,
							"content": err.Error(),
							"isError": true,
						},
					})
				} else {
					sess.Conversation.AddToolResult(tc.ID, output.Content, output.IsError)
					sendWS(conn, WSResponse{
						Type:      "tool_result",
						SessionID: sessionID,
						Data: map[string]interface{}{
							"id":      tc.ID,
							"content": output.Content,
							"isError": output.IsError,
						},
					})
				}
			}
			continue
		}

		// End of turn
		return
	}
}

// sendWS sends a JSON message over a WebSocket connection.
func sendWS(conn *websocket.Conn, msg WSResponse) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket marshal error: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("WebSocket write error: %v", err)
	}
}

// convertSessionMessages flattens runtime.Session messages into a format
// the frontend can render directly (role + content pairs).
func convertSessionMessages(session *runtime.Session) []map[string]interface{} {
	var result []map[string]interface{}

	for _, msg := range session.Messages {
		switch msg.Role {
		case "user":
			// Check if this is a tool_result user message
			hasToolResult := false
			for _, block := range msg.Blocks {
				if block.Type == api.ContentToolResult {
					hasToolResult = true
					result = append(result, map[string]interface{}{
						"role": "tool",
						"toolResult": map[string]interface{}{
							"id":      block.ToolUseID,
							"content": block.Content,
							"isError": block.IsError,
						},
					})
				}
			}
			// If no tool results, treat as regular user message
			if !hasToolResult {
				var texts []string
				for _, block := range msg.Blocks {
					if block.Type == api.ContentText {
						texts = append(texts, block.Text)
					}
				}
				if len(texts) > 0 {
					result = append(result, map[string]interface{}{
						"role":    "user",
						"content": joinTexts(texts),
					})
				}
			}

		case "assistant":
			for _, block := range msg.Blocks {
				switch block.Type {
				case api.ContentText:
					result = append(result, map[string]interface{}{
						"role":    "assistant",
						"content": block.Text,
					})
				case api.ContentToolUse:
					result = append(result, map[string]interface{}{
						"role":   "tool",
						"toolUse": map[string]interface{}{
							"id":    block.ID,
							"name":  block.Name,
							"input": string(block.Input),
						},
					})
				}
			}
		}
	}

	return result
}

// joinTexts concatenates text strings with newlines.
func joinTexts(texts []string) string {
	result := ""
	for i, t := range texts {
		if i > 0 {
			result += "\n"
		}
		result += t
	}
	return result
}
