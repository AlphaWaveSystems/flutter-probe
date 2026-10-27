package probelink

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A transport error must not print the agent token, which the request URL carries in its query.
func TestHTTPCallErrorDoesNotPrintToken(t *testing.T) {
	const token = "quorvex-agent-token-value"
	// Nothing listens on port 1, so the call fails at the dial.
	c := &HTTPClient{
		baseURL:    "http://127.0.0.1:1/probe/rpc",
		token:      token,
		httpClient: &http.Client{Timeout: 2 * time.Second},
		addr:       "http://127.0.0.1:1/probe/rpc",
	}
	_, err := c.Call(context.Background(), "ping", nil)
	if err == nil {
		t.Fatal("expected the call to fail")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("the transport error printed the agent token: %v", err)
	}
}
