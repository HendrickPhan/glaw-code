package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hieu-glaw/glaw-code/internal/api"
	"github.com/hieu-glaw/glaw-code/internal/commands"
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
	sessions := s.store.ListSessions(s.workspaceRoot)
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
			sessions := s.store.ListSessions(s.workspaceRoot)
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
		// Auto-create session with runtime
		ws, err := s.store.CreateSessionWithRuntime(s.runtimeFactory)
		if err != nil {
			sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "failed to create session: " + err.Error()}})
			return
		}
		sessionID = ws.ID
		s.store.SetCurrentSession(sessionID)
		sendWS(conn, WSResponse{Type: "session_created", SessionID: sessionID, Data: map[string]string{"session_id": sessionID}})
		// Send updated session list so the sidebar reflects the new session
		sessions := s.store.ListSessions(s.workspaceRoot)
		sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
	} else {
		// Track which session is active
		s.store.SetCurrentSession(sessionID)
	}

	// Ensure the session has a runtime
	sess, err := s.store.EnsureRuntime(sessionID, s.runtimeFactory)
	if err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": err.Error()}})
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
		// Save session even in local mode
		if sess, ok := s.store.GetSession(sessionID); ok {
			s.saveSession(sess)
		}
		// Send updated session list so sidebar reflects the new session
		sessions := s.store.ListSessions(s.workspaceRoot)
		sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
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

	// Parse the command to extract name and args
	input := "/" + cmd.Command
	parsed, _ := commands.Parse(input)

	if parsed == nil {
		// Unknown command — try to suggest alternatives
		suggestions := commands.SuggestCommands(input)
		if len(suggestions) > 0 {
			names := make([]string, len(suggestions))
			for i, sg := range suggestions {
				names[i] = "/" + sg.Name
			}
			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": cmd.Command,
					"message": fmt.Sprintf("Unknown command. Did you mean: %s?", strings.Join(names, ", ")),
				},
			})
		} else {
			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": cmd.Command,
					"message": "Unknown command. Type /help for available commands.",
				},
			})
		}
		return
	}

	commandName := parsed.Spec.Name

	// Commands that have special WebSocket handling (side effects beyond just text)
	switch commandName {
	case "clear":
		if cmd.SessionID != "" {
			if sess, ok := s.store.GetSession(cmd.SessionID); ok {
				sess.Conversation.Messages = nil
				s.saveSession(sess)
			}
		}
		sendWS(conn, WSResponse{Type: "session_cleared", SessionID: cmd.SessionID})
		return

	case "session":
		s.handleWebSessionCommand(conn, cmd, parsed.Remainder)
		return

	case "quit", "exit":
		// Don't actually quit the web server, just inform the user
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": commandName,
				"message": "Close the browser tab to exit the web session.",
			},
		})
		return
	}

	// For all other commands, try to use the commands.Dispatcher if a runtime is available
	if cmd.SessionID != "" {
		sess, ok := s.store.GetSession(cmd.SessionID)
		if ok && sess.Dispatcher != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := sess.Dispatcher.Handle(ctx, input)
			if err != nil {
				sendWS(conn, WSResponse{
					Type:      "command_result",
					SessionID: cmd.SessionID,
					Data: map[string]interface{}{
						"command": commandName,
						"message": "Error: " + err.Error(),
					},
				})
				return
			}

			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": commandName,
					"message": result.Message,
				},
			})

			// Send updated session list after commands that may change it
			if commandName == "compact" || commandName == "clear" {
				sessions := s.store.ListSessions(s.workspaceRoot)
				sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
			}
			return
		}
	}

	// Fallback for sessions without a runtime — handle a few basic commands
	s.handleCommandFallback(conn, cmd, commandName, parsed.Remainder)
}

// handleCommandFallback handles commands when no runtime is available.
func (s *WebServer) handleCommandFallback(conn *websocket.Conn, cmd WSCommandMessage, commandName, remainder string) {
	switch commandName {
	case "help":
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "help",
				"message": buildHelpText(),
			},
		})

	case "version":
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "version",
				"message": "glaw-code v1.0.0",
			},
		})

	default:
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": commandName,
				"message": fmt.Sprintf("/%s requires an active agent runtime. Start a conversation first.", commandName),
			},
		})
	}
}

// handleWebSessionCommand handles /session sub-commands for the web UI.
// This has special WebSocket side effects (creating sessions, sending lists).
func (s *WebServer) handleWebSessionCommand(conn *websocket.Conn, cmd WSCommandMessage, remainder string) {
	parts := strings.Fields(remainder)
	subCmd := ""
	if len(parts) > 0 {
		subCmd = parts[0]
	}

	switch subCmd {
	case "list", "ls", "":
		// List sessions from filesystem to match CLI behavior
		sessionsDir := filepath.Join(s.workspaceRoot, ".glaw", "sessions")
		entries, err := os.ReadDir(sessionsDir)
		var lines []string

		if err != nil {
			// If sessions dir doesn't exist, show message
			lines = append(lines, "  (no sessions found)")
		} else {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
					info, err := e.Info()
					if err != nil {
						continue
					}
					name := strings.TrimSuffix(e.Name(), ".json")
					msgCount := ""
					// Try to read message count from session file
					data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name()))
					if err == nil {
						var sess struct {
							Messages []struct{} `json:"messages"`
						}
						if json.Unmarshal(data, &sess) == nil {
							msgCount = fmt.Sprintf("  %d msgs", len(sess.Messages))
						}
					}
					lines = append(lines, fmt.Sprintf("  %s  %s%s", name, info.ModTime().Format("2006-01-02 15:04"), msgCount))
				}
			}
		}

		msg := "Sessions:\n"
		if len(lines) == 0 {
			msg += "  (no saved sessions found. Sessions are saved automatically.)"
		} else {
			msg += strings.Join(lines, "\n")
		}
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "session",
				"message": msg,
			},
		})

	case "new":
		// Save the current active session before creating a new one (like CLI's NewSession does)
		if currentSess := s.store.GetCurrentSession(); currentSess != nil {
			s.saveSession(currentSess)
		}

		ws, err := s.store.CreateSessionWithRuntime(s.runtimeFactory)
		if err != nil {
			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": "session",
					"message": "Failed to create session: " + err.Error(),
				},
			})
			return
		}
		id := ws.ID
		sendWS(conn, WSResponse{Type: "session_created", Data: map[string]string{"session_id": id}})
		// Also send updated session list
		sessions := s.store.ListSessions(s.workspaceRoot)
		sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "session",
				"message": fmt.Sprintf("New session created: %s", id),
			},
		})

	case "delete", "rm":
		if len(parts) < 2 {
			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": "session",
					"message": "Usage: /session delete <session-id>",
				},
			})
			return
		}
		targetID := parts[1]
		if targetID == cmd.SessionID {
			sendWS(conn, WSResponse{
				Type:      "command_result",
				SessionID: cmd.SessionID,
				Data: map[string]interface{}{
					"command": "session",
					"message": "Cannot delete the current session. Use /session new first.",
				},
			})
			return
		}
		s.store.DeleteSession(targetID, s.workspaceRoot)
		sessions := s.store.ListSessions(s.workspaceRoot)
		sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "session",
				"message": fmt.Sprintf("Session %s deleted.", targetID),
			},
		})

	default:
		sendWS(conn, WSResponse{
			Type:      "command_result",
			SessionID: cmd.SessionID,
			Data: map[string]interface{}{
				"command": "session",
				"message": "Usage: /session [list|new|delete]\n  list   - Show all sessions\n  new    - Create a fresh session\n  delete - Delete a session",
			},
		})
	}
}

// buildHelpText generates the help text from the authoritative command specs.
func buildHelpText() string {
	var sb strings.Builder
	sb.WriteString("Available Commands:\n\n")

	categories := []commands.Category{
		commands.CategoryCore,
		commands.CategoryWorkspace,
		commands.CategorySession,
		commands.CategoryGit,
		commands.CategoryAutomation,
	}

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("  %s:\n", strings.ToUpper(string(cat[:1]))+string(cat[1:])))
		for _, spec := range commands.Specs {
			if spec.Category == cat {
				aliases := ""
				if len(spec.Aliases) > 0 {
					aliases = fmt.Sprintf(" (%s)", strings.Join(spec.Aliases, ", "))
				}
				hint := ""
				if spec.ArgumentHint != "" {
					hint = " " + spec.ArgumentHint
				}
				sb.WriteString(fmt.Sprintf("    /%s%s%s - %s\n", spec.Name, hint, aliases, spec.Summary))
			}
		}
	}

	return sb.String()
}

// handleSwitchSession sends the history for a selected session.
// It saves the current session to disk before switching (matching CLI behavior).
func (s *WebServer) handleSwitchSession(conn *websocket.Conn, data json.RawMessage) {
	var payload struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "invalid switch message"}})
		return
	}

	// Save the current active session before switching (like CLI's LoadSession does)
	if currentSess := s.store.GetCurrentSession(); currentSess != nil && currentSess.ID != payload.SessionID {
		s.saveSession(currentSess)
	}

	sess, err := s.store.EnsureRuntime(payload.SessionID, s.runtimeFactory)
	if err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": err.Error()}})
		return
	}

	// Track the active session
	s.store.SetCurrentSession(payload.SessionID)

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
	// Save current session before creating a new one
	if currentSess := s.store.GetCurrentSession(); currentSess != nil {
		s.saveSession(currentSess)
	}

	ws, err := s.store.CreateSessionWithRuntime(s.runtimeFactory)
	if err != nil {
		sendWS(conn, WSResponse{Type: "error", Data: map[string]string{"error": "failed to create session: " + err.Error()}})
		return
	}
	id := ws.ID
	s.store.SetCurrentSession(id)
	sendWS(conn, WSResponse{Type: "session_created", Data: map[string]string{"session_id": id}})

	// Also send updated session list
	sessions := s.store.ListSessions(s.workspaceRoot)
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

// saveSession persists a session to disk.
func (s *WebServer) saveSession(sess *WrappedSession) {
	if sess == nil || sess.Conversation == nil || s.workspaceRoot == "" {
		return
	}
	sessionsDir := filepath.Join(s.workspaceRoot, ".glaw", "sessions")
	if path, err := runtime.SaveSession(sess.Conversation, sessionsDir); err != nil {
		log.Printf("Warning: failed to save session %s: %v", sess.ID, err)
	} else {
		log.Printf("Session saved to %s", path)
	}
}

// runAgentTurn executes an agent turn and streams results over WebSocket.
func (s *WebServer) runAgentTurn(conn *websocket.Conn, sessionID string, sess *WrappedSession) {
	defer func() {
		// Persist session to disk after every agent turn (matches CLI behavior)
		s.saveSession(sess)

		// Send updated session list so sidebar reflects new message counts
		sessions := s.store.ListSessions(s.workspaceRoot)
		sendWS(conn, WSResponse{Type: "session_list", Data: sessions})
		sendWS(conn, WSResponse{Type: "done", SessionID: sessionID})
	}()

	// Use the session's existing runtime if available, otherwise create one
	var rt *runtime.ConversationRuntime
	var cleanup func()

	if sess.Runtime != nil {
		rt = sess.Runtime
		cleanup = func() {} // don't clean up the persistent runtime
	} else if s.runtimeFactory != nil {
		var err error
		rt, cleanup, err = s.runtimeFactory(sess.Conversation)
		if err != nil {
			sendWS(conn, WSResponse{Type: "error", SessionID: sessionID, Data: map[string]string{"error": err.Error()}})
			return
		}
		defer cleanup()
	} else {
		sendWS(conn, WSResponse{Type: "error", SessionID: sessionID, Data: map[string]string{"error": "no runtime available"}})
		return
	}

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
