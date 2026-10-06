package sysdialog

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner mimics the iOS runner's HTTP API.
type fakeRunner struct {
	mu       sync.Mutex
	srv      *httptest.Server
	dialog   *Dialog
	lastType map[string]any
	shutdown bool
}

func (f *fakeRunner) setDialog(d *Dialog) { f.mu.Lock(); f.dialog = d; f.mu.Unlock() }

func newFakeRunner(t *testing.T, d *Dialog) *fakeRunner {
	f := &fakeRunner{dialog: d}
	mux := http.NewServeMux()
	// Every handler runs under the lock: the test goroutine changes the dialog while requests are in flight.
	locked := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { f.mu.Lock(); defer f.mu.Unlock(); h(w, r) }
	}
	reply := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	body := func(r *http.Request) map[string]any {
		var m map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &m)
		return m
	}
	match := func(m map[string]any) bool {
		title, _ := m["title"].(string)
		return f.dialog != nil && f.dialog.MatchesTitle(title)
	}
	mux.HandleFunc("/health", locked(func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"ok": true, "driver": "flutter-probe-ios-driver", "version": "test"})
	}))
	mux.HandleFunc("/see", locked(func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"ok": true, "found": match(body(r))})
	}))
	mux.HandleFunc("/dialogs", locked(func(w http.ResponseWriter, r *http.Request) {
		var list []Dialog
		if f.dialog != nil {
			list = append(list, *f.dialog)
		}
		reply(w, map[string]any{"ok": true, "dialogs": list})
	}))
	mux.HandleFunc("/tap", locked(func(w http.ResponseWriter, r *http.Request) {
		m := body(r)
		if !match(m) {
			reply(w, map[string]any{"ok": false, "error": "no system dialog found"})
			return
		}
		i := Match(m["button"].(string), f.dialog.Buttons)
		if i < 0 {
			reply(w, map[string]any{"ok": false, "error": "no button in the dialog", "buttons": f.dialog.Buttons})
			return
		}
		reply(w, map[string]any{"ok": true, "tapped": f.dialog.Buttons[i]})
		f.dialog = nil
	}))
	mux.HandleFunc("/type", locked(func(w http.ResponseWriter, r *http.Request) {
		f.lastType = body(r)
		reply(w, map[string]any{"ok": false, "error": "field rejected " + f.lastType["text"].(string), "fields": []string{"Password"}})
	}))
	mux.HandleFunc("/dismiss", locked(func(w http.ResponseWriter, r *http.Request) {
		if f.dialog == nil {
			reply(w, map[string]any{"ok": true, "dismissed": false})
			return
		}
		f.dialog = nil
		reply(w, map[string]any{"ok": true, "dismissed": true})
	}))
	mux.HandleFunc("/shutdown", locked(func(w http.ResponseWriter, r *http.Request) { f.shutdown = true; reply(w, map[string]any{"ok": true}) }))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func portOf(t *testing.T, srv *httptest.Server) int {
	_, p, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	n, _ := strconv.Atoi(p)
	return n
}

func TestIOSDriver_ReusesARunningRunnerAndDrivesIt(t *testing.T) {
	f := newFakeRunner(t, &Dialog{Title: "“App” Would Like to Send You Notifications", Texts: []string{"“App” Would Like to Send You Notifications"}, Buttons: []string{"Don’t Allow", "Allow"}})
	d, err := NewIOS(context.Background(), "udid", IOSOptions{Port: portOf(t, f.srv)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if ok, _ := d.See(ctx, "Notifications"); !ok {
		t.Error("See should find the dialog")
	}
	if ok, _ := d.See(ctx, "Apple Account"); ok {
		t.Error("See must respect the title")
	}
	if _, err := d.Tap(ctx, "Nope", ""); err == nil || !strings.Contains(err.Error(), "Allow") {
		t.Errorf("error should list buttons: %v", err)
	}
	got, err := d.Tap(ctx, "Allow", "Notifications")
	if err != nil || got != "Allow" {
		t.Fatalf("tap: %q %v", got, err)
	}
	if ok, _ := d.See(ctx, ""); ok {
		t.Error("dialog should be gone after tapping")
	}
	if did, err := d.Dismiss(ctx, ""); did || err != nil {
		t.Errorf("dismiss with no dialog = %v %v", did, err)
	}
	if err := d.Close(); err != nil || f.shutdown {
		t.Errorf("Close must not stop a runner this process did not start (shutdown=%v err=%v)", f.shutdown, err)
	}
}

func TestIOSDriver_TypeNeverLeaksTheSecretInErrors(t *testing.T) {
	f := newFakeRunner(t, &Dialog{Texts: []string{"Sign in"}, Buttons: []string{"OK"}})
	d, err := NewIOS(context.Background(), "udid", IOSOptions{Port: portOf(t, f.srv)})
	if err != nil {
		t.Fatal(err)
	}
	err = d.Type(context.Background(), "Password", "hunter2", "")
	if err == nil {
		t.Fatal("expected the runner's error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("secret leaked into the error: %v", err)
	}
	if f.lastType["text"] != "hunter2" {
		t.Error("the runner must receive the real text")
	}
}

func TestIOSDriver_WaitPolls(t *testing.T) {
	f := newFakeRunner(t, nil)
	d, _ := NewIOS(context.Background(), "udid", IOSOptions{Port: portOf(t, f.srv)})
	go func() {
		time.Sleep(500 * time.Millisecond)
		f.setDialog(&Dialog{Texts: []string{"Sign in to Apple Account"}, Buttons: []string{"OK"}})
	}()
	ok, err := d.Wait(context.Background(), "Apple Account", true, 5*time.Second)
	if err != nil || !ok {
		t.Fatalf("should see the dialog appear: %v %v", ok, err)
	}
}

func TestPortForIsStableAndInRange(t *testing.T) {
	a, b := PortFor("AAAA-1"), PortFor("BBBB-2")
	if a != PortFor("AAAA-1") {
		t.Error("port must be deterministic")
	}
	for _, p := range []int{a, b} {
		if p < BasePort || p >= BasePort+200 {
			t.Errorf("port %d out of range", p)
		}
	}
}

func TestFindIOSDriverDir_HonoursEnvAndExplainsHowToInstall(t *testing.T) {
	t.Setenv("PROBE_HOME", t.TempDir())
	t.Setenv("PROBE_IOS_DRIVER_DIR", "")
	if _, err := FindIOSDriverDir("9.9.9"); err == nil || !strings.Contains(err.Error(), "probe ios-driver install") {
		t.Errorf("want an actionable error, got %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "X.xctestrun"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROBE_IOS_DRIVER_DIR", dir)
	got, err := FindIOSDriverDir("9.9.9")
	if err != nil || got != dir {
		t.Errorf("env override: %q %v", got, err)
	}
}

func makeZip(t *testing.T, entries map[string]string) string {
	p := filepath.Join(t.TempDir(), "d.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestUnzip_ExtractsAndRejectsZipSlip(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	if err := Unzip(makeZip(t, map[string]string{"P.xctestrun": "x", "Debug/a/b.txt": "y"}), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := findXCTestRun(dest); err != nil {
		t.Errorf("xctestrun not found after unzip: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "Debug/a/b.txt")); string(b) != "y" {
		t.Error("nested file missing")
	}
	evil := makeZip(t, map[string]string{"../../escape.txt": "boom"})
	if err := Unzip(evil, filepath.Join(t.TempDir(), "o2")); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Errorf("zip-slip must be rejected, got %v", err)
	}
}

func TestInstallIOSDriver_RefusesDevBuilds(t *testing.T) {
	if _, err := InstallIOSDriver(context.Background(), "dev"); err == nil {
		t.Error("a dev build has no release to download")
	}
}

func TestInstallIOSDriverFrom_DownloadsAndUnpacks(t *testing.T) {
	zipPath := makeZip(t, map[string]string{"P.xctestrun": "x"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, zipPath) }))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "v")
	got, err := InstallIOSDriverFrom(context.Background(), srv.URL+"/probe-ios-driver.zip", dest)
	if err != nil || got != dest {
		t.Fatalf("%q %v", got, err)
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	if _, err := InstallIOSDriverFrom(context.Background(), bad.URL+"/x.zip", filepath.Join(t.TempDir(), "w")); err == nil {
		t.Error("HTTP 404 must be an error")
	}
}
