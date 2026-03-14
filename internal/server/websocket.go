// Package server — WebSocket handler for real-time chat streaming.
// Uses nhooyr.io/websocket (stdlib-friendly, modern WebSocket library).
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"

	"github.com/Gsirawan/ifs-kiseki/internal/chat"
)

// ── Wire types ──────────────────────────────────────────────────

// wsIncoming is a JSON message from the browser.
type wsIncoming struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
}

// wsOutgoing is a JSON message sent to the browser.
type wsOutgoing struct {
	Type      string   `json:"type"`
	Content   string   `json:"content,omitempty"`
	Message   string   `json:"message,omitempty"`
	SessionID string   `json:"session_id,omitempty"`
	Usage     *wsUsage `json:"usage,omitempty"`
}

// wsUsage is the token usage payload inside a "done" message.
type wsUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

// ── Handler ─────────────────────────────────────────────────────

// WebSocketHandler manages WebSocket connections for chat streaming.
type WebSocketHandler struct {
	engine *chat.Engine
}

// NewWebSocketHandler creates a WebSocket handler backed by the chat engine.
func NewWebSocketHandler(engine *chat.Engine) *WebSocketHandler {
	return &WebSocketHandler{engine: engine}
}

// HandleWebSocket upgrades the HTTP connection to WebSocket and runs the
// read/write loop. Each connection gets its own goroutine.
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Accept the WebSocket connection — allow localhost origins.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow any origin (localhost dev)
	})
	if err != nil {
		log.Printf("[ws] accept failed: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")

	log.Printf("[ws] client connected from %s", r.RemoteAddr)

	// Check that engine is available.
	if h.engine == nil {
		_ = h.sendJSON(r.Context(), conn, wsOutgoing{
			Type:    "error",
			Message: "no LLM provider configured — set an API key in settings",
		})
		return
	}

	// Ensure a session exists.
	if h.engine.GetSession() == nil {
		sessionID := h.engine.NewSession()
		_ = h.sendJSON(r.Context(), conn, wsOutgoing{
			Type:      "session_created",
			SessionID: sessionID,
		})
	}

	// Read loop — blocks until connection closes or error.
	h.readLoop(r.Context(), conn)

	log.Printf("[ws] client disconnected from %s", r.RemoteAddr)
}

// readLoop reads JSON messages from the WebSocket and dispatches them.
func (h *WebSocketHandler) readLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			// Normal closure or context cancelled — not an error.
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway ||
				ctx.Err() != nil {
				return
			}
			log.Printf("[ws] read error: %v", err)
			return
		}

		var msg wsIncoming
		if err := json.Unmarshal(data, &msg); err != nil {
			_ = h.sendJSON(ctx, conn, wsOutgoing{
				Type:    "error",
				Message: "invalid JSON message",
			})
			continue
		}

		h.dispatch(ctx, conn, msg)
	}
}

// dispatch routes an incoming message to the appropriate handler.
func (h *WebSocketHandler) dispatch(ctx context.Context, conn *websocket.Conn, msg wsIncoming) {
	switch msg.Type {
	case "message":
		h.handleMessage(ctx, conn, msg.Content)
	case "new_session":
		h.handleNewSession(ctx, conn)
	default:
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "unknown message type: " + msg.Type,
		})
	}
}

// handleMessage sends a user message to the chat engine and streams
// response tokens back over the WebSocket.
func (h *WebSocketHandler) handleMessage(ctx context.Context, conn *websocket.Conn, content string) {
	if content == "" {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "empty message content",
		})
		return
	}

	// Create a child context with timeout for the LLM call.
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	ch, err := h.engine.SendMessage(streamCtx, content)
	if err != nil {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: err.Error(),
		})
		return
	}

	// Relay each StreamEvent as a JSON message to the browser.
	for event := range ch {
		var out wsOutgoing

		switch event.Type {
		case "delta":
			out = wsOutgoing{
				Type:    "delta",
				Content: event.Delta,
			}
		case "done":
			out = wsOutgoing{Type: "done"}
			if event.Usage != nil {
				out.Usage = &wsUsage{
					Input:  event.Usage.InputTokens,
					Output: event.Usage.OutputTokens,
				}
			}
		case "error":
			errMsg := "unknown error"
			if event.Error != nil {
				errMsg = event.Error.Error()
			}
			out = wsOutgoing{
				Type:    "error",
				Message: errMsg,
			}
		default:
			continue
		}

		if err := h.sendJSON(ctx, conn, out); err != nil {
			log.Printf("[ws] write error during stream: %v", err)
			return
		}
	}
}

// handleNewSession creates a new chat session and notifies the client.
func (h *WebSocketHandler) handleNewSession(ctx context.Context, conn *websocket.Conn) {
	sessionID := h.engine.NewSession()
	_ = h.sendJSON(ctx, conn, wsOutgoing{
		Type:      "session_created",
		SessionID: sessionID,
	})
}

// sendJSON marshals and writes a JSON message to the WebSocket.
func (h *WebSocketHandler) sendJSON(ctx context.Context, conn *websocket.Conn, msg wsOutgoing) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return conn.Write(writeCtx, websocket.MessageText, data)
}
