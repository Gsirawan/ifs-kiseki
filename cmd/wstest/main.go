// Quick WebSocket test client — sends a message and prints streamed response.
// Usage: go run cmd/wstest/main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"nhooyr.io/websocket"
)

type wsMsg struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	Message   string `json:"message,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Usage     *struct {
		Input  int `json:"input"`
		Output int `json:"output"`
	} `json:"usage,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws://127.0.0.1:3737/ws", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.CloseNow()

	// Send a message
	msg := wsMsg{Type: "message", Content: "Hello! Just say hi back in one sentence."}
	data, _ := json.Marshal(msg)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Print("Response: ")

	// Read streamed response
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nread error: %v\n", err)
			break
		}

		var resp wsMsg
		if err := json.Unmarshal(raw, &resp); err != nil {
			fmt.Fprintf(os.Stderr, "\nparse error: %v\n", err)
			continue
		}

		switch resp.Type {
		case "delta":
			fmt.Print(resp.Content)
		case "done":
			fmt.Println()
			if resp.Usage != nil {
				fmt.Printf("Tokens — input: %d, output: %d\n", resp.Usage.Input, resp.Usage.Output)
			}
			conn.Close(websocket.StatusNormalClosure, "done")
			return
		case "error":
			fmt.Fprintf(os.Stderr, "\nError: %s\n", resp.Message)
			os.Exit(1)
		case "session_created":
			fmt.Fprintf(os.Stderr, "[session: %s]\n", resp.SessionID)
		}
	}
}
