package updatecheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

// sandbox points the preferences folder at a temporary directory and the endpoint at a test server, and restores both afterwards.
func sandbox(t *testing.T, handler http.HandlerFunc) (dir string, hits *atomic.Int32) {
	t.Helper()
	dir = t.TempDir()
	oldDir, oldEndpoint, oldBudget := userprefs.Dir, Endpoint, Budget
	userprefs.Dir = func() (string, error) { return dir, nil }
	hits = new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	Endpoint = srv.URL
	Budget = 2 * time.Second
	t.Cleanup(func() {
		srv.Close()
		userprefs.Dir, Endpoint, Budget = oldDir, oldEndpoint, oldBudget
	})
	return dir, hits
}

// unreachable points the endpoint at a closed port, which is what an offline machine sees.
func unreachable(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldDir, oldEndpoint, oldBudget := userprefs.Dir, Endpoint, Budget
	userprefs.Dir = func() (string, error) { return dir, nil }
	Endpoint = "http://127.0.0.1:1/releases/latest"
	Budget = 2 * time.Second
	t.Cleanup(func() { userprefs.Dir, Endpoint, Budget = oldDir, oldEndpoint, oldBudget })
}

func noEnv(string) string { return "" }

func latestRelease(w http.ResponseWriter, tag string) {
	w.Header().Set("ETag", `"v1"`)
	_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "html_url": "https://example.test/release", "draft": false, "prerelease": false})
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"0.29.0", "0.28.1", 1, true},
		{"0.28.1", "0.29.0", -1, true},
		{"0.28.1", "0.28.1", 0, true},
		{"0.28.1", "0.28.1-next.1", 1, true}, // a stable release is newer than its canary
		{"0.28.1-next.2", "0.28.1-next.1", 1, true},
		{"0.28.1-next.10", "0.28.1-next.9", 1, true}, // numeric identifiers compare by value
		{"0.28.1-alpha", "0.28.1-1", 1, true},        // text identifiers are greater than numeric ones
		{"v0.29.0", "0.28.1", 1, true},
		{"1.0.0", "0.99.99", 1, true},
		{"dev", "0.28.1", 0, false},
		{"0.28", "0.28.1", 0, false},
		{"0.28.1-", "0.28.1", 0, false},
		{"0.28.+1", "0.28.1", 0, false},
	}
	for _, c := range cases {
		got, ok := Compare(c.a, c.b)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", c.a, c.b, got, ok, c.want, c.ok)
		}
	}
}

func TestNoticeText(t *testing.T) {
	if got := Notice("0.28.1", "0.29.0", "", "probe"); !strings.HasSuffix(got, "Update: brew upgrade probe") {
		t.Errorf("stable brew notice = %q", got)
	}
	if got := Notice("0.28.1-next.1", "0.28.1", "", "probe@next"); !strings.HasSuffix(got, "Update: brew uninstall probe@next && brew install AlphaWaveSystems/tap/probe") {
		t.Errorf("canary brew notice = %q", got)
	}
	if got := Notice("0.28.1", "0.29.0", "https://example.test/r", ""); !strings.HasSuffix(got, "Download: https://example.test/r") {
		t.Errorf("manual notice = %q", got)
	}
	if got := Notice("0.28.1", "0.29.0", "", ""); !strings.HasSuffix(got, "Download: "+ReleasesURL) {
		t.Errorf("manual notice without a link = %q", got)
	}
}

func TestHomebrewFormula(t *testing.T) {
	cases := map[string]string{
		"/opt/homebrew/Cellar/probe/0.28.1/bin/probe":              "probe",
		"/opt/homebrew/Cellar/probe@next/0.28.1-next.1/bin/probe":  "probe@next",
		"/home/linuxbrew/.linuxbrew/Cellar/probe/0.28.1/bin/probe": "probe",
		"/usr/local/bin/probe":                                     "",
		"/Users/someone/dev/flutter-probe/bin/probe":               "",
	}
	for exe, want := range cases {
		if got := HomebrewFormula(exe); got != want {
			t.Errorf("HomebrewFormula(%q) = %q, want %q", exe, got, want)
		}
	}
}

func TestStartAsksOnceADayAndReusesTheStoredAnswer(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })

	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if st.Source != "network" || st.Latest != "0.29.0" || !st.UpdateAvailable {
		t.Fatalf("first run = %+v, want a network answer with 0.29.0 available", st)
	}
	if hits.Load() != 1 {
		t.Fatalf("first run made %d requests, want 1", hits.Load())
	}

	again := Start("0.28.1", noEnv, time.Now()).Status(0)
	if hits.Load() != 1 {
		t.Errorf("a run within a day made a request; hits = %d", hits.Load())
	}
	if again.Source != "cache" || !again.UpdateAvailable {
		t.Errorf("a run within a day = %+v, want the stored answer", again)
	}
}

func TestStaleAnswerSendsTheETagAndAccepts304(t *testing.T) {
	dir, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		latestRelease(w, "v0.29.0")
	})
	old := cache{ETag: `"v1"`, Latest: "0.29.0", URL: "https://example.test/release", CheckedAt: time.Now().Add(-48 * time.Hour).Unix()}
	if err := writeCache(old); err != nil {
		t.Fatal(err)
	}

	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if hits.Load() != 1 {
		t.Fatalf("a stale run made %d requests, want 1", hits.Load())
	}
	if st.Source != "network" || st.Latest != "0.29.0" || !st.UpdateAvailable {
		t.Errorf("after 304 = %+v, want the stored release kept", st)
	}
	if got := readCache(); got.CheckedAt <= old.CheckedAt {
		t.Errorf("a 304 did not refresh the check time (%d)", got.CheckedAt)
	}
	_ = dir
}

func TestOfflineIsSilentAndUnknown(t *testing.T) {
	unreachable(t)
	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if st.UpdateAvailable || st.Latest != "" || st.Source != "unknown" {
		t.Errorf("offline run = %+v, want an unknown answer with no update", st)
	}
}

func TestServerErrorIsSilent(t *testing.T) {
	sandbox(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) })
	st := Start("0.28.1", noEnv, time.Now()).Status(Budget)
	if st.UpdateAvailable || st.Latest != "" {
		t.Errorf("a 500 = %+v, want no answer", st)
	}
}

func TestDraftAndPrereleaseAreIgnored(t *testing.T) {
	sandbox(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.9.9", "html_url": "https://example.test/x", "draft": false, "prerelease": true})
	})
	if st := Start("0.28.1", noEnv, time.Now()).Status(Budget); st.Latest != "" || st.UpdateAvailable {
		t.Errorf("a prerelease answer = %+v, want it ignored", st)
	}
}

func TestEnvironmentSwitchesTheCheckOff(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	off := func(k string) string {
		if k == "PROBE_NO_UPDATE_CHECK" {
			return "1"
		}
		return ""
	}
	st := Start("0.28.1", off, time.Now()).Status(Budget)
	if hits.Load() != 0 || st.Source != "disabled" || st.UpdateAvailable {
		t.Errorf("with PROBE_NO_UPDATE_CHECK=1: hits=%d status=%+v, want no request and disabled", hits.Load(), st)
	}
}

func TestStoredChoiceSwitchesTheCheckOff(t *testing.T) {
	_, hits := sandbox(t, func(w http.ResponseWriter, r *http.Request) { latestRelease(w, "v0.29.0") })
	if err := SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("Enabled() = true after SetEnabled(false)")
	}
	if st := Start("0.28.1", noEnv, time.Now()).Status(Budget); hits.Load() != 0 || st.Source != "disabled" {
		t.Errorf("with the stored choice off: hits=%d status=%+v", hits.Load(), st)
	}
	if err := SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Error("Enabled() = false after SetEnabled(true)")
	}
}

func TestWriteCacheRefusesALink(t *testing.T) {
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	target := filepath.Join(dir, "elsewhere.json")
	if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, cacheName)); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if err := writeCache(cache{Latest: "9.9.9"}); err == nil {
		t.Fatal("writeCache wrote through a link")
	}
	if data, _ := os.ReadFile(target); string(data) != "untouched" {
		t.Errorf("the link target was changed to %q", data)
	}
}

func TestReadCacheIgnoresAFileOthersCanRead(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("file modes are not checked on Windows")
	}
	dir, _ := sandbox(t, func(w http.ResponseWriter, r *http.Request) {})
	path := filepath.Join(dir, cacheName)
	if err := os.WriteFile(path, []byte(`{"latest":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readCache(); got.Latest != "" {
		t.Errorf("a group-readable cache was trusted: %+v", got)
	}
}
