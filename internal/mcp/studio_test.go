package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeStudio answers the automation protocol and records what it was asked.
func fakeStudio(t *testing.T, results map[string]any) (calls *[]map[string]any) {
	t.Helper()
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"missing or wrong token"}`))
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		got = append(got, req)
		res, ok := results[req["method"].(string)]
		if !ok {
			_, _ = w.Write([]byte(`{"error":"unknown method"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": res})
	}))
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "automation.json")
	doc, _ := json.Marshal(map[string]any{"url": srv.URL, "token": "tok123", "pid": 1})
	if err := os.WriteFile(file, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	return &got
}

func callTool(t *testing.T, name string, args map[string]any) map[string]any {
	t.Helper()
	resp := roundTrip(t, map[string]any{
		"jsonrpc": "2.0", "id": 9, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	return resp["result"].(map[string]any)
}

func textOf(res map[string]any) string {
	c := res["content"].([]any)[0].(map[string]any)
	return c["text"].(string)
}

func TestStudioToolsCallTheAutomationEndpoint(t *testing.T) {
	calls := fakeStudio(t, map[string]any{
		"open_workspace": []any{map[string]any{"name": "a.probe"}},
		"connect":        map[string]any{"connected": true, "deviceId": "dev-1"},
		"run_file":       map[string]any{"started": true},
		"run_state":      map[string]any{"running": true, "current": map[string]any{"line": 4}},
		"screenshot":     "QUJD",
	})
	if txt := textOf(callTool(t, "studio_open_workspace", map[string]any{"path": "/w"})); !strings.Contains(txt, "a.probe") {
		t.Errorf("open_workspace: %s", txt)
	}
	callTool(t, "studio_connect", map[string]any{"device_id": "dev-1"})
	callTool(t, "studio_run_file", map[string]any{"path": "/w/a.probe"})
	if txt := textOf(callTool(t, "studio_run_state", nil)); !strings.Contains(txt, `"line":4`) {
		t.Errorf("run_state: %s", txt)
	}
	shot := callTool(t, "studio_screenshot", nil)
	img := shot["content"].([]any)[1].(map[string]any)
	if img["type"] != "image" || img["data"] != "QUJD" {
		t.Errorf("screenshot: %+v", img)
	}
	want := []string{"open_workspace", "connect", "run_file", "run_state", "screenshot"}
	for i, m := range want {
		if (*calls)[i]["method"] != m {
			t.Errorf("call %d = %v, want %s", i, (*calls)[i]["method"], m)
		}
	}
	if p := (*calls)[1]["params"].(map[string]any); p["deviceId"] != "dev-1" {
		t.Errorf("connect params: %v", p)
	}
}

func TestStudioToolsExplainWhenStudioIsNotRunning(t *testing.T) {
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", filepath.Join(t.TempDir(), "missing.json"))
	res := callTool(t, "studio_run_state", nil)
	if res["isError"] != true || !strings.Contains(textOf(res), "PROBE_STUDIO_AUTOMATION=1") {
		t.Errorf("must tell how to enable it: %v", res)
	}
}

func TestStudioToolsSurfaceErrorsFromStudio(t *testing.T) {
	fakeStudio(t, map[string]any{}) // every method unknown
	res := callTool(t, "studio_cancel", nil)
	if res["isError"] != true || !strings.Contains(textOf(res), "unknown method") {
		t.Errorf("studio errors must come through: %v", res)
	}
}

func TestStudioTokenIsNeverSentOffThisMachine(t *testing.T) {
	file := filepath.Join(t.TempDir(), "automation.json")
	for _, url := range []string{"http://evil.example:8080", "http://192.168.1.5:9000", "https://127.0.0.1:1", "file:///etc/passwd"} {
		doc, _ := json.Marshal(map[string]any{"url": url, "token": "secret", "pid": 1})
		if err := os.WriteFile(file, doc, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
		res := callTool(t, "studio_run_state", nil)
		if res["isError"] != true || !strings.Contains(textOf(res), "loopback") && !strings.Contains(textOf(res), "unusable") {
			t.Errorf("%s must be refused before any request: %v", url, res)
		}
	}
	for _, ok := range []string{"http://127.0.0.1:1234", "http://localhost:1234", "http://[::1]:1234"} {
		if err := requireLoopbackURL(ok); err != nil {
			t.Errorf("%s is loopback: %v", ok, err)
		}
	}
}
