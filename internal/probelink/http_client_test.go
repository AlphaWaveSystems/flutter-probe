package probelink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClient_Call(t *testing.T) {
	// Mock server that returns a JSON-RPC response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		body, _ := io.ReadAll(r.Body)
		var req Request
		json.Unmarshal(body, &req)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"ok":true}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &HTTPClient{
		baseURL:    srv.URL + "/probe/rpc",
		token:      "test-token",
		httpClient: srv.Client(),
		addr:       srv.URL,
	}

	result, err := c.Call(context.Background(), MethodPing, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var res struct {
		OK bool `json:"ok"`
	}
	json.Unmarshal(result, &res)
	if !res.OK {
		t.Error("expected ok=true")
	}
}

func TestHTTPClient_CallRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		json.Unmarshal(body, &req)

		resp := Response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32001, Message: "Widget not found"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := &HTTPClient{
		baseURL:    srv.URL + "/probe/rpc",
		token:      "t",
		httpClient: srv.Client(),
	}

	_, err := c.Call(context.Background(), MethodTap, nil)
	if err == nil {
		t.Fatal("expected RPC error")
	}
	if err.Error() != "rpc error -32001: Widget not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPClient_Connected(t *testing.T) {
	c := &HTTPClient{}
	if !c.Connected() {
		t.Error("new client should be connected")
	}
	c.Close()
	if c.Connected() {
		t.Error("closed client should not be connected")
	}
}

func TestHTTPClient_CallWhenClosed(t *testing.T) {
	c := &HTTPClient{closed: true}
	_, err := c.Call(context.Background(), MethodPing, nil)
	if err == nil {
		t.Error("expected error when calling closed client")
	}
}

// Verify HTTPClient satisfies ProbeClient interface
var _ ProbeClient = (*HTTPClient)(nil)
var _ ProbeClient = (*Client)(nil)
