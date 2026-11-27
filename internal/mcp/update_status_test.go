package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/updatecheck"
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// updateStatusAnswer runs the update_status tool over JSON-RPC and returns the JSON in its text result.
func updateStatusAnswer(t *testing.T) map[string]any {
	t.Helper()
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "update_status", "arguments": map[string]any{}},
	})
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected a result, got %+v", resp)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("update_status returned an error result: %+v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("update_status did not return JSON: %q", text)
	}
	return out
}

// sandboxUpdateCheck keeps the stored answer in a temporary folder and points the endpoint where a test needs it.
func sandboxUpdateCheck(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldDir, oldEndpoint := userprefs.Dir, updatecheck.Endpoint
	userprefs.Dir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userprefs.Dir, updatecheck.Endpoint = oldDir, oldEndpoint })
}

// TestUpdateStatusOfflineIsAResultNotAnError is the rule for the tool as for the CLI: with no connection it answers normally, with
// the latest version unknown, and it is never an MCP error.
func TestUpdateStatusOfflineIsAResultNotAnError(t *testing.T) {
	sandboxUpdateCheck(t)
	t.Setenv("PROBE_NO_UPDATE_CHECK", "")
	oldVersion := Version
	Version = "0.28.1"
	t.Cleanup(func() { Version = oldVersion })
	updatecheck.Endpoint = "http://127.0.0.1:1/releases/latest"
	out := updateStatusAnswer(t)
	if out["latest"] != "" || out["update_available"] != false {
		t.Errorf("offline answer = %+v, want an unknown latest version and no update", out)
	}
}

// TestUpdateStatusDevelopmentBuildMakesNoRequest: a development build has no release to compare with, so the tool asks nothing.
func TestUpdateStatusDevelopmentBuildMakesNoRequest(t *testing.T) {
	sandboxUpdateCheck(t)
	t.Setenv("PROBE_NO_UPDATE_CHECK", "")
	requested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requested = true }))
	defer srv.Close()
	updatecheck.Endpoint = srv.URL
	oldVersion := Version
	Version = "dev"
	t.Cleanup(func() { Version = oldVersion })
	out := updateStatusAnswer(t)
	if requested || out["source"] != "unknown" || out["update_available"] != false {
		t.Errorf("a development build: requested=%v answer=%+v", requested, out)
	}
}

// TestUpdateStatusFollowsTheOptOut checks that the environment opt-out makes no request at all.
func TestUpdateStatusFollowsTheOptOut(t *testing.T) {
	sandboxUpdateCheck(t)
	requested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requested = true }))
	defer srv.Close()
	updatecheck.Endpoint = srv.URL
	t.Setenv("PROBE_NO_UPDATE_CHECK", "1")
	out := updateStatusAnswer(t)
	if requested || out["source"] != "disabled" {
		t.Errorf("with the opt-out: requested=%v answer=%+v", requested, out)
	}
}
