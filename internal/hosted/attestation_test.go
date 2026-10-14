package hosted

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fakeJWT = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJyZXBvIn0.c2lnbmF0dXJlLWJ5dGVz"

var testReport = []byte(`{"metadata":{"platform":"android"},"results":[]}`)

func ciEnvelope() Envelope {
	return Envelope{ProbeVersion: "0.25.1", DeviceClass: "emulator", CI: &CIInfo{
		Provider: "github-actions", Repository: "a/b", Branch: "main", CommitSHA: strings.Repeat("a", 40), RunID: "1", RunAttempt: "1",
	}}
}

// tokenServer plays GitHub's identity token endpoint.
func tokenServer(t *testing.T, status int, body string, seen *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			seen.Store(r.Header.Get("Authorization") + "|" + r.URL.Query().Get("audience"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestRequestAttestationAsksGitHubForTheAudience(t *testing.T) {
	var seen atomic.Value
	srv := tokenServer(t, 200, `{"value":"`+fakeJWT+`"}`, &seen)
	defer srv.Close()
	env := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": srv.URL + "/token?api-version=2", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-credential"}
	var warnings []string
	got := RequestAttestation(context.Background(), envMap(env), nil, "", func(m string) { warnings = append(warnings, m) })
	if got != fakeJWT || len(warnings) != 0 {
		t.Fatalf("token %q, warnings %v", got, warnings)
	}
	if seen.Load() != "bearer request-credential|"+DefaultAudience {
		t.Errorf("request = %v; want the bearer credential and the default audience", seen.Load())
	}
	got = RequestAttestation(context.Background(), envMap(env), nil, "my-audience", nil)
	if got != fakeJWT || !strings.HasSuffix(seen.Load().(string), "|my-audience") {
		t.Errorf("the configured audience must be used: %v", seen.Load())
	}
}

func TestRequestAttestationFailuresAreWarningsAndNeverLeakTheCredential(t *testing.T) {
	long := `{"value":"` + strings.Repeat("a", MaxAttestationLength+1) + `"}`
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", 500, `oops request-credential`},
		{"forbidden", 403, `{}`},
		{"not json", 200, `<html>`},
		{"no value", 200, `{}`},
		{"too long", 200, long},
		{"whitespace in the token", 200, `{"value":"a b"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := tokenServer(t, c.status, c.body, nil)
			defer srv.Close()
			env := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": srv.URL, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-credential"}
			var warnings []string
			if got := RequestAttestation(context.Background(), envMap(env), nil, "", func(m string) { warnings = append(warnings, m) }); got != "" {
				t.Fatalf("a failed request must give no token, got %q", got)
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want exactly one", warnings)
			}
			if strings.Contains(warnings[0], "request-credential") {
				t.Errorf("the warning leaks the request credential: %s", warnings[0])
			}
		})
	}
}

func TestRequestAttestationWithoutVariablesIsQuiet(t *testing.T) {
	var warnings []string
	warn := func(m string) { warnings = append(warnings, m) }
	if got := RequestAttestation(context.Background(), envMap(map[string]string{}), nil, "", warn); got != "" || len(warnings) != 0 {
		t.Errorf("outside Actions: token %q, warnings %v; want nothing", got, warnings)
	}
	// Inside Actions without `id-token: write`: one line that says what to do.
	RequestAttestation(context.Background(), envMap(map[string]string{"GITHUB_ACTIONS": "true"}), nil, "", warn)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "id-token: write") {
		t.Errorf("warnings = %v; want one line naming permissions: id-token: write", warnings)
	}
}

func TestRequestAttestationNeverFollowsARedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.RedirectHandler(other.URL, http.StatusFound))
	defer srv.Close()
	env := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": srv.URL, "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-credential"}
	if got := RequestAttestation(context.Background(), envMap(env), nil, "", nil); got != "" {
		t.Errorf("a redirect must give no token, got %q", got)
	}
	if elsewhere.Load() != 0 {
		t.Error("the request credential was forwarded to another host")
	}
}

func TestRequestAttestationRefusesPlainHTTPToARemoteHost(t *testing.T) {
	env := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "http://tokens.example.com/x", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-credential"}
	var warnings []string
	if got := RequestAttestation(context.Background(), envMap(env), nil, "", func(m string) { warnings = append(warnings, m) }); got != "" || len(warnings) != 1 {
		t.Errorf("token %q, warnings %v", got, warnings)
	}
}

func TestAttestationAvailable(t *testing.T) {
	both := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "u", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"}
	if !AttestationAvailable(envMap(both)) || AttestationAvailable(envMap(map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "u"})) || AttestationAvailable(nil) {
		t.Error("available only when both variables are set")
	}
}

// ---- the upload ----

func TestUploadSendsTheAttestationHeaderOnlyToTheConfiguredEndpoint(t *testing.T) {
	var gotHeader, gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader.Store(r.Header.Get(AttestationHeader))
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(201)
	}))
	defer srv.Close()
	o := Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: srv.URL, Attestation: fakeJWT}
	if err := Upload(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if gotHeader.Load() != fakeJWT || gotAuth.Load() != "Bearer FP2.test.key.sig" {
		t.Errorf("header %v, authorization %v", gotHeader.Load(), gotAuth.Load())
	}
	o.Attestation = ""
	if err := Upload(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if gotHeader.Load() != "" {
		t.Errorf("without a token the header must be absent, got %v", gotHeader.Load())
	}
	o.Attestation = strings.Repeat("a", MaxAttestationLength+1)
	if err := Upload(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if gotHeader.Load() != "" {
		t.Error("a token over the limit must be left out")
	}
}

func TestUploadRedirectDoesNotForwardTheAttestation(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.RedirectHandler(other.URL, http.StatusTemporaryRedirect))
	defer srv.Close()
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: srv.URL, Attestation: fakeJWT})
	if err == nil || elsewhere.Load() != 0 {
		t.Fatalf("err %v, hits elsewhere %d; want a refusal and no forwarding", err, elsewhere.Load())
	}
	if strings.Contains(err.Error(), fakeJWT) {
		t.Error("the error shows the token")
	}
}

func TestDryRunShowsOnlyPresentOrAbsent(t *testing.T) {
	for _, c := range []struct {
		name string
		o    Options
		want string
	}{
		{"token", Options{Attestation: fakeJWT}, "attestation: present"},
		{"would request one", Options{DryAttestation: true}, "attestation: present"},
		{"none", Options{}, "attestation: absent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			o := c.o
			o.Report, o.Envelope, o.DryRun, o.Out = testReport, ciEnvelope(), true, &out
			if err := Upload(context.Background(), o); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), c.want+"\n") {
				t.Errorf("want %q in:\n%s", c.want, out.String())
			}
			if strings.Contains(out.String(), fakeJWT) || strings.Contains(out.String(), "eyJ") {
				t.Error("the dry run shows the token or a prefix of it")
			}
		})
	}
}

func TestTheTokenIsRedactedFromServerEchoesAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte("bad token " + r.Header.Get(AttestationHeader) + " and a cut-off eyJhbGciOiJSUzI1NiJ9.eyJzdWIi"))
	}))
	defer srv.Close()
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: srv.URL, Attestation: fakeJWT})
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "eyJ") || strings.Contains(err.Error(), fakeJWT) {
		t.Errorf("the error shows a token: %v", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Retryable() {
		t.Errorf("a 400 is a StatusError that is not retried: %v", err)
	}
}

func TestRejectedAttestationIsAWarningWithTheReasonCodeOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"attestation":"rejected","attestation_reason":"wrong_audience"}`))
	}))
	defer srv.Close()
	var warnings []string
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: srv.URL, Attestation: fakeJWT,
		Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatalf("a stored report is a success: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "wrong_audience") || strings.Contains(warnings[0], "eyJ") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestAttestationNote(t *testing.T) {
	for body, want := range map[string]string{
		`{"attestation":"base"}`:     "",
		`{"attestation":"verified"}`: "",
		`not json`:                   "",
		`{"attestation":"rejected","attestation_reason":"bad one"}`: "did not accept its identity token",
	} {
		got := attestationNote([]byte(body))
		if (want == "" && got != "") || (want != "" && !strings.Contains(got, want)) {
			t.Errorf("%s -> %q, want %q", body, got, want)
		}
	}
	if strings.Contains(attestationNote([]byte(`{"attestation":"rejected","attestation_reason":"bad one"}`)), "bad one") {
		t.Error("a reason that is not a short code must not be echoed")
	}
}

// ---- retries ----

type scripted struct {
	statuses []int
	retry    string
	hits     atomic.Int32
	srv      *httptest.Server
}

func newScripted(t *testing.T, statuses ...int) *scripted {
	t.Helper()
	s := &scripted{statuses: statuses}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(s.hits.Add(1)) - 1
		code := s.statuses[len(s.statuses)-1]
		if i < len(s.statuses) {
			code = s.statuses[i]
		}
		if s.retry != "" {
			w.Header().Set("Retry-After", s.retry)
		}
		w.WriteHeader(code)
		if code == 503 {
			_, _ = w.Write([]byte(`{"error":"attestation_unavailable"}`))
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// fakeClock is a clock that only moves when a test "sleeps".
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// sleepOn records each wait and moves the fake clock by it.
func (c *fakeClock) sleepOn(into *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error { *into = append(*into, d); c.t = c.t.Add(d); return nil }
}

func TestBusyServiceIsRetriedWithBackoffUntilItAnswers(t *testing.T) {
	s := newScripted(t, 429, 503, 503, 201)
	var sleeps []time.Duration
	var warnings []string
	clk := &fakeClock{t: time.Unix(0, 0)}
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s.srv.URL,
		Sleep: clk.sleepOn(&sleeps), Now: clk.now, Warn: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if s.hits.Load() != 4 {
		t.Errorf("requests = %d, want 4", s.hits.Load())
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	if len(sleeps) != 3 || sleeps[0] != want[0] || sleeps[1] != want[1] || sleeps[2] != want[2] {
		t.Errorf("backoff = %v, want %v", sleeps, want)
	}
	if len(warnings) == 0 {
		t.Error("a retry must be said out loud")
	}
}

func TestBackoffIsCappedAndRetryAfterIsHonouredUpToACap(t *testing.T) {
	s := newScripted(t, 503)
	s.retry = "120"
	var sleeps []time.Duration
	clk := &fakeClock{t: time.Unix(0, 0)}
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s.srv.URL,
		RetryWindow: 10 * time.Minute, Sleep: clk.sleepOn(&sleeps), Now: clk.now})
	if err == nil {
		t.Fatal("a service that never recovers must give an error in the end")
	}
	for _, d := range sleeps {
		if d != retryAfterCap {
			t.Fatalf("sleeps = %v; a Retry-After over the cap waits exactly the cap", sleeps)
		}
	}
	if len(sleeps) < 5 {
		t.Errorf("a 10-minute window with 60s waits allows nine waits, got %d", len(sleeps))
	}
}

func TestGivingUpAfterTheWindowIsAnErrorNotAPanicAndSaysSo(t *testing.T) {
	s := newScripted(t, 503)
	var sleeps []time.Duration
	clk := &fakeClock{t: time.Unix(0, 0)}
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s.srv.URL,
		RetryWindow: time.Millisecond, Sleep: clk.sleepOn(&sleeps), Now: clk.now})
	if err == nil || !strings.Contains(err.Error(), "still busy") || !strings.Contains(err.Error(), "nothing was stored") {
		t.Fatalf("err = %v", err)
	}
	if s.hits.Load() != 1 || len(sleeps) != 0 {
		t.Errorf("requests %d, sleeps %v; a window shorter than the first wait means one try", s.hits.Load(), sleeps)
	}
}

func TestOtherFailuresAreNotRetried(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 413, 500} {
		s := newScripted(t, code)
		err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s.srv.URL,
			Sleep: func(context.Context, time.Duration) error { t.Fatal("must not wait"); return nil }})
		if err == nil || s.hits.Load() != 1 {
			t.Errorf("HTTP %d: err %v, requests %d; want one try and an error", code, err, s.hits.Load())
		}
	}
}

func TestRetriesCanBeTurnedOffAndStopWithTheContext(t *testing.T) {
	s := newScripted(t, 503)
	if err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s.srv.URL, RetryWindow: -1}); err == nil || s.hits.Load() != 1 {
		t.Errorf("a negative window means no retries: err %v, requests %d", err, s.hits.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	s2 := newScripted(t, 503)
	err := Upload(ctx, Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig", BaseURL: s2.srv.URL,
		Sleep: func(c context.Context, _ time.Duration) error { cancel(); return c.Err() }})
	if err == nil || !strings.Contains(err.Error(), "retries stopped") {
		t.Errorf("err = %v", err)
	}
}

func TestACustomTransportErrorIsNotRetried(t *testing.T) {
	n := 0
	err := Upload(context.Background(), Options{Report: testReport, Envelope: ciEnvelope(), LicenseKey: "FP2.test.key.sig",
		Transport: transportFunc(func() error { n++; return errors.New("boom") })})
	if err == nil || n != 1 {
		t.Errorf("err %v, calls %d", err, n)
	}
}

type transportFunc func() error

func (f transportFunc) Send(context.Context, string, []byte) error { return f() }
