package record

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestBundleLayoutAndRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	b, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b.Dir() != dir {
		t.Fatalf("Dir() = %q, want %q", b.Dir(), dir)
	}
	now := time.Date(2026, 10, 16, 12, 0, 0, 123, time.UTC)
	manifest := Manifest{SchemaVersion: SchemaVersion, RunID: "run", TestName: "test", File: "test.probe", FileSHA256: "abc", Attempt: 2, DeviceName: "phone", Platform: "ios", OSVersion: "18", AgentVersion: "1", CLIVersion: "2", StartedAt: now, EndedAt: now.Add(time.Second), Status: "passed", Capture: Capture{Screenshots: "all", Native: "on", Redaction: true}, Truncated: Truncation{Exchanges: 3, Bytes: 42}}
	timeline := Timeline{Steps: []Step{{Index: 1, Depth: 2, File: "test.probe", Line: 3, Description: "tap", Status: "failed", Error: "error", StartedAt: now, DurationMs: 21, ScreenshotBefore: "before.png", ScreenshotAfter: "after.png", Tree: "0001.txt", Interaction: &Interaction{Kind: "tap", Target: &Rect{Left: 1, Top: 2, Width: 3, Height: 4}, TapX: 5, TapY: 6}, Exchanges: []string{"0001"}, AppRestarted: true}}}
	exchange := Exchange{ID: "0001", Seq: 1, Method: "POST", URL: "https://example.test", Status: 200, DurationMs: 17, Mocked: true, Error: "error", RequestHeaders: map[string]string{"Content-Type": "text/plain"}, ResponseHeaders: map[string]string{"X-Test": "yes"}, RequestBody: "request", ResponseBody: "response", Source: "native", Duplicate: true, TLSFailed: true}
	for _, tc := range []struct {
		name  string
		value any
		write func() error
	}{
		{"manifest.json", manifest, func() error { return b.WriteManifest(manifest) }},
		{"timeline.json", timeline, func() error { return b.WriteTimeline(timeline) }},
		{"http/0001.json", exchange, func() error { return b.WriteExchange(exchange) }},
	} {
		if err := tc.write(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, tc.name))
		if err != nil {
			t.Fatal(err)
		}
		got := reflect.New(reflect.TypeOf(tc.value))
		if err := json.Unmarshal(data, got.Interface()); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Elem().Interface(), tc.value) {
			t.Errorf("%s round-trip differs: %s", tc.name, data)
		}
		var indented bytes.Buffer
		if err := json.Indent(&indented, data, "", "  "); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, indented.Bytes()) {
			t.Errorf("%s not two-space indented", tc.name)
		}
	}
	if err := b.WriteTree("0001", "tree text"); err != nil {
		t.Fatal(err)
	}
	png := []byte{137, 80, 78, 71, 0, 255}
	if err := b.WriteScreenshot("step_1-after.png", png); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{"tree/0001.txt": []byte("tree text"), "screenshots/step_1-after.png": png} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	expected := map[string]bool{".": true, "http": true, "tree": true, "screenshots": true, "manifest.json": false, "timeline.json": false, "http/0001.json": false, "tree/0001.txt": false, "screenshots/step_1-after.png": false}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		isDir, ok := expected[filepath.ToSlash(rel)]
		if !ok {
			t.Errorf("unexpected file (possible leftover temporary): %s", rel)
			return nil
		}
		if info.IsDir() != isDir {
			t.Errorf("wrong file type for %s", rel)
		}
		if runtime.GOOS != "windows" {
			want := os.FileMode(0600)
			if isDir {
				want = 0700
			}
			if info.Mode().Perm() != want {
				t.Errorf("%s mode = %o, want %o", rel, info.Mode().Perm(), want)
			}
		}
		delete(expected, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != 0 {
		t.Errorf("missing paths: %v", expected)
	}
}

func TestBundleInvalidNames(t *testing.T) {
	b, err := Create(filepath.Join(t.TempDir(), "bundle"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "1", "123", "abcd", "../x", "0001/0002", "0001\\0002", "/0001", "0001\n", "１２３４"} {
		if err := b.WriteExchange(Exchange{ID: id}); err == nil {
			t.Errorf("exchange accepted %q", id)
		}
		if err := b.WriteTree(id, "text"); err == nil {
			t.Errorf("tree accepted %q", id)
		}
	}
	for _, name := range []string{"", "..", "../x", "a..png", "/x.png", "a/b.png", "a\\b.png", "a b.png", "a.png\n", "☃.png"} {
		if err := b.WriteScreenshot(name, nil); err == nil {
			t.Errorf("screenshot accepted %q", name)
		}
	}
	for _, sub := range []string{"http", "tree", "screenshots"} {
		entries, err := os.ReadDir(filepath.Join(b.Dir(), sub))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("invalid names created files in %s: %v", sub, entries)
		}
	}
	if err := b.WriteTree("12345", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteScreenshot("Az09._-.png", nil); err != nil {
		t.Fatal(err)
	}
}

func TestBundleReplacementAndFailureCleanup(t *testing.T) {
	b, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"old contents", "new"} {
		if err := b.WriteTree("0001", value); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(b.Dir(), "tree", "0001.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("replacement = %q", data)
	}
	// A directory at the destination forces rename to fail after writing the temp.
	if err := os.Mkdir(filepath.Join(b.Dir(), "tree", "0002.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteTree("0002", "failure"); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, err := os.ReadDir(filepath.Join(b.Dir(), "tree"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}

func TestBundleCreateFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(path); err == nil {
		t.Fatal("Create accepted a regular file")
	}
}
