package probelink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestDialRelay verifies that DialRelay connects to a WebSocket server,
// sends the correct role and token query params, and can exchange JSON-RPC
// messages through the connection.
func TestDialRelay(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	var receivedRole, receivedToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRole = r.URL.Query().Get("role")
		receivedToken = r.URL.Query().Get("token")

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()

		// Echo back a ping response
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(msg, &req); err != nil {
			return
		}

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"ok":true}`),
		}
		data, _ := json.Marshal(resp)
		conn.WriteMessage(websocket.TextMessage, data)
	}))
	defer srv.Close()

	// Convert http:// to the relay URL format
	relayURL := srv.URL + "/api/v1/relay/rly_test"

	ctx := context.Background()
	client, err := DialRelay(ctx, relayURL, "cli_secret_token", 5*time.Second)
	if err != nil {
		t.Fatalf("DialRelay: %v", err)
	}
	defer client.Close()

	// Verify query params
	if receivedRole != "cli" {
		t.Errorf("role = %q, want cli", receivedRole)
	}
	if receivedToken != "cli_secret_token" {
		t.Errorf("token = %q, want cli_secret_token", receivedToken)
	}

	// Verify we can call through the relay
	result, err := client.Call(ctx, MethodPing, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var pingResp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(result, &pingResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !pingResp.OK {
		t.Error("expected ok=true")
	}
}

// TestDialRelay_TokenMaskedInError verifies that the CLI token is not
// exposed in error messages when connection fails.
func TestDialRelay_TokenMaskedInError(t *testing.T) {
	// Connect to a non-existent server
	ctx := context.Background()
	_, err := DialRelay(ctx, "http://127.0.0.1:1/relay/test", "secret_token_value", 1*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "secret_token_value") {
		t.Errorf("error message contains token: %s", errMsg)
	}
	if !strings.Contains(errMsg, "token=***") {
		t.Errorf("error message should mask token: %s", errMsg)
	}
}

// TestDialRelay_InvalidURL verifies error handling for malformed URLs.
func TestDialRelay_InvalidURL(t *testing.T) {
	ctx := context.Background()
	_, err := DialRelay(ctx, "://invalid", "token", 1*time.Second)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
