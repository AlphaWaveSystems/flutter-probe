package probelink

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// screenshotServer answers probe.screenshot with the given result and records the params it was sent.
func screenshotServer(t *testing.T, result string, gotParams *map[string]any) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req Request
		_ = json.Unmarshal(body, &req)
		if gotParams != nil {
			m := map[string]any{}
			_ = json.Unmarshal(req.Params, &m)
			*gotParams = m
		}
		_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(result)})
	}))
	t.Cleanup(srv.Close)
	return &HTTPClient{baseURL: srv.URL + "/probe/rpc", token: "t", httpClient: srv.Client(), addr: srv.URL}
}

func TestScreenshotBytesAsksForTheImageInlineAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old) //nolint:errcheck

	var params map[string]any
	data := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	c := screenshotServer(t, `{"path":"/data/tmp/studio_1.png","data":"`+data+`"}`, &params)
	got, err := c.ScreenshotBytes(context.Background(), "studio")
	if err != nil || string(got) != "PNGDATA" {
		t.Fatalf("got %q, err %v", got, err)
	}
	if params["inline"] != true || params["name"] != "studio" {
		t.Errorf("params = %v; the request must carry inline:true so the agent deletes its copy", params)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("ScreenshotBytes must not write on the host, found %d entries", len(entries))
	}
}

func TestScreenshotBytesReportsAnAnswerWithoutData(t *testing.T) {
	c := screenshotServer(t, `{"path":"/data/tmp/x.png"}`, nil)
	if _, err := c.ScreenshotBytes(context.Background(), "studio"); !errors.Is(err, ErrNoInlineScreenshot) {
		t.Errorf("err = %v, want ErrNoInlineScreenshot", err)
	}
	c = screenshotServer(t, `{"path":"/x.png","data":"!!not base64!!"}`, nil)
	if _, err := c.ScreenshotBytes(context.Background(), "studio"); err == nil {
		t.Error("bad base64 must be an error")
	}
}

// Screenshot (the one `probe test` uses) still saves the file; only ScreenshotBytes avoids the disk.
func TestScreenshotStillSavesAFileForTestRuns(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old) //nolint:errcheck
	data := base64.StdEncoding.EncodeToString([]byte("PNGDATA"))
	c := screenshotServer(t, `{"path":"/data/tmp/shot.png","data":"`+data+`"}`, nil)
	p, err := c.Screenshot(context.Background(), "shot")
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "PNGDATA" || filepath.Base(p) != "shot.png" {
		t.Errorf("saved %q: %q, %v", p, b, err)
	}
}

type plainClient struct {
	ProbeClient
	path string
}

func (p plainClient) Screenshot(context.Context, string) (string, error) { return p.path, nil }

type providerClient struct {
	ProbeClient
}

func (providerClient) ScreenshotBytes(context.Context, string) ([]byte, error) { return []byte("inline"), nil }

func TestScreenshotBytesHelperPrefersTheInlineProvider(t *testing.T) {
	read := func(string) ([]byte, error) { t.Fatal("must not read a file"); return nil, nil }
	if got, err := ScreenshotBytes(context.Background(), providerClient{}, "x", read); err != nil || string(got) != "inline" {
		t.Errorf("provider path: %q, %v", got, err)
	}
	f := filepath.Join(t.TempDir(), "a.png")
	_ = os.WriteFile(f, []byte("from-file"), 0o644)
	got, err := ScreenshotBytes(context.Background(), plainClient{path: f}, "x", os.ReadFile)
	if err != nil || string(got) != "from-file" {
		t.Errorf("fallback path: %q, %v", got, err)
	}
}
