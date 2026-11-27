package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/updatecheck"
	"github.com/alphawavesystems/flutter-probe/internal/userprefs"
)

func TestUpdateNoticeSkipRules(t *testing.T) {
	none := func(string) string { return "" }
	inCI := func(k string) string {
		if k == "CI" {
			return "true"
		}
		return ""
	}
	cases := []struct {
		name     string
		args     []string
		getenv   func(string) string
		terminal bool
		version  string
		skipped  bool
	}{
		{"a normal run in a terminal", []string{"test", "tests/"}, none, true, "0.28.1", false},
		{"a development build", []string{"test"}, none, true, "dev", true},
		{"CI", []string{"test"}, inCI, true, "0.28.1", true},
		{"stderr is a pipe", []string{"test"}, none, false, "0.28.1", true},
		{"json output", []string{"test", "--format", "json"}, none, true, "0.28.1", true},
		{"junit output", []string{"test", "--format=junit"}, none, true, "0.28.1", true},
		{"text output keeps the notice", []string{"test", "--format", "text"}, none, true, "0.28.1", false},
		{"the MCP stdio server", []string{"mcp-server"}, none, true, "0.28.1", true},
	}
	for _, c := range cases {
		reason := updateNoticeSkip(c.args, c.getenv, c.terminal, c.version)
		if (reason != "") != c.skipped {
			t.Errorf("%s: skip reason %q, want skipped=%v", c.name, reason, c.skipped)
		}
	}
}

// offlineUpdateCheck makes the update check reach a closed port, with a fresh preferences folder and a non-development version,
// which is what an offline machine looks like to the CLI.
func offlineUpdateCheck(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldDir, oldEndpoint, oldVersion := userprefs.Dir, updatecheck.Endpoint, Version
	userprefs.Dir = func() (string, error) { return dir, nil }
	updatecheck.Endpoint = "http://127.0.0.1:1/releases/latest"
	Version = "0.28.1"
	t.Cleanup(func() { userprefs.Dir, updatecheck.Endpoint, Version = oldDir, oldEndpoint, oldVersion })
}

// TestOfflineRunsAreSilent is the rule the owner asked for: with no connection, the check fails without any message, on a run and
// in the doctor report alike.
func TestOfflineRunsAreSilent(t *testing.T) {
	offlineUpdateCheck(t)
	var out bytes.Buffer
	check := updatecheck.Start(Version, func(string) string { return "" }, time.Now())
	printUpdateNotice(&out, check)
	if out.Len() != 0 {
		t.Errorf("an offline run printed %q, want nothing", out.String())
	}
	if _, ok := updateDoctorCheck(); ok {
		t.Error("an offline doctor report has an update line, want none")
	}
}

func TestNewerReleasePrintsOneLineOnStderr(t *testing.T) {
	offlineUpdateCheck(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`{"tag_name":"v0.29.0","html_url":"https://example.test/release","draft":false,"prerelease":false}`))
	}))
	defer srv.Close()
	updatecheck.Endpoint = srv.URL

	check := updatecheck.Start(Version, func(string) string { return "" }, time.Now())
	check.Status(2 * time.Second)
	var out bytes.Buffer
	printUpdateNotice(&out, check)
	want := "A new FlutterProbe version is available: 0.29.0 (you have 0.28.1). Download: https://example.test/release\n"
	if out.String() != want {
		t.Errorf("notice = %q, want %q", out.String(), want)
	}
}

func TestPrefsUpdateCheckSetAndShow(t *testing.T) {
	dir := t.TempDir()
	oldDir := userprefs.Dir
	userprefs.Dir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userprefs.Dir = oldDir })
	t.Setenv("PROBE_NO_UPDATE_CHECK", "")

	var out bytes.Buffer
	prefsSetCmd.SetOut(&out)
	if err := prefsSetCmd.RunE(prefsSetCmd, []string{"update-check", "off"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "update-check: off") || updatecheck.Enabled() {
		t.Fatalf("set off: output %q, enabled %v", out.String(), updatecheck.Enabled())
	}

	out.Reset()
	prefsShowCmd.SetOut(&out)
	if err := prefsShowCmd.RunE(prefsShowCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "update-check: off") {
		t.Errorf("show does not report the update check as off: %q", out.String())
	}

	if err := prefsSetCmd.RunE(prefsSetCmd, []string{"update-check", "on"}); err != nil {
		t.Fatal(err)
	}
	if !updatecheck.Enabled() {
		t.Error("set on did not turn the update check back on")
	}
}
