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

const goodSHA = "0123456789abcdef0123456789abcdef01234567"

func goodCI() *CIInfo {
	return &CIInfo{Provider: "github-actions", Repository: "a/b", Branch: "main", CommitSHA: goodSHA, RunID: "7", RunAttempt: "1"}
}

// baseOpts is a valid upload: CI facts complete and the report names its platform.
func baseOpts(report string) Options {
	report = strings.Replace(report, "{", `{"metadata":{"platform":"android"},`, 1)
	return Options{
		Report:     []byte(report),
		Envelope:   Envelope{ProbeVersion: "0.24.0", DeviceClass: "simulator", CI: goodCI(), Retries: []RetryOutcome{{File: "a.probe", Test: "t", Attempts: 2, Passed: true}}},
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
		{"server error", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) }, "HTTP 500"},
		{"unauthorized", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no plan", http.StatusForbidden) }, "HTTP 403"},
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

func TestRedirectsAreNeverFollowedSoTheKeyCannotLeak(t *testing.T) {
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = r.Header.Get("Authorization") != ""
	}))
	defer target.Close()
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/v1/reports", code)
		}))
		o := baseOpts(`{"results":[]}`)
		o.BaseURL = srv.URL
		err := Upload(context.Background(), o)
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "redirect") {
			t.Fatalf("HTTP %d: err = %v", code, err)
		}
	}
	if leaked {
		t.Fatal("the Authorization header was forwarded to a redirect target")
	}
}

func TestServerTextIsSanitizedBeforeItIsShown(t *testing.T) {
	hostile := "\x1b[31mRED\x1b[0m \x1b]0;pwned title\x07ok\x00\x07\x08 bell\r\nnext line\xff\xfe \u0085 and the key FP2.test.key.sig echoed"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(hostile))
	}))
	defer srv.Close()
	o := baseOpts(`{"results":[]}`)
	o.BaseURL = srv.URL
	err := Upload(context.Background(), o)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, r := range msg {
		if r < 0x20 && r != '\t' || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("control character %U in %q", r, msg)
		}
	}
	for _, bad := range []string{"\x1b", "pwned", "FP2.test.key.sig"} {
		if strings.Contains(msg, bad) {
			t.Fatalf("%q must not be shown: %q", bad, msg)
		}
	}
	if !strings.Contains(msg, "HTTP 502") || !strings.Contains(msg, "RED") || !strings.Contains(msg, "[redacted]") {
		t.Fatalf("message = %q", msg)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"plain":                    "plain",
		"a\x1b[2Jb":                "ab",
		"tab\tnew\nline":           "tab new line",
		"bad\xffutf8":              "badutf8",
		"c1\u009bcontrol":          "c1control",
		"key=SECRETKEY here":       "key=[redacted] here",
		"\x1b]8;;http://x\x07link": "link",
	}
	for in, want := range cases {
		if got := Sanitize(in, "SECRETKEY"); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabelsAndJobAreFittedToTheirCharsets(t *testing.T) {
	for in, want := range map[string]string{"shard-2-of-4": "shard-2-of-4", "ios 18 [x]": "ios-18--x-", "ünï": "-n-", "a:b/c.d_e": "a:b/c.d_e", strings.Repeat("x", 90): strings.Repeat("x", 64)} {
		got := SanitizeLabel(in)
		if got != want || !ValidLabel(got) {
			t.Errorf("SanitizeLabel(%q) = %q (valid=%v), want %q", in, got, ValidLabel(got), want)
		}
	}
	if ValidLabel("has space") || ValidLabel(strings.Repeat("x", 65)) || !ValidLabel("") {
		t.Error("ValidLabel")
	}
	env := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_JOB": "e2e_tests-1"}
	if ci := CIFromEnv(envMap(env)); ci.Job != "e2e_tests-1" {
		t.Errorf("job = %q", ci.Job)
	}
	env["GITHUB_JOB"] = "bad job!"
	if ci := CIFromEnv(envMap(env)); ci.Job != "" {
		t.Errorf("an invalid job id must be omitted, got %q", ci.Job)
	}
}

func TestPreflightNamesWhatWouldBeRefused(t *testing.T) {
	ok := baseOpts(`{"results":[]}`)
	if p := Preflight(ok.Envelope, ok.Report); len(p) != 0 {
		t.Fatalf("a valid upload has problems: %v", p)
	}
	cases := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{"no CI", func(o *Options) { o.Envelope.CI = nil }, "not a GitHub Actions run"},
		{"short sha", func(o *Options) { o.Envelope.CI.CommitSHA = "abc" }, "40-character"},
		{"upper-case sha", func(o *Options) { o.Envelope.CI.CommitSHA = strings.ToUpper(goodSHA) }, "40-character"},
		{"run id not a number", func(o *Options) { o.Envelope.CI.RunID = "x1" }, "not a number"},
		{"no branch", func(o *Options) { o.Envelope.CI.Branch = "" }, "no branch"},
		{"bad label", func(o *Options) { o.Envelope.RunLabel = "a b" }, "run label"},
		{"no platform", func(o *Options) { o.Report = []byte(`{"results":[]}`) }, "platform"},
		{"too many results", func(o *Options) {
			o.Report = []byte(`{"metadata":{"platform":"ios"},"results":[` + strings.TrimSuffix(strings.Repeat("{},", MaxResults+1), ",") + `]}`)
		}, "at most"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := baseOpts(`{"results":[]}`)
			o.Envelope.CI = goodCI()
			c.mutate(&o)
			if p := strings.Join(Preflight(o.Envelope, o.Report), "; "); !strings.Contains(p, c.want) {
				t.Fatalf("problems = %q, want %q", p, c.want)
			}
			o.LicenseKey = "k"
			if err := Upload(context.Background(), o); err == nil || !strings.Contains(err.Error(), "nothing was uploaded") {
				t.Fatalf("a refused upload must not be sent: %v", err)
			}
		})
	}
}

func TestStripeShapedSecretsAreRedacted(t *testing.T) {
	// Built from parts so this file itself holds no key-shaped literal.
	live := "sk" + "_live_" + "51HabcdEFGHijklMNOP"
	secrets := []string{
		live, "sk" + "_test_" + "4eC39HqLyjWDarjtT1zdp7dc", "rk" + "_live_" + "51HabcdEFGHijklMNOP", "rk" + "_test_" + "51HabcdEFGHijklMNOP",
		"whsec" + "_" + "t2ZBuQwlq8kLm9nPqRsTuVwX",
	}
	sc := &Scrubber{Cwd: "/work", Home: "/home/x"}
	for _, k := range secrets {
		for name, in := range map[string]string{
			"bare":         "key " + k + " here",
			"in an error":  "request failed: Authorization: " + k + " was rejected",
			"in a url":     "https://api.example/pay?api=" + k,
			"after a name": "STRIPE_KEY=" + k,
			"glued by _":   "my_" + k,
		} {
			got := sc.scrubString(in)
			if strings.Contains(got, k[:12]) || !strings.Contains(got, "[redacted]") {
				t.Errorf("%s (%s): %q", name, k[:8], got)
			}
		}
	}
	// Nested JSON: a secret inside an error, a test name and a nested object is redacted wherever it is.
	report := `{"results":[{"name":"pays with ` + live + `","file":"a.probe","error":"HTTP 401 for ` + live + `","extra":{"deep":["x ` + live + ` y"]}}]}`
	out, st, err := sc.Scrub([]byte(report))
	if err != nil || strings.Contains(string(out), "51Habcd") || st.Redactions < 3 {
		t.Fatalf("out=%s stats=%+v err=%v", out, st, err)
	}
	// Harmless look-alikes stay.
	for _, ok := range []string{"task_live_view", "the risk_test_suite", "sk_live"} {
		if got := sc.scrubString(ok); got != ok {
			t.Errorf("%q changed to %q", ok, got)
		}
	}
}

func TestRetriesInTheEnvelopeAreRelativeAndScrubbed(t *testing.T) {
	o := baseOpts(`{"results":[{"name":"t","file":"/work/tests/a.probe","passed":true}]}`)
	o.Envelope.Retries = []RetryOutcome{
		{File: "/work/tests/a.probe", Test: "checkout [row 2]", Attempts: 2, Passed: true},
		{File: "/Users/jane/other/b.probe", Test: "uses sk" + "_live_" + "51HabcdEFGHijklMNOP", Attempts: 3, Passed: false},
		{File: "tests/c.probe", Test: "already relative", Attempts: 2, Passed: true},
	}
	body, _, err := BuildPayload(o)
	if err != nil {
		t.Fatal(err)
	}
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	want := []string{"tests/a.probe", "b.probe", "tests/c.probe"}
	for i, r := range p.Envelope.Retries {
		if r.File != want[i] {
			t.Errorf("retries[%d].file = %q, want %q", i, r.File, want[i])
		}
	}
	if strings.Contains(string(body), "51Habcd") {
		t.Errorf("a secret in a retried test's name must be redacted:\n%s", body)
	}
	// Walk every string of the payload: none may start with "/" or hold a home directory.
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, y := range x {
				walk(y)
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		case string:
			if strings.HasPrefix(x, "/") || strings.Contains(x, "/Users/") || strings.Contains(x, "/home/") {
				t.Errorf("a local path in the payload: %q", x)
			}
		}
	}
	var any_ any
	if err := json.Unmarshal(body, &any_); err != nil {
		t.Fatal(err)
	}
	walk(any_)
}
