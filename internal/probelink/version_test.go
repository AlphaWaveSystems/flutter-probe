package probelink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVersionMismatchWarning(t *testing.T) {
	cases := []struct {
		name          string
		client, agent string
		wantEmpty     bool
	}{
		{"identical versions", "0.9.9", "0.9.9", true},
		{"differing patch", "0.9.9", "0.9.3", false},
		{"differing minor", "0.9.9", "0.8.0", false},
		{"empty client version", "", "0.9.9", true},
		{"empty agent version", "0.9.9", "", true},
		{"both empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := VersionMismatchWarning(tc.client, tc.agent)
			if tc.wantEmpty && got != "" {
				t.Errorf("VersionMismatchWarning(%q, %q) = %q, want empty", tc.client, tc.agent, got)
			}
			if !tc.wantEmpty && got == "" {
				t.Errorf("VersionMismatchWarning(%q, %q) = empty, want a warning", tc.client, tc.agent)
			}
		})
	}
}

func TestMajorVersionIncompatible(t *testing.T) {
	cases := []struct {
		name          string
		client, agent string
		want          bool
	}{
		{"same major, differing minor/patch", "0.9.9", "0.9.3", false},
		{"different major", "1.0.0", "2.0.0", true},
		{"different major, 0.x vs 1.x", "0.9.9", "1.0.0", true},
		{"client is dev build", "dev", "0.9.9", false},
		{"agent version unknown (empty)", "0.9.9", "", false},
		{"client version unknown (empty)", "", "0.9.9", false},
		{"v-prefixed versions", "v1.2.3", "v1.9.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MajorVersionIncompatible(tc.client, tc.agent); got != tc.want {
				t.Errorf("MajorVersionIncompatible(%q, %q) = %v, want %v", tc.client, tc.agent, got, tc.want)
			}
		})
	}
}

// pingServer starts an httptest server that decodes the incoming PingParams
// and replies with the given agentVersion baked into PingResult. If
// omitAgentVersion is true, it replies with the pre-handshake {"ok":true}
// shape instead, simulating an agent built before this field existed.
func pingServer(t *testing.T, omitAgentVersion bool, agentVersion string, gotClientVersion *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		_ = json.Unmarshal(body, &req)

		var params PingParams
		_ = json.Unmarshal(req.Params, &params)
		*gotClientVersion = params.ClientVersion

		var resultJSON json.RawMessage
		if omitAgentVersion {
			resultJSON = json.RawMessage(`{"ok":true}`)
		} else {
			res := PingResult{OK: true, AgentVersion: agentVersion}
			resultJSON, _ = json.Marshal(res)
		}

		resp := Response{JSONRPC: "2.0", ID: req.ID, Result: resultJSON}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestHTTPClient_Handshake_SendsClientVersionAndParsesAgentVersion(t *testing.T) {
	var gotClientVersion string
	srv := pingServer(t, false, "0.9.9", &gotClientVersion)
	defer srv.Close()

	c := &HTTPClient{baseURL: srv.URL + "/probe/rpc", token: "t", httpClient: srv.Client()}

	res, err := c.Handshake(context.Background(), "0.9.3")
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}
	if gotClientVersion != "0.9.3" {
		t.Errorf("agent received client_version = %q, want 0.9.3", gotClientVersion)
	}
	if res.AgentVersion != "0.9.9" {
		t.Errorf("AgentVersion = %q, want 0.9.9", res.AgentVersion)
	}
}

func TestHTTPClient_Handshake_ToleratesAgentPredatingVersionField(t *testing.T) {
	var gotClientVersion string
	srv := pingServer(t, true, "", &gotClientVersion)
	defer srv.Close()

	c := &HTTPClient{baseURL: srv.URL + "/probe/rpc", token: "t", httpClient: srv.Client()}

	res, err := c.Handshake(context.Background(), "0.9.9")
	if err != nil {
		t.Fatalf("Handshake failed against old-shape agent response: %v", err)
	}
	if res.AgentVersion != "" {
		t.Errorf("AgentVersion = %q, want empty (agent predates this field)", res.AgentVersion)
	}
}

func TestCheckHandshake(t *testing.T) {
	newClient := func(t *testing.T, agentVersion string, omit bool) ProbeClient {
		var discard string
		srv := pingServer(t, omit, agentVersion, &discard)
		t.Cleanup(srv.Close)
		return &HTTPClient{baseURL: srv.URL + "/probe/rpc", token: "t", httpClient: srv.Client()}
	}

	t.Run("matching versions: no warning, no error", func(t *testing.T) {
		c := newClient(t, "0.9.9", false)
		warning, err := CheckHandshake(context.Background(), c, "0.9.9")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if warning != "" {
			t.Errorf("expected no warning, got %q", warning)
		}
	})

	t.Run("differing minor: warning, no error", func(t *testing.T) {
		c := newClient(t, "0.9.3", false)
		warning, err := CheckHandshake(context.Background(), c, "0.9.9")
		if err != nil {
			t.Fatalf("unexpected error for a minor/patch drift: %v", err)
		}
		if warning == "" {
			t.Error("expected a warning for differing versions, got none")
		}
	})

	t.Run("differing major: hard error", func(t *testing.T) {
		c := newClient(t, "2.0.0", false)
		_, err := CheckHandshake(context.Background(), c, "1.0.0")
		if err == nil {
			t.Fatal("expected an error for a major-version mismatch, got none")
		}
	})

	t.Run("agent predates version field: no warning, no error", func(t *testing.T) {
		c := newClient(t, "", true)
		warning, err := CheckHandshake(context.Background(), c, "0.9.9")
		if err != nil {
			t.Fatalf("unexpected error against an agent with no version field: %v", err)
		}
		if warning != "" {
			t.Errorf("expected no warning when the agent's version is unknown, got %q", warning)
		}
	})
}
