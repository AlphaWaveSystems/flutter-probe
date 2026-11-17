package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The six endpoint methods that the other automation tests never call: list_devices, disconnect, results, ui_state, screenshot and widget_tree.
// Each is checked three ways against a fake backend: its result, a refusal without the token, and a refusal of a request from a web page.

var endpointMethods = []string{"list_devices", "disconnect", "results", "ui_state", "screenshot", "widget_tree"}

func TestEndpointMethodsReturnWhatTheBackendHolds(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.connected = "dev-1"
	fb.state = RunState{
		Running: true, Finished: false, Error: "boom",
		Current: &RunStep{Line: 3, Description: `tap "Go"`, Status: "started"},
		Results: []RunResult{{Name: "t one", Passed: true}, {Name: "t two", Passed: false}},
	}

	var devs []DeviceInfo
	_, out := rpc(t, s, s.token, "list_devices", nil, nil)
	if err := json.Unmarshal(out["result"], &devs); err != nil || len(devs) != 1 || devs[0].ID != "dev-1" || devs[0].Name != "Sim" {
		t.Errorf("list_devices = %s (%v)", out["result"], err)
	}

	var res struct {
		Running, Finished bool
		Error             string
		Results           []RunResult
	}
	_, out = rpc(t, s, s.token, "results", nil, nil)
	if err := json.Unmarshal(out["result"], &res); err != nil || !res.Running || res.Finished || res.Error != "boom" || len(res.Results) != 2 || res.Results[0].Name != "t one" || res.Results[1].Passed {
		t.Errorf("results = %s (%v)", out["result"], err)
	}

	var ui struct {
		Status ConnectionStatus
		Run    RunState
	}
	_, out = rpc(t, s, s.token, "ui_state", nil, nil)
	if err := json.Unmarshal(out["result"], &ui); err != nil || !ui.Status.Connected || ui.Status.DeviceID != "dev-1" || !ui.Run.Running || ui.Run.Current == nil || ui.Run.Current.Line != 3 || len(ui.Run.Results) != 2 {
		t.Errorf("ui_state = %s (%v)", out["result"], err)
	}

	var shot, tree string
	_, out = rpc(t, s, s.token, "screenshot", nil, nil)
	if err := json.Unmarshal(out["result"], &shot); err != nil || shot != "BASE64" {
		t.Errorf("screenshot = %s (%v)", out["result"], err)
	}
	_, out = rpc(t, s, s.token, "widget_tree", nil, nil)
	if err := json.Unmarshal(out["result"], &tree); err != nil || tree != "tree" {
		t.Errorf("widget_tree = %s (%v)", out["result"], err)
	}

	var ok map[string]bool
	_, out = rpc(t, s, s.token, "disconnect", nil, nil)
	if err := json.Unmarshal(out["result"], &ok); err != nil || !ok["ok"] {
		t.Errorf("disconnect = %s (%v)", out["result"], err)
	}
	if fb.connected != "" {
		t.Errorf("disconnect did not reach the backend: still connected to %q", fb.connected)
	}
}

func TestEndpointMethodsRefuseARequestWithoutTheTokenOrFromAWebPage(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.connected = "dev-1"
	for _, m := range endpointMethods {
		if code, _ := rpc(t, s, "", m, nil, nil); code != http.StatusUnauthorized {
			t.Errorf("%s without a token: %d, want 401", m, code)
		}
		if code, _ := rpc(t, s, "not-the-token", m, nil, nil); code != http.StatusUnauthorized {
			t.Errorf("%s with a wrong token: %d, want 401", m, code)
		}
		if code, _ := rpc(t, s, s.token, m, nil, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); code != http.StatusForbidden {
			t.Errorf("%s from a web page: %d, want 403", m, code)
		}
	}
	if fb.connected != "dev-1" {
		t.Error("a refused disconnect reached the backend")
	}
}

// With no connection, the methods that need a device say so, and the others answer with what is empty. This runs against the real App, not the fake.
func TestEndpointMethodsWithoutAConnection(t *testing.T) {
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", t.TempDir()+"/sub/automation.json")
	a := NewApp()
	s, err := startAutomation(a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	for _, m := range []string{"screenshot", "widget_tree"} {
		code, out := rpc(t, s, s.token, m, nil, nil)
		if code != http.StatusOK || out["error"] == nil || !strings.Contains(string(out["error"]), "not connected") || !strings.Contains(string(out["error"]), "pick a device") {
			t.Errorf("%s without a connection: %d %s, want a clear \"not connected\" error", m, code, out["error"])
		}
		if out["result"] != nil && string(out["result"]) != "null" {
			t.Errorf("%s without a connection returned a result: %s", m, out["result"])
		}
	}
	var ui struct {
		Status ConnectionStatus
		Run    RunState
	}
	_, out := rpc(t, s, s.token, "ui_state", nil, nil)
	if err := json.Unmarshal(out["result"], &ui); err != nil || ui.Status.Connected || ui.Run.Running || len(ui.Run.Steps) != 0 {
		t.Errorf("ui_state without a connection = %s (%v)", out["result"], err)
	}
	_, out = rpc(t, s, s.token, "results", nil, nil)
	var res struct {
		Running, Finished bool
		Results           []RunResult
	}
	if err := json.Unmarshal(out["result"], &res); err != nil || res.Running || res.Finished || len(res.Results) != 0 {
		t.Errorf("results without a run = %s (%v)", out["result"], err)
	}
	if _, out := rpc(t, s, s.token, "disconnect", nil, nil); out["error"] != nil {
		t.Errorf("disconnect with nothing connected must be a no-op, got %s", out["error"])
	}
}

// ---- the endpoint's guards, with the six methods as the thing being protected --------------------------------------------------------------------------

// rpcRaw posts a body as it is, with the headers the caller sets, and returns the status and the body text.
func rpcRaw(t *testing.T, s *automationServer, body string, header map[string]string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, s.URL()+"/rpc", strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

// The token is compared whole: a prefix of it, the token with something after it, another letter case and a scheme other than Bearer are all refused, and
// no backend method is reached.
func TestEndpointTokenIsComparedWhole(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.connected = "dev-1"
	good := s.token
	for name, auth := range map[string]string{
		"a prefix of the token":   "Bearer " + good[:len(good)-1],
		"half of the token":       "Bearer " + good[:len(good)/2],
		"one character":           "Bearer " + good[:1],
		"the token and one more":  "Bearer " + good + "x",
		"the token twice":         "Bearer " + good + good,
		"the token in other case": "Bearer " + strings.ToUpper(good),
		"no token after Bearer":   "Bearer ",
		"another scheme":          "Basic " + good,
		"a lower-case scheme":     "bearer " + good,
		"nothing":                 "",
	} {
		hdr := map[string]string{}
		if auth != "" {
			hdr["Authorization"] = auth
		}
		code, body := rpcRaw(t, s, `{"method":"disconnect"}`, hdr)
		if code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", name, code, body)
		}
	}
	if fb.connected != "dev-1" {
		t.Error("a refused request reached the backend")
	}
	if code, _ := rpcRaw(t, s, `{"method":"status"}`, map[string]string{"Authorization": "Bearer " + good}); code != http.StatusOK {
		t.Errorf("the whole token: %d", code)
	}
}

// Any Origin header is a web page: https, http, another port of this machine, and the opaque origin "null" (a sandboxed frame, a file page) are all refused.
func TestEndpointRefusesAnyOriginHeader(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.connected = "dev-1"
	for _, origin := range []string{"https://evil.example", "http://evil.example", "http://127.0.0.1:1", "http://localhost:3000", "null", "file://", "*"} {
		code, body := rpcRaw(t, s, `{"method":"disconnect"}`, map[string]string{"Authorization": "Bearer " + s.token, "Origin": origin})
		if code != http.StatusForbidden {
			t.Errorf("Origin %q: %d %s, want 403", origin, code, body)
		}
	}
	if fb.connected != "dev-1" {
		t.Error("a request with an Origin reached the backend")
	}
}

// The body is read up to 1 MiB: a larger one is cut and then is not the JSON it was meant to be, so it is refused (400) and nothing is reached.
func TestEndpointBodyIsLimitedToOneMebibyte(t *testing.T) {
	s, fb, _ := startTest(t)
	fb.connected = "dev-1"
	auth := map[string]string{"Authorization": "Bearer " + s.token}
	// a valid request whose last bytes are beyond the limit: padding inside a JSON string, so only the cut makes it invalid
	pad := strings.Repeat("x", (1<<20)+1000)
	code, body := rpcRaw(t, s, `{"method":"disconnect","params":{"pad":"`+pad+`"}}`, auth)
	if code != http.StatusBadRequest {
		t.Errorf("a body over 1 MiB: %d %s, want 400", code, body)
	}
	if fb.connected != "dev-1" {
		t.Error("a body over the limit reached the backend")
	}
	// a body under the limit with the same shape is served
	small := strings.Repeat("x", 100000)
	if code, body := rpcRaw(t, s, `{"method":"disconnect","params":{"pad":"`+small+`"}}`, auth); code != http.StatusOK || !strings.Contains(body, "ok") {
		t.Errorf("a body under the limit: %d %s", code, body)
	}
	if fb.connected != "" {
		t.Error("the request under the limit did not reach the backend")
	}
}
