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

func startHeadlessTest(t *testing.T) (*headlessServer, *App) {
	t.Helper()
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
	resp, _ = http.Get(s.URL() + "/__headless/bridge.js")
	b, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(b), s.token) || strings.Contains(string(b), "__TOKEN__") {
		t.Fatal("the bridge must carry this launch's token")
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
}

func TestHeadlessCallsAppMethodsLikeWails(t *testing.T) {
	s, _ := startHeadlessTest(t)
	dir := t.TempDir()
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
	resp, err := http.Get(s.URL() + "/__headless/events?token=" + s.token)
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
	bad, _ := http.Get(s.URL() + "/__headless/events?token=nope")
	if bad.StatusCode != http.StatusUnauthorized {
		t.Errorf("events need the token: %d", bad.StatusCode)
	}
}
