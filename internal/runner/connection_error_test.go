package runner

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// FP-13: the agent reports step timeouts as JSON-RPC -32000, and the client
// used to fail pending calls on a dropped connection with the same code. The
// error classifier matched "rpc error -32000", so every failed `wait` looked
// like a dead connection and burned the whole auto-reconnect budget (4
// backoff attempts, ~55 s) before surfacing a confusing error.
func TestIsConnectionError_AgentTimeoutIsNotAConnectionError(t *testing.T) {
	timeout := &probelink.RPCError{Code: -32000, Message: `Timed out waiting for "X" to appear`}
	if isConnectionError(timeout) {
		t.Errorf("agent step timeout must not be treated as a dropped connection: %v", timeout)
	}
	if isConnectionError(fmt.Errorf("wrapped: %w", timeout)) {
		t.Error("wrapping must not change the classification")
	}
}

func TestIsConnectionError_DroppedConnectionStillDetected(t *testing.T) {
	closed := &probelink.RPCError{Code: probelink.CodeConnectionClosed, Message: "connection closed: websocket: close 1006"}
	for name, err := range map[string]error{
		"pending call failed by readLoop": closed,
		"write on closed conn":            errors.New("probelink: write: write tcp: use of closed network connection"),
		"EOF":                             errors.New("EOF"),
		"broken pipe":                     errors.New("write: broken pipe"),
	} {
		if !isConnectionError(err) {
			t.Errorf("%s should be a connection error: %v", name, err)
		}
	}
}
