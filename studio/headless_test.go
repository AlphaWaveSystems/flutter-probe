package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testHeadlessToken = "0123456789abcdef0123456789abcdef-test"

func startHeadlessTest(t *testing.T) (*headlessServer, *App) {
	t.Helper()
	t.Setenv(headlessTokenEnv, testHeadlessToken)
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head><title>x</title></head><body>studio</body></html>")}, "app.js": {Data: []byte("1")}}
	a := NewApp()
	s, err := startHeadless(a, assets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, a
}

func hcall(t *testing.T, s *headlessServer, token, method string, args ...any) (int, map[string]json.RawMessage) {
	t.Helper()
	raw := make([]json.RawMessage, len(args))
	for i, a := range args {
		raw[i], _ = json.Marshal(a)
	}
	body, _ := json.Marshal(map[string]any{"method": method, "args": raw})
	req, _ := http.NewRequest(http.MethodPost, s.URL()+"/__headless/call", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("X-Headless-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out map[string]json.RawMessage
	_ = json.Unmarshal(b, &out)
	return resp.StatusCode, out
}

func TestHeadlessServesIndexWithTheBridgeAndAssets(t *testing.T) {
	s, _ := startHeadlessTest(t)
	resp, _ := http.Get(s.URL() + "/")
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `/__headless/bridge.js`) || !strings.Contains(string(b), "studio") {
		t.Fatalf("index: %s", b)
	}
	resp, _ = http.Get(s.URL() + "/app.js")
	b, _ = io.ReadAll(resp.Body)
	if string(b) != "1" {
		t.Fatalf("asset: %q", b)
	}
	// The script and the page hold no secret: a process that fetches them learns nothing it can call with.
	for _, path := range []string{"/__headless/bridge.js", "/", "/index.html", "/app.js"} {
		resp, _ = http.Get(s.URL() + path)
		b, _ = io.ReadAll(resp.Body)
		if strings.Contains(string(b), s.token) || strings.Contains(string(b), "__TOKEN__") || strings.Contains(strings.ToLower(string(b)), "token = ") {
			t.Fatalf("%s carries the token or a place for it", path)
		}
	}
}

// Studio does not make a token: the launcher gives it in the environment, it must be long enough, and Studio does not pass it on.
func TestHeadlessNeedsALongTokenFromTheLauncherAndKeepsItToItself(t *testing.T) {
	assets := fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}
	for name, tok := range map[string]string{
		"none": "", "short": "abc", "one under": strings.Repeat("x", minHeadlessToken-1),
		"only spaces": strings.Repeat(" ", minHeadlessToken), "spaces around a short one": "  abc" + strings.Repeat(" ", minHeadlessToken),
		"a line break inside": strings.Repeat("x", 20) + "\n" + strings.Repeat("x", 20), "a tab": strings.Repeat("x", 40) + "\t",
		"not ASCII": strings.Repeat("é", 20), "a control character": strings.Repeat("x", 40) + "\x07",
	} {
		t.Setenv(headlessTokenEnv, tok)
		if s, err := startHeadless(NewApp(), assets); err == nil {
			s.Close()
			t.Errorf("%s: the bridge started with a token it should refuse", name)
		} else if !strings.Contains(err.Error(), headlessTokenEnv) {
			t.Errorf("%s: the refusal does not name %s: %v", name, headlessTokenEnv, err)
		}
	}
	t.Setenv(headlessTokenEnv, strings.Repeat("x", minHeadlessToken))
	s, err := startHeadless(NewApp(), assets)
	if err != nil {
		t.Fatalf("a token of exactly %d characters was refused: %v", minHeadlessToken, err)
	}
	defer s.Close()
	if os.Getenv(headlessTokenEnv) != "" {
		t.Error("the token is still in Studio's environment, so every program it starts inherits it")
	}
}

func TestHeadlessCallNeedsTheTokenAndALocalCaller(t *testing.T) {
	s, _ := startHeadlessTest(t)
	if code, _ := hcall(t, s, "", "Status"); code != http.StatusUnauthorized {
		t.Errorf("no token: %d", code)
	}
	if code, _ := hcall(t, s, "nope", "Status"); code != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", code)
	}
	if code, out := hcall(t, s, s.token, "Status"); code != http.StatusOK || out["result"] == nil {
		t.Errorf("valid call: %d %v", code, out)
	}
	req, _ := http.NewRequest(http.MethodGet, s.URL()+"/", nil)
	req.Header.Set("Origin", "http://evil.example")
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a page from another origin must be refused: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, s.URL()+"/", nil)
	req.Host = "evil.example"
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a foreign Host header must be refused: %d", resp.StatusCode)
	}
	for site, want := range map[string]int{"cross-site": http.StatusForbidden, "same-site": http.StatusForbidden, "same-origin": http.StatusOK, "none": http.StatusOK} {
		req, _ = http.NewRequest(http.MethodGet, s.URL()+"/", nil)
		req.Header.Set("Sec-Fetch-Site", site)
		if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != want {
			t.Errorf("Sec-Fetch-Site %q: want %d, got %v %v", site, want, resp, err)
		}
	}
}

func TestHeadlessCallsAppMethodsLikeWails(t *testing.T) {
	s, a := startHeadlessTest(t)
	dir := t.TempDir()
	a.SetWorkspace(dir) // the page opens the workspace before it lists or reads its files
	if err := os.WriteFile(filepath.Join(dir, "a.probe"), []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out := hcall(t, s, s.token, "ListDir", dir)
	var entries []FileEntry
	_ = json.Unmarshal(out["result"], &entries)
	if len(entries) != 1 || entries[0].Name != "a.probe" {
		t.Fatalf("ListDir: %v", out)
	}
	if _, out := hcall(t, s, s.token, "ReadFile", filepath.Join(dir, "a.probe")); out["result"] == nil {
		t.Errorf("ReadFile: %v", out)
	}
	if _, out := hcall(t, s, s.token, "ReadFile", filepath.Join(dir, "a.txt")); out["error"] == nil {
		t.Errorf("a method error must come back as an error: %v", out)
	}
	if _, out := hcall(t, s, s.token, "ListDir"); out["error"] == nil || !strings.Contains(string(out["error"]), "arguments") {
		t.Errorf("wrong argument count: %v", out)
	}
	if _, out := hcall(t, s, s.token, "NoSuchMethod"); out["error"] == nil {
		t.Errorf("unknown method: %v", out)
	}
	if _, out := hcall(t, s, s.token, "PickWorkspace"); out["error"] == nil {
		t.Errorf("PickWorkspace needs a window: %v", out)
	}
	if _, out := hcall(t, s, s.token, "SetWorkspace", dir); out["error"] != nil {
		t.Errorf("a void method must succeed: %v", out)
	}
}

func TestHeadlessEventsReachTheBrowser(t *testing.T) {
	s, a := startHeadlessTest(t)
	req, _ := http.NewRequest(http.MethodGet, s.URL()+"/__headless/events", nil)
	req.Header.Set("X-Headless-Token", s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events: %d", resp.StatusCode)
	}
	got := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				got <- strings.TrimPrefix(sc.Text(), "data: ")
				return
			}
		}
	}()
	time.Sleep(100 * time.Millisecond) // the subscription is made when the handler starts
	a.emit("run:started", "/x.probe")
	select {
	case line := <-got:
		var ev hubEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Name != "run:started" || len(ev.Data) != 1 || ev.Data[0] != "/x.probe" {
			t.Fatalf("event = %s", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no event arrived")
	}
	bad, _ := http.Get(s.URL() + "/__headless/events")
	if bad.StatusCode != http.StatusUnauthorized {
		t.Errorf("events need the token: %d", bad.StatusCode)
	}
	wrong, _ := http.NewRequest(http.MethodGet, s.URL()+"/__headless/events", nil)
	wrong.Header.Set("X-Headless-Token", "nope")
	if resp, _ := http.DefaultClient.Do(wrong); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("events with a wrong token: %d", resp.StatusCode)
	}
	// the right token in the address is not a token: an address is logged and kept in history
	if resp, _ := http.Get(s.URL() + "/__headless/events?token=" + s.token); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a token in the query string was accepted: %d", resp.StatusCode)
	}
}
