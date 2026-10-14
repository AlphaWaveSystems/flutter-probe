package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// studioFake implements just what the `probe studio` handler calls; every other method panics through the nil
// embedded interface, which would show a call the handler is not supposed to make.
type studioFake struct {
	probelink.ProbeClient
	taps      []probelink.SelectorParam
	tapErr    error
	shot      []byte
	shotCalls int
}

func (f *studioFake) Tap(_ context.Context, sel probelink.SelectorParam) error {
	f.taps = append(f.taps, sel)
	return f.tapErr
}
func (f *studioFake) DumpWidgetTree(context.Context) (string, error) { return "Scaffold", nil }
func (f *studioFake) ScreenshotBytes(context.Context, string) ([]byte, error) {
	f.shotCalls++
	return f.shot, nil
}

const testOrigin = "http://127.0.0.1:9191"

func do(t *testing.T, h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	return m
}

func TestStudioExecIsHonestAboutNotRunningAnything(t *testing.T) {
	f := &studioFake{}
	h := newStudioHandler(context.Background(), f, testOrigin)
	rec := do(t, h, http.MethodPost, "/api/exec", `{"step":"tap \"Sign In\""}`, nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	m := decode(t, rec)
	if m["ok"] != false || !strings.Contains(m["error"].(string), "not implemented") {
		t.Errorf("body = %v; want ok false and a not-implemented message", m)
	}
	if len(f.taps) != 0 {
		t.Error("the step must not run anything")
	}
	if rec := do(t, h, http.MethodGet, "/api/exec", "", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", rec.Code)
	}
}

func TestStudioTapByTextWorksAndByCoordinateIsRefused(t *testing.T) {
	f := &studioFake{}
	h := newStudioHandler(context.Background(), f, testOrigin)

	rec := do(t, h, http.MethodPost, "/api/tap", `{"text":"Sign In"}`, nil)
	if rec.Code != http.StatusOK || decode(t, rec)["ok"] != true || len(f.taps) != 1 || f.taps[0].Text != "Sign In" || f.taps[0].Kind != "text" {
		t.Fatalf("tap by text: status %d, body %s, taps %v", rec.Code, rec.Body.String(), f.taps)
	}

	// A coordinate (what a click on the screenshot sends) is not implemented and must not tap anything.
	rec = do(t, h, http.MethodPost, "/api/tap", `{"x":0.4,"y":0.6,"text":""}`, nil)
	m := decode(t, rec)
	if rec.Code != http.StatusNotImplemented || m["ok"] != false || !strings.Contains(m["error"].(string), "coordinate") {
		t.Errorf("coordinate tap: status %d, body %v", rec.Code, m)
	}
	// Neither text nor a coordinate is a bad request.
	rec = do(t, h, http.MethodPost, "/api/tap", `{}`, nil)
	if rec.Code != http.StatusBadRequest || decode(t, rec)["ok"] != false {
		t.Errorf("empty tap: status %d, body %s", rec.Code, rec.Body.String())
	}
	if len(f.taps) != 1 {
		t.Errorf("only the tap by text may reach the agent, got %d taps", len(f.taps))
	}

	f.tapErr = errors.New("widget not found")
	rec = do(t, h, http.MethodPost, "/api/tap", `{"text":"Nope"}`, nil)
	m = decode(t, rec)
	if m["ok"] != false || m["error"] != "widget not found" {
		t.Errorf("an agent error must reach the page: %v", m)
	}
	if rec := do(t, h, http.MethodPost, "/api/tap", `not json`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad JSON status = %d, want 400", rec.Code)
	}
}

func TestStudioScreenshotWritesNothingToDisk(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old) //nolint:errcheck

	f := &studioFake{shot: []byte("\x89PNG fake")}
	h := newStudioHandler(context.Background(), f, testOrigin)
	for i := 0; i < 3; i++ {
		rec := do(t, h, http.MethodGet, "/api/screenshot", "", nil)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.String() != string(f.shot) {
			t.Fatalf("screenshot: status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	if f.shotCalls != 3 {
		t.Errorf("inline capture calls = %d, want 3", f.shotCalls)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the live view must not write files, found %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "reports")); err == nil {
		t.Error("reports/ must not be created by a screenshot request")
	}
}

func TestStudioRefusesOtherOrigins(t *testing.T) {
	h := newStudioHandler(context.Background(), &studioFake{}, testOrigin)
	if rec := do(t, h, http.MethodGet, "/api/tree", "", map[string]string{"Origin": "http://evil.example"}); rec.Code != http.StatusForbidden {
		t.Errorf("foreign origin status = %d, want 403", rec.Code)
	}
	rec := do(t, h, http.MethodGet, "/api/tree", "", map[string]string{"Origin": testOrigin})
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != testOrigin {
		t.Errorf("own origin: status %d, header %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec := do(t, h, http.MethodGet, "/api/tree", "", nil); rec.Code != http.StatusOK {
		t.Errorf("no origin status = %d, want 200", rec.Code)
	}
}
