package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refEvent writes an event payload to a file in a test directory and returns its path.
func refEvent(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// base_branch is sent only for pull_request and pull_request_target. default_branch comes from the event payload
// whenever it names one. A missing, unreadable or garbled payload only adds a warning, and the upload goes on.
func TestCIRefFieldsByEvent(t *testing.T) {
	const payload = `{"repository":{"default_branch":"main"}}`
	pathOf := func(body string) func(*testing.T) string {
		return func(t *testing.T) string { return refEvent(t, body) }
	}
	missing := func(t *testing.T) string { return filepath.Join(t.TempDir(), "no-such-event.json") }
	unset := func(t *testing.T) string { return "" }
	cases := []struct {
		name         string
		event        string
		baseRef      string // GITHUB_BASE_REF, set when non-empty
		path         func(*testing.T) string
		wantBase     string
		wantDefault  string
		wantWarnings int
	}{
		{"push: default branch only", "push", "", pathOf(payload), "", "main", 0},
		{"pull_request: base and default branch", "pull_request", "release/1.2", pathOf(payload), "release/1.2", "main", 0},
		{"pull_request_target: base and default branch", "pull_request_target", "release/1.2", pathOf(payload), "release/1.2", "main", 0},
		{"pull_request_review: base is not sent", "pull_request_review", "release/1.2", pathOf(payload), "", "main", 0},
		{"schedule: default branch only", "schedule", "release/1.2", pathOf(payload), "", "main", 0},
		{"workflow_dispatch: default branch only", "workflow_dispatch", "", pathOf(payload), "", "main", 0},
		{"event file missing", "pull_request", "release/1.2", missing, "release/1.2", "", 1},
		{"event file garbled", "pull_request", "release/1.2", pathOf("{not json"), "release/1.2", "", 1},
		{"event path not set", "pull_request", "release/1.2", unset, "release/1.2", "", 1},
		{"event file without a default branch", "push", "", pathOf(`{"repository":{}}`), "", "", 0},
		{"default branch with a control character", "push", "", pathOf(`{"repository":{"default_branch":"ma\u0007in"}}`), "", "", 1},
		{"default branch longer than 255 characters", "push", "", pathOf(`{"repository":{"default_branch":"` + strings.Repeat("b", 256) + `"}}`), "", "", 1},
		{"base ref with a tab is left out", "pull_request", "rel\tease", pathOf(payload), "", "main", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]string{
				"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "a/b", "GITHUB_REF_NAME": "main",
				"GITHUB_SHA": strings.Repeat("a", 40), "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1",
				"GITHUB_EVENT_NAME": c.event,
			}
			if c.baseRef != "" {
				env["GITHUB_BASE_REF"] = c.baseRef
			}
			if p := c.path(t); p != "" {
				env["GITHUB_EVENT_PATH"] = p
			}
			var warnings []string
			ci := CIFromEnv(envMap(env), func(m string) { warnings = append(warnings, m) })
			if ci == nil {
				t.Fatal("inside GitHub Actions there must be CI facts")
			}
			if ci.BaseBranch != c.wantBase || ci.DefaultBranch != c.wantDefault {
				t.Errorf("base_branch %q, default_branch %q; want %q, %q", ci.BaseBranch, ci.DefaultBranch, c.wantBase, c.wantDefault)
			}
			if len(warnings) != c.wantWarnings {
				t.Errorf("warnings = %v, want %d", warnings, c.wantWarnings)
			}
			// A field that is not sent is absent from the JSON, never an empty string.
			raw, err := json.Marshal(ci)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range []struct{ key, val string }{{"base_branch", c.wantBase}, {"default_branch", c.wantDefault}} {
				if has := strings.Contains(string(raw), `"`+f.key+`"`); has != (f.val != "") {
					t.Errorf("%s in the JSON = %v, want %v: %s", f.key, has, f.val != "", raw)
				}
			}
		})
	}
}

// The envelope is not scrubbed, so both fields reach the dry-run payload exactly as they were read, slash included.
func TestRefFieldsReachTheDryRunPayloadUnchanged(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "a/b", "GITHUB_REF_NAME": "42/merge",
		"GITHUB_HEAD_REF": "feature/x", "GITHUB_EVENT_NAME": "pull_request", "GITHUB_BASE_REF": "release/1.2",
		"GITHUB_SHA": strings.Repeat("a", 40), "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1",
		"GITHUB_EVENT_PATH": refEvent(t, `{"repository":{"default_branch":"main"}}`),
	}
	var out bytes.Buffer
	o := Options{
		Report:   []byte(`{"metadata":{"platform":"android"},"results":[]}`),
		Envelope: Envelope{ProbeVersion: "0.25.1", DeviceClass: "emulator", CI: CIFromEnv(envMap(env), nil)},
		DryRun:   true,
		Out:      &out,
	}
	if err := Upload(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"base_branch": "release/1.2"`, `"default_branch": "main"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry run must show %s:\n%s", want, out.String())
		}
	}
}
