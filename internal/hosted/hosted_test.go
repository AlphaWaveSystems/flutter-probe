package hosted

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestCIFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want *CIInfo
	}{
		{"not in CI", map[string]string{"GITHUB_REPOSITORY": "a/b"}, nil},
		{"push", map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "a/b", "GITHUB_REF_NAME": "main", "GITHUB_SHA": "abc", "GITHUB_RUN_ID": "7", "GITHUB_RUN_ATTEMPT": "2", "GITHUB_EVENT_NAME": "push"},
			&CIInfo{Provider: "github-actions", Repository: "a/b", Branch: "main", CommitSHA: "abc", RunID: "7", RunAttempt: "2"}},
		{"pull request", map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_REPOSITORY": "a/b", "GITHUB_REF_NAME": "42/merge", "GITHUB_HEAD_REF": "feat/x", "GITHUB_EVENT_NAME": "pull_request", "GITHUB_SHA": "def", "GITHUB_RUN_ID": "8", "GITHUB_RUN_ATTEMPT": "1"},
			&CIInfo{Provider: "github-actions", Repository: "a/b", Branch: "feat/x", CommitSHA: "def", PullRequest: 42, RunID: "8", RunAttempt: "1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CIFromEnv(envMap(c.env))
			if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestScrubDropsArtifactsPathsAndSecrets(t *testing.T) {
	report := `{
	  "metadata": {"device_id": "UDID-1", "device_name": "Pixel", "config_file": "/Users/jane/proj/probe.yaml", "probe_version": "0.24.0"},
	  "results": [
	    {"name": "login", "file": "/Users/jane/proj/tests/login.probe", "passed": false,
	     "error": "failed at /Users/jane/proj/tests/login.probe line 3 with token=abcd1234efgh and key AKIAABCDEFGHIJKLMNOP",
	     "artifacts": ["/Users/jane/proj/reports/shot.png"], "duration_ms": 120}
	  ]}`
	sc := &Scrubber{Cwd: "/Users/jane/proj", Home: "/Users/jane"}
	out, st, err := sc.Scrub([]byte(report))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"/Users/", "jane", "UDID-1", "shot.png", "abcd1234efgh", "AKIAABCDEFGHIJKLMNOP"} {
		if strings.Contains(s, bad) {
			t.Errorf("%q must not be in the payload:\n%s", bad, s)
		}
	}
	for _, want := range []string{`"file": "tests/login.probe"`, `"config_file": "probe.yaml"`, "[redacted]", `"duration_ms": 120`, `"device_name": "Pixel"`} {
		if !strings.Contains(s, want) {
			t.Errorf("%q missing:\n%s", want, s)
		}
	}
	if st.DroppedFields != 2 || st.Redactions == 0 {
		t.Errorf("stats = %+v", st)
	}
}

func TestScrubKeepsRelativePathsAndRefusesBadJSON(t *testing.T) {
	sc := &Scrubber{Cwd: "/work", Home: "/home/x"}
	out, _, err := sc.Scrub([]byte(`{"results":[{"file":"tests/a.probe","error":"windows C:\\Users\\bob\\app broke"}]}`))
	if err != nil || !strings.Contains(string(out), "tests/a.probe") || strings.Contains(string(out), "bob") {
		t.Fatalf("%s %v", out, err)
	}
	if _, _, err := sc.Scrub([]byte("not json")); err == nil {
		t.Fatal("invalid JSON must be refused")
	}
}

func TestSecretPatterns(t *testing.T) {
	cases := []string{
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789", "github_pat_abcdefghijklmnopqrstuvwxyz", "sk-abcdefghijklmnopqrstuvwxyz",
		"Authorization: Bearer abcdefghijklmnop", "password=hunter22", "api_key: abcdefgh1234",
		"eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.SflKxwRJSMeKKF2QT4", "FP2.k1.payloadpayload.sigsigsigsig",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIE\n-----END RSA PRIVATE KEY-----",
	}
	sc := &Scrubber{Cwd: "/work", Home: "/home/x"}
	for _, c := range cases {
		got := sc.scrubString("before " + c + " after")
		if !strings.Contains(got, "[redacted]") || strings.Contains(got, "hunter22") || strings.Contains(got, "abcdefghijklmnop") {
			t.Errorf("%q -> %q", c, got)
		}
	}
	if got := sc.scrubString("tap on the Pay button"); got != "tap on the Pay button" {
		t.Errorf("harmless text changed: %q", got)
	}
}

func baseOpts(report string) Options {
	return Options{
		Report:     []byte(report),
		Envelope:   Envelope{ProbeVersion: "0.24.0", DeviceClass: "simulator", CI: &CIInfo{Provider: "github-actions", Repository: "a/b"}, Retries: []RetryOutcome{{File: "a.probe", Test: "t", Attempts: 2, Passed: true}}},
		LicenseKey: "FP2.test.key.sig",
		Scrubber:   &Scrubber{Cwd: "/work", Home: "/home/x"},
	}
}

func TestUploadSendsScrubbedPayloadWithBearer(t *testing.T) {
	var gotAuth, gotPath, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath, gotCT = r.Header.Get("Authorization"), r.URL.Path, r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	o := baseOpts(`{"total_tests":1,"results":[{"name":"t","file":"/work/a.probe","passed":true,"artifacts":["/work/x.png"]}]}`)
	o.BaseURL = srv.URL
	if err := Upload(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer FP2.test.key.sig" || gotPath != "/v1/reports" || gotCT != "application/json" {
		t.Fatalf("auth=%q path=%q ct=%q", gotAuth, gotPath, gotCT)
	}
	var p Payload
	if err := json.Unmarshal(gotBody, &p); err != nil || p.Schema != SchemaVersion || p.Envelope.DeviceClass != "simulator" || len(p.Envelope.Retries) != 1 {
		t.Fatalf("payload = %s (%v)", gotBody, err)
	}
	if bytes.Contains(gotBody, []byte("x.png")) || bytes.Contains(gotBody, []byte("/work/")) {
		t.Fatalf("artifacts and absolute paths must not be sent: %s", gotBody)
	}
}

func TestUploadErrorsAreReturnedNotFatal(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }, "HTTP 500"},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no plan", 403) }, "HTTP 403"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(c.handler)
			defer srv.Close()
			o := baseOpts(`{"results":[]}`)
			o.BaseURL = srv.URL
			if err := Upload(context.Background(), o); err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestUploadTimesOutQuickly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(2 * time.Second) }))
	defer srv.Close()
	o := baseOpts(`{"results":[]}`)
	o.BaseURL = srv.URL
	o.Timeout = 150 * time.Millisecond
	start := time.Now()
	if err := Upload(context.Background(), o); err == nil {
		t.Fatal("a slow endpoint must produce an error")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("the upload blocked for %v", time.Since(start))
	}
}

func TestDryRunPrintsThePayloadAndSendsNothing(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	o := baseOpts(`{"results":[{"name":"t","file":"/work/a.probe"}]}`)
	o.BaseURL = srv.URL
	o.DryRun = true
	o.LicenseKey = "" // a dry run needs no key
	var out bytes.Buffer
	o.Out = &out
	if err := Upload(context.Background(), o); err != nil || hit {
		t.Fatalf("err=%v hit=%v", err, hit)
	}
	if !strings.Contains(out.String(), `"envelope"`) || !strings.Contains(out.String(), "a.probe") || strings.Contains(out.String(), "FP2.") {
		t.Fatalf("dry run output:\n%s", out.String())
	}
}

func TestPayloadOverTheCapShortensErrorsThenRefuses(t *testing.T) {
	big := strings.Repeat("x", 1900)
	var results []string
	for i := 0; i < 700; i++ {
		results = append(results, `{"name":"t","file":"a.probe","error":"`+big+`"}`)
	}
	o := baseOpts(`{"results":[` + strings.Join(results, ",") + `]}`)
	body, _, err := BuildPayload(o)
	if err != nil || len(body) > MaxPayloadBytes {
		t.Fatalf("shortening errors should fit: len=%d err=%v", len(body), err)
	}
	var many []string
	for i := 0; i < 20000; i++ {
		many = append(many, `{"name":"test number `+strings.Repeat("n", 40)+`","file":"a.probe","passed":true}`)
	}
	o = baseOpts(`{"results":[` + strings.Join(many, ",") + `]}`)
	if _, _, err := BuildPayload(o); err == nil || !strings.Contains(err.Error(), "nothing was sent") {
		t.Fatalf("a huge report must be refused: %v", err)
	}
}

func TestValidateBaseURL(t *testing.T) {
	for url, ok := range map[string]bool{
		"https://api.flutterprobe.dev": true, "http://localhost:8080": true, "http://127.0.0.1:9": true,
		"http://example.com": false, "ftp://x": false, "not a url": false, "": false,
	} {
		if _, err := ValidateBaseURL(url); (err == nil) != ok {
			t.Errorf("%q: err=%v want ok=%v", url, err, ok)
		}
	}
}
