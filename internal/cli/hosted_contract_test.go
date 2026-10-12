package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// validateContract checks a payload against the rules of the hosted service's field table (written out
// here on purpose, independent of the client's own checks):
//
//	schema: 1
//	envelope.probe_version: non-empty
//	envelope.device_class: emulator | simulator | physical | cloud | unknown
//	envelope.ci (required): provider "github-actions"; repository; branch (never "N/merge");
//	  commit_sha exactly 40 lower-case hex; pull_request integer (pull requests only);
//	  base_branch optional (pull requests only); run_id and run_attempt decimal strings; job optional
//	envelope.run_label: optional, 1 to 64 of [0-9A-Za-z._:/-]
//	envelope.ci.job: optional, 1 to 100 of [0-9A-Za-z_-]
//	report.metadata.platform: android | ios
//	report.results: at most 5000; file relative; name; duration_ms; row optional integer;
//	  attempts optional 2 to 20; total/passed/failed/skipped equal the counts computed from results
//	body: at most 1 MiB
func validateContract(raw []byte) []string {
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	if len(raw) > 1<<20 {
		bad("body over 1 MiB")
	}
	var p struct {
		Schema   int `json:"schema"`
		Envelope struct {
			ProbeVersion string `json:"probe_version"`
			DeviceClass  string `json:"device_class"`
			RunLabel     string `json:"run_label"`
			CI           *struct {
				Provider    string `json:"provider"`
				Repository  string `json:"repository"`
				Branch      string `json:"branch"`
				BaseBranch  string `json:"base_branch"`
				CommitSHA   string `json:"commit_sha"`
				PullRequest any    `json:"pull_request"`
				RunID       any    `json:"run_id"`
				RunAttempt  any    `json:"run_attempt"`
				Job         string `json:"job"`
			} `json:"ci"`
		} `json:"envelope"`
		Report struct {
			Metadata struct {
				Platform string `json:"platform"`
			} `json:"metadata"`
			TotalTests int `json:"total_tests"`
			Passed     int `json:"passed"`
			Failed     int `json:"failed"`
			Skipped    int `json:"skipped"`
			Results    []struct {
				File     string   `json:"file"`
				Name     string   `json:"name"`
				Passed   bool     `json:"passed"`
				Skipped  bool     `json:"skipped"`
				Duration *float64 `json:"duration_ms"`
				Row      *int     `json:"row"`
				Attempts *int     `json:"attempts"`
			} `json:"results"`
		} `json:"report"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return []string{"not JSON: " + err.Error()}
	}
	if p.Schema != 1 {
		bad("schema %d", p.Schema)
	}
	if p.Envelope.ProbeVersion == "" {
		bad("probe_version missing")
	}
	switch p.Envelope.DeviceClass {
	case "emulator", "simulator", "physical", "cloud", "unknown":
	default:
		bad("device_class %q", p.Envelope.DeviceClass)
	}
	if s := p.Envelope.RunLabel; s != "" && !regexp.MustCompile(`^[0-9A-Za-z._:/-]{1,64}$`).MatchString(s) {
		bad("run_label %q", s)
	}
	ci := p.Envelope.CI
	if ci == nil {
		bad("ci is required")
	} else {
		if ci.Provider != "github-actions" {
			bad("ci.provider %q", ci.Provider)
		}
		if ci.Repository == "" || ci.Branch == "" {
			bad("ci.repository/branch missing")
		}
		if regexp.MustCompile(`^\d+/merge$`).MatchString(ci.Branch) {
			bad("ci.branch is the merge ref %q", ci.Branch)
		}
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ci.CommitSHA) {
			bad("ci.commit_sha %q", ci.CommitSHA)
		}
		for name, v := range map[string]any{"run_id": ci.RunID, "run_attempt": ci.RunAttempt} {
			if s, ok := v.(string); !ok || !regexp.MustCompile(`^[0-9]+$`).MatchString(s) {
				bad("ci.%s must be a decimal string, got %#v", name, v)
			}
		}
		if ci.PullRequest != nil {
			if n, ok := ci.PullRequest.(float64); !ok || n != float64(int(n)) || n < 1 {
				bad("ci.pull_request must be an integer, got %#v", ci.PullRequest)
			}
		}
		if ci.BaseBranch != "" && ci.PullRequest == nil {
			bad("ci.base_branch without a pull request")
		}
	}
	if pl := p.Report.Metadata.Platform; pl != "android" && pl != "ios" {
		bad("report.metadata.platform %q", pl)
	}
	if len(p.Report.Results) > 5000 {
		bad("too many results")
	}
	passed, failed, skipped := 0, 0, 0
	for i, r := range p.Report.Results {
		if r.File == "" || filepath.IsAbs(r.File) || strings.HasPrefix(r.File, "..") || strings.Contains(r.File, ":\\") {
			bad("results[%d].file %q is not relative", i, r.File)
		}
		if r.Name == "" || r.Duration == nil {
			bad("results[%d] needs name and duration_ms", i)
		}
		if r.Attempts != nil && (*r.Attempts < 2 || *r.Attempts > 20) {
			bad("results[%d].attempts %d", i, *r.Attempts)
		}
		switch {
		case r.Skipped:
			skipped++
		case r.Passed:
			passed++
		default:
			failed++
		}
	}
	if p.Report.TotalTests != len(p.Report.Results) || p.Report.Passed != passed || p.Report.Failed != failed || p.Report.Skipped != skipped {
		bad("counts %d/%d/%d/%d do not match the results %d/%d/%d/%d", p.Report.TotalTests, p.Report.Passed, p.Report.Failed, p.Report.Skipped, len(p.Report.Results), passed, failed, skipped)
	}
	return errs
}

func dryRunPayload(t *testing.T, platform, suite string) (payload []byte, out string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	c := uploadCmd(false, true)
	if suite != "" {
		c.Flags().String("upload-label", suite, "")
	}
	var cmdOut, status bytes.Buffer
	c.SetOut(&cmdOut)
	results := []runner.TestResult{
		{TestName: "login", File: "tests/login.probe", Passed: true, Duration: 120 * time.Millisecond},
		{TestName: "pay", File: "tests/pay.probe", Passed: true, Attempts: 25, Duration: time.Second},
		{TestName: "row case", File: "tests/data.probe", Passed: false, Row: 2, Duration: time.Second, Error: fmt.Errorf("boom")},
		{TestName: "later", File: "tests/later.probe", Skipped: true},
	}
	hostedUpload(c, &status, &config.Config{}, results, runner.RunMetadata{Platform: platform, ProbeVersion: "x"}, "simulator")
	out = cmdOut.String()
	i := strings.Index(out, "\n{")
	if i < 0 {
		t.Fatalf("no payload in the dry-run output: %q", out)
	}
	return []byte(out[i+1:]), out
}

func TestPayloadMeetsTheHostedContract(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name     string
		env      map[string]string
		platform string
		suite    string
		wantErrs []string // substrings of the validator's findings ("" = valid)
		wantNote bool     // the dry run says a real upload would be refused
		check    func(t *testing.T, p map[string]any)
	}{
		{
			name: "push event",
			env: map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
				"GITHUB_SHA": strings.ToUpper(sha), "GITHUB_RUN_ID": "99", "GITHUB_RUN_ATTEMPT": "2", "GITHUB_JOB": "e2e", "GITHUB_BASE_REF": ""},
			platform: "ios",
			check: func(t *testing.T, p map[string]any) {
				ci := p["envelope"].(map[string]any)["ci"].(map[string]any)
				if ci["branch"] != "main" || ci["commit_sha"] != sha || ci["job"] != "e2e" || ci["pull_request"] != nil || ci["base_branch"] != nil {
					t.Fatalf("ci = %v", ci)
				}
			},
		},
		{
			name: "pull request event",
			env: map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "pull_request", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "42/merge",
				"GITHUB_HEAD_REF": "feat/x", "GITHUB_BASE_REF": "main", "GITHUB_SHA": sha, "GITHUB_RUN_ID": "100", "GITHUB_RUN_ATTEMPT": "1"},
			platform: "android", suite: "shard-2-of-4",
			check: func(t *testing.T, p map[string]any) {
				env := p["envelope"].(map[string]any)
				ci := env["ci"].(map[string]any)
				if ci["branch"] != "feat/x" || ci["base_branch"] != "main" || ci["pull_request"] != float64(42) || env["run_label"] != "shard-2-of-4" {
					t.Fatalf("envelope = %v", env)
				}
			},
		},
		{
			name:     "no CI at all: dry run only, with a note",
			env:      map[string]string{"GITHUB_ACTIONS": ""},
			platform: "ios",
			wantErrs: []string{"ci is required"},
			wantNote: true,
		},
		{
			name: "no platform in the report",
			env: map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
				"GITHUB_SHA": sha, "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1"},
			platform: "",
			wantErrs: []string{"metadata.platform"},
			wantNote: true,
		},
		{
			name: "short commit hash",
			env: map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
				"GITHUB_SHA": "abc123", "GITHUB_RUN_ID": "1", "GITHUB_RUN_ATTEMPT": "1"},
			platform: "ios",
			wantErrs: []string{"commit_sha"},
			wantNote: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			payload, out := dryRunPayload(t, c.platform, c.suite)
			errs := validateContract(payload)
			if len(c.wantErrs) == 0 && len(errs) != 0 {
				t.Fatalf("the payload breaks the contract: %v\n%s", errs, payload)
			}
			for _, want := range c.wantErrs {
				if !strings.Contains(strings.Join(errs, "; "), want) {
					t.Fatalf("expected a finding about %q, got %v", want, errs)
				}
			}
			if gotNote := strings.Contains(out, "a real upload would be refused"); gotNote != c.wantNote {
				t.Fatalf("note = %v, want %v:\n%s", gotNote, c.wantNote, out)
			}
			if c.check != nil {
				var p map[string]any
				if err := json.Unmarshal(payload, &p); err != nil {
					t.Fatal(err)
				}
				c.check(t, p)
			}
		})
	}
}

func TestInvalidPayloadsAreNotSentForReal(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A real upload outside CI is refused with a clear message before anything is sent.
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator")
	if !strings.Contains(w.String(), "not a GitHub Actions run") && !strings.Contains(w.String(), "license") {
		t.Fatalf("out=%q", w.String())
	}
}

func TestTwoUploadsOfOneJobDifferOnlyByTheirLabel(t *testing.T) {
	for k, v := range map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
		"GITHUB_SHA": "0123456789abcdef0123456789abcdef01234567", "GITHUB_RUN_ID": "5", "GITHUB_RUN_ATTEMPT": "1", "GITHUB_JOB": "e2e"} {
		t.Setenv(k, v)
	}
	label := func(l string) string {
		p, _ := dryRunPayload(t, "android", l)
		var v struct {
			Envelope struct {
				RunLabel string `json:"run_label"`
				CI       struct {
					Job string `json:"job"`
				} `json:"ci"`
			} `json:"envelope"`
		}
		if err := json.Unmarshal(p, &v); err != nil {
			t.Fatal(err)
		}
		if v.Envelope.CI.Job != "e2e" {
			t.Fatalf("job = %q", v.Envelope.CI.Job)
		}
		return v.Envelope.RunLabel
	}
	a, b, none := label("shard-1-of-2"), label("shard-2-of-2"), label("")
	if a == b || a == "" || b == "" || none != "" {
		t.Fatalf("labels: %q %q %q", a, b, none)
	}
	// Matrix values with spaces and brackets are folded into the allowed character set, never rejected.
	if got := label("ios 18 [iPhone 17]"); got != "ios-18--iPhone-17-" {
		t.Fatalf("sanitized label = %q", got)
	}
}

// A retried test must not carry its absolute local path into the envelope (it leaked the machine's path and a server
// refuses a payload whose retry file is absolute): walk every string of the dry-run payload.
func TestDryRunPayloadOfARetriedTestHasNoLocalPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for k, v := range map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
		"GITHUB_SHA": "0123456789abcdef0123456789abcdef01234567", "GITHUB_RUN_ID": "5", "GITHUB_RUN_ATTEMPT": "1"} {
		t.Setenv(k, v)
	}
	cwd, _ := os.Getwd()
	c := uploadCmd(false, true)
	var cmdOut, status bytes.Buffer
	c.SetOut(&cmdOut)
	results := []runner.TestResult{
		{TestName: "pay", File: filepath.Join(cwd, "tests", "pay.probe"), Passed: true, Row: -1, Attempts: 2, Duration: time.Second},
		{TestName: "other", File: "/Users/jane/elsewhere/other.probe", Passed: false, Row: -1, Attempts: 3, Error: fmt.Errorf("boom"), Duration: time.Second},
	}
	hostedUpload(c, &status, &config.Config{}, results, runner.RunMetadata{Platform: "android"}, "emulator")
	out := cmdOut.String()
	i := strings.Index(out, "\n{")
	if i < 0 {
		t.Fatalf("no payload: %q", out)
	}
	payload := []byte(out[i+1:])
	var doc any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				walk(y, path+"."+k)
			}
		case []any:
			for i, y := range x {
				walk(y, fmt.Sprintf("%s[%d]", path, i))
			}
		case string:
			if strings.HasPrefix(x, "/") || strings.Contains(x, "/Users/") || strings.Contains(x, cwd) {
				t.Errorf("%s holds a local path: %q", path, x)
			}
		}
	}
	walk(doc, "$")
	if errs := validateContract(payload); len(errs) != 0 {
		t.Errorf("the payload breaks the contract: %v", errs)
	}
	var p struct {
		Envelope struct {
			Retries []struct {
				File string `json:"file"`
			} `json:"retries"`
		} `json:"envelope"`
	}
	_ = json.Unmarshal(payload, &p)
	if len(p.Envelope.Retries) != 2 || p.Envelope.Retries[0].File != "tests/pay.probe" || p.Envelope.Retries[1].File != "other.probe" {
		t.Errorf("retries = %+v", p.Envelope.Retries)
	}
}
