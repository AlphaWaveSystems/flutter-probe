package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateRelaySession(t *testing.T) {
	want := RelaySession{
		SessionID:  "rly_test123",
		AgentToken: "agt_abc",
		CLIToken:   "cli_xyz",
		RelayURL:   "wss://example.com/api/v1/relay/rly_test123",
		ExpiresAt:  time.Date(2026, 3, 17, 12, 10, 0, 0, time.UTC),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/relay/sessions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong auth header: %s", r.Header.Get("Authorization"))
		}

		var req createRelayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Provider != "browserstack" {
			t.Errorf("expected provider browserstack, got %s", req.Provider)
		}
		if req.TTLSeconds != 600 {
			t.Errorf("expected TTL 600, got %d", req.TTLSeconds)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	got, err := c.CreateRelaySession(context.Background(), "browserstack", "Pixel 7", 600)
	if err != nil {
		t.Fatalf("CreateRelaySession: %v", err)
	}
	if got.SessionID != want.SessionID {
		t.Errorf("session_id = %q, want %q", got.SessionID, want.SessionID)
	}
	if got.AgentToken != want.AgentToken {
		t.Errorf("agent_token = %q, want %q", got.AgentToken, want.AgentToken)
	}
	if got.CLIToken != want.CLIToken {
		t.Errorf("cli_token = %q, want %q", got.CLIToken, want.CLIToken)
	}
	if got.RelayURL != want.RelayURL {
		t.Errorf("relay_url = %q, want %q", got.RelayURL, want.RelayURL)
	}
}

func TestPollRelayStatus_AgentConnected(t *testing.T) {
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		status := "created"
		if n >= 2 {
			status = "agent_connected"
		}
		json.NewEncoder(w).Encode(RelayStatus{
			SessionID: "rly_test",
			Status:    status,
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	got, err := c.PollRelayStatus(context.Background(), "rly_test", 10*time.Second)
	if err != nil {
		t.Fatalf("PollRelayStatus: %v", err)
	}
	if got.Status != "agent_connected" {
		t.Errorf("status = %q, want agent_connected", got.Status)
	}
}

func TestPollRelayStatus_Expired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RelayStatus{
			SessionID: "rly_test",
			Status:    "expired",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	_, err := c.PollRelayStatus(context.Background(), "rly_test", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for expired session")
	}
}

func TestDeleteRelaySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/relay/sessions/rly_del" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	if err := c.DeleteRelaySession(context.Background(), "rly_del"); err != nil {
		t.Fatalf("DeleteRelaySession: %v", err)
	}
}

func TestCreateRelaySession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-token")
	_, err := c.CreateRelaySession(context.Background(), "browserstack", "Pixel 7", 600)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}
