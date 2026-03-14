// Package server — WebSocket handler for real-time chat streaming.
// Uses nhooyr.io/websocket (stdlib-friendly, modern WebSocket library).
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"

	"github.com/Gsirawan/ifs-kiseki/internal/chat"
	"github.com/Gsirawan/ifs-kiseki/internal/crisis"
)

// ── Wire types ──────────────────────────────────────────────────

// wsIncoming is a JSON message from the browser.
type wsIncoming struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// wsOutgoing is a JSON message sent to the browser.
type wsOutgoing struct {
	Type      string        `json:"type"`
	Content   string        `json:"content,omitempty"`
	Message   string        `json:"message,omitempty"`
	SessionID string        `json:"session_id,omitempty"`
	Resources string        `json:"resources,omitempty"`
	Usage     *wsUsage      `json:"usage,omitempty"`
	Messages  []wsMessageRO `json:"messages,omitempty"`
}

// wsMessageRO is a read-only message for session_loaded responses.
type wsMessageRO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	db     *sql.DB
	crisis *crisis.RegexCrisisDetector // may be nil — crisis detection optional
}

// NewWebSocketHandler creates a WebSocket handler backed by the chat engine.
// Pass a non-nil crisis detector to enable crisis keyword scanning.
// db is required for switch_session (loading past session messages).
func NewWebSocketHandler(engine *chat.Engine, db *sql.DB, crisisDetector *crisis.RegexCrisisDetector) *WebSocketHandler {
	return &WebSocketHandler{
		engine: engine,
		db:     db,
		crisis: crisisDetector,
	}
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

	// Save the session to memory on disconnect. This is async inside EndSession —
	// it fires a goroutine and returns immediately so we don't block shutdown.
	if err := h.engine.EndSession(); err != nil {
		log.Printf("[ws] EndSession on disconnect: %v", err)
	}
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
	case "switch_session":
		h.handleSwitchSession(ctx, conn, msg.SessionID)
	default:
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "unknown message type: " + msg.Type,
		})
	}
}

// handleMessage sends a user message to the chat engine and streams
// response tokens back over the WebSocket.
//
// Crisis check: if a crisis detector is configured, the message is scanned
// BEFORE being sent to the LLM. If crisis content is detected:
//   - A {"type":"crisis","resources":"..."} message is sent to the client.
//   - The message is NOT forwarded to the LLM.
//   - The message content is NOT logged (privacy).
func (h *WebSocketHandler) handleMessage(ctx context.Context, conn *websocket.Conn, content string) {
	if content == "" {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "empty message content",
		})
		return
	}

	// ── Crisis check (BEFORE LLM call) ──────────────────────────
	if h.crisis != nil {
		if detected, category := h.crisis.Scan(content); detected {
			// Log the detection category — NOT the message content (privacy).
			log.Printf("[crisis] crisis content detected (category: %s) — resources sent, message not forwarded to LLM", category)

			_ = h.sendJSON(ctx, conn, wsOutgoing{
				Type:      "crisis",
				Resources: h.crisis.Resources(),
			})
			return // Do NOT send to LLM.
		}
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

// handleSwitchSession loads a past session from the database, sets it as the
// active session on the engine (so new messages go to it), and sends the
// session's message history back to the browser.
func (h *WebSocketHandler) handleSwitchSession(ctx context.Context, conn *websocket.Conn, sessionID string) {
	if sessionID == "" {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "switch_session requires a session_id",
		})
		return
	}

	if h.db == nil {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "database not available",
		})
		return
	}

	// Verify the session exists and fetch its timestamps.
	var startedAt int64
	var endedAt sql.NullInt64
	err := h.db.QueryRowContext(ctx,
		`SELECT started_at, ended_at FROM sessions WHERE id = ?`, sessionID,
	).Scan(&startedAt, &endedAt)
	if err == sql.ErrNoRows {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "session not found: " + sessionID,
		})
		return
	}
	if err != nil {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "failed to query session",
		})
		return
	}

	// Fetch the session's messages from the database.
	rows, err := h.db.QueryContext(ctx,
		`SELECT role, content FROM messages WHERE session_id = ? ORDER BY timestamp`, sessionID,
	)
	if err != nil {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "failed to query session messages",
		})
		return
	}
	defer rows.Close()

	var wireMessages []wsMessageRO
	var chatMessages []chat.LoadedMessage
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			_ = h.sendJSON(ctx, conn, wsOutgoing{
				Type:    "error",
				Message: "failed to read session messages",
			})
			return
		}
		wireMessages = append(wireMessages, wsMessageRO{Role: role, Content: content})
		chatMessages = append(chatMessages, chat.LoadedMessage{Role: role, Content: content})
	}
	if err := rows.Err(); err != nil {
		_ = h.sendJSON(ctx, conn, wsOutgoing{
			Type:    "error",
			Message: "failed to iterate session messages",
		})
		return
	}

	// Load the session into the engine as the active session.
	var endedAtPtr *int64
	if endedAt.Valid {
		endedAtPtr = &endedAt.Int64
	}
	h.engine.LoadSession(sessionID, startedAt, endedAtPtr, chatMessages)

	// Send the session history to the browser.
	_ = h.sendJSON(ctx, conn, wsOutgoing{
		Type:      "session_loaded",
		SessionID: sessionID,
		Messages:  wireMessages,
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
