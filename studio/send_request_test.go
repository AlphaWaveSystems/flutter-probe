package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sendResolver map[string][]string

func (r sendResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	var out []net.IPAddr
	for _, s := range r[host] {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	if len(out) == 0 {
		return nil, errors.New("no such host")
	}
	return out, nil
}

func hostPort(srv *httptest.Server) (string, string) {
	h, p, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	return h, p
}

// backend is a fake service on 127.0.0.1 that answers with what a real one might hold: a secret in a header, a body with credentials and
// markup, and what it received.
func backend(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "sid="+netPlain+"; HttpOnly")
		w.Header().Set("X-Seen-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Request-Id", "req-7")
		_, _ = w.Write([]byte(`{"ok":true,"password":"` + netPlain + `","note":"<img src=x onerror=1>","email":"` + netEmail + `"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// asJSON is what the page gets: a pointer inside a result is followed.
func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestSendRequestNeedsAnAllowedHost(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	a := &App{}
	in := SendInput{Method: "GET", URL: srv.URL + "/p"}
	if _, err := a.SendRequest(in); !errors.Is(err, errSendHostNotAllowed) {
		t.Fatalf("an unallowed host: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("a request that was refused reached the server")
	}
	info, err := a.CheckTarget(srv.URL + "/p")
	h, p := hostPort(srv)
	if err != nil || info.Host != h || info.Port != p || info.Key != h+":"+p || info.Allowed || !info.Local || len(info.Addresses) != 1 || info.Classes[0] != "this machine" {
		t.Fatalf("%+v %v", info, err)
	}
	if hits.Load() != 0 {
		t.Fatal("CheckTarget sent a request")
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatal(err)
	}
	if again, _ := a.CheckTarget(srv.URL); !again.Allowed {
		t.Fatal("the host is not remembered")
	}
	// Another port of the same host is another host.
	if other, _ := a.CheckTarget("http://" + h + ":1/"); other.Allowed {
		t.Fatal("another port was allowed")
	}
	in.Headers = []SendHeader{{Name: "Authorization", Value: "Bearer " + netPlain}, {Name: "X-Note", Value: "ok"}}
	in.URL += "?token=" + netPlain + "&id=7"
	res, err := a.SendRequest(in)
	if err != nil || hits.Load() != 1 {
		t.Fatalf("%v, %d hit(s)", err, hits.Load())
	}
	if res.Status != 200 || res.RemoteClass != "this machine" || res.Truncated || res.Stopped != nil {
		t.Fatalf("%+v", res)
	}
	// What was received is shown redacted, and what was typed is not echoed with its secrets.
	all := asJSON(res)
	mustNotContain(t, "the result", all, netPlain, netEmail, "jane.doe")
	if !strings.Contains(res.Sent, "?token=[redacted]&id=[redacted]") || !strings.HasPrefix(res.Sent, "GET ") {
		t.Errorf("sent = %q", res.Sent)
	}
	hv := func(hs []NetHeader, name string) string {
		for _, h := range hs {
			if h.Name == name {
				return h.Value
			}
		}
		return "<missing>"
	}
	if hv(res.RequestHeaders, "Authorization") != redactedMark || hv(res.RequestHeaders, "X-Note") != "ok" || hv(res.ResponseHeaders, "Set-Cookie") != redactedMark || hv(res.ResponseHeaders, "X-Seen-Auth") != redactedMark || hv(res.ResponseHeaders, "X-Request-Id") != "req-7" {
		t.Errorf("headers %+v %+v", res.RequestHeaders, res.ResponseHeaders)
	}
	if !strings.Contains(res.ResponseBody.Text, `"password": "[redacted]"`) || !strings.Contains(res.ResponseBody.Text, "<img src=x onerror=1>") {
		t.Errorf("body %q", res.ResponseBody.Text)
	}
}

// mustAllow does what a person does: check where a host leads, then allow it.
func mustAllow(t *testing.T, a *App, rawURL string) {
	t.Helper()
	info, err := a.CheckTarget(rawURL)
	if err != nil {
		t.Fatalf("check %s: %v", rawURL, err)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatalf("allow %s: %v (%+v)", info.Key, err, info)
	}
}

// forceAllow makes an allow that a check would not have made, to test what the call does when the name leads somewhere else later.
func forceAllow(a *App, key string, level int) {
	a.send.mu.Lock()
	defer a.send.mu.Unlock()
	if a.send.allowed == nil {
		a.send.allowed = map[string]int{}
	}
	a.send.allowed[key] = level
}

func TestAllowHostRefusesNonsense(t *testing.T) {
	a := &App{}
	for _, k := range []string{"", "host", ":80", "host:", "a/b:80", "u@h:80", "h :80", "h:80\n", "Example.TEST:443", "example.test.:443", "example.test:0", "example.test:65536",
		"example.test:080", "::1:80", "[::1]", "[fe80::1%eth0]:80", strings.Repeat("a", 1<<20) + ":80", strings.Repeat("a", 300) + ".test:80"} {
		if err := a.AllowHost(k); err == nil {
			t.Errorf("%q was accepted", k[:min(len(k), 40)])
		}
	}
	// A host that was not checked is not allowed, whatever its key.
	if err := a.AllowHost("example.test:443"); !errors.Is(err, errSendNotChecked) {
		t.Fatalf("an unchecked host: %v", err)
	}
	a.send.resolver = sendResolver{"example.test": {"127.0.0.2"}}
	info, err := a.CheckTarget("https://Example.TEST./x")
	if err != nil || info.Key != "example.test:443" {
		t.Fatalf("%+v %v", info, err)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatal(err)
	}
	if again, _ := a.CheckTarget("https://example.test/x"); !again.Allowed {
		t.Error("host names are matched without case and without the trailing dot")
	}
}

// Everything is keyed in the ASCII form: an IPv6 address in brackets, a name written in unicode in punycode, with the unicode name as a label.
func TestKeysAreInTheNormalForm(t *testing.T) {
	a := &App{}
	a.send.resolver = sendResolver{"xn--bcher-kva.test": {"127.0.0.2"}, "v6.test": {"::1"}}
	info, err := a.CheckTarget("http://[::1]:8080/x")
	if err != nil || info.Key != "[::1]:8080" || info.Host != "::1" {
		t.Fatalf("v6 literal: %+v %v", info, err)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatalf("[::1]: %v", err)
	}
	if again, _ := a.CheckTarget("http://[::1]:8080/y"); !again.Allowed {
		t.Error("[::1] is not remembered")
	}
	if info, err = a.CheckTarget("http://[2001:db8::1]/"); err != nil || info.Key != "[2001:db8::1]:80" {
		t.Fatalf("documentation range: %+v %v", info, err)
	}
	info, err = a.CheckTarget("https://b\u00fccher.test/")
	if err != nil || info.Key != "xn--bcher-kva.test:443" || info.Host != "xn--bcher-kva.test" || info.Label != "b\u00fccher.test" || len(info.Addresses) != 1 {
		t.Fatalf("idn: %+v %v", info, err)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatalf("idn: %v", err)
	}
	if again, _ := a.CheckTarget("https://B\u00fcCHER.test./"); !again.Allowed {
		t.Error("the unicode and the ASCII forms are not the same host")
	}
	if err := a.AllowHost("b\u00fccher.test:443"); err == nil {
		t.Error("a unicode key was accepted")
	}
}

// What is remembered is bounded: the length of a host, how many hosts, and how many address checks a minute.
func TestSendMemoryAndChecksAreBounded(t *testing.T) {
	a := &App{}
	a.send.resolver = sendResolver{}
	clock := time.Unix(5000, 0)
	a.send.now = func() time.Time { return clock }
	for i := 0; i < sendMaxAllowed+20; i++ {
		clock = clock.Add(5 * time.Second) // the limit on checks is not what is tested here
		host := "h" + strconv.Itoa(i) + ".test"
		a.send.resolver.(sendResolver)[host] = []string{"127.0.0.2"}
		mustAllow(t, a, "http://"+host+"/")
	}
	a.send.mu.Lock()
	n, shown := len(a.send.allowed), len(a.send.shown)
	a.send.mu.Unlock()
	if n != sendMaxAllowed || shown != sendMaxAllowed {
		t.Fatalf("%d allowed and %d shown, want %d", n, shown, sendMaxAllowed)
	}
	if again, _ := a.CheckTarget("http://h0.test/"); again.Allowed {
		t.Error("the oldest allow was kept")
	}
	if _, err := a.CheckTarget("http://" + strings.Repeat("a", 300) + ".test/"); err == nil {
		t.Error("a host longer than 253 bytes was looked up")
	}
	b := &App{}
	b.send.resolver = sendResolver{"c.test": {"127.0.0.2"}}
	b.send.now = func() time.Time { return time.Unix(1000, 0) }
	for i := 0; i < sendCheckPerMin; i++ {
		if _, err := b.CheckTarget("http://c.test/"); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if _, err := b.CheckTarget("http://c.test/"); !errors.Is(err, errSendCheckRate) {
		t.Fatalf("one check too many: %v", err)
	}
	b.send.now = func() time.Time { return time.Unix(1000+61, 0) }
	if _, err := b.CheckTarget("http://c.test/"); err != nil {
		t.Fatalf("a minute later: %v", err)
	}
}

// The request is checked as the engine's function checks it, and the answers are fixed sentences.
func TestSendRequestRefusalsAreFixedSentences(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	a := &App{}
	h, p := hostPort(srv)
	mustAllow(t, a, "http://"+h+":"+p+"/")
	cases := map[string]SendInput{
		"a non-http address": {Method: "GET", URL: "file:///etc/passwd"},
		"ftp":                {Method: "GET", URL: "ftp://" + h + ":" + p + "/"},
		"user info":          {Method: "GET", URL: "http://u:pw@" + h + ":" + p + "/"},
		"no host":            {Method: "GET", URL: "http:///x"},
		"a method":           {Method: "CONNECT", URL: srv.URL},
		"a forbidden header": {Method: "GET", URL: srv.URL, Headers: []SendHeader{{Name: "Host", Value: "evil.test"}}},
		"a body with GET":    {Method: "GET", URL: srv.URL, Body: "x"},
		"a body over 256 KB": {Method: "POST", URL: srv.URL, Body: strings.Repeat("a", 256<<10+1)},
		"a line break":       {Method: "GET", URL: srv.URL, Headers: []SendHeader{{Name: "X-A", Value: "a\r\nX-B: b"}}},
	}
	for name, in := range cases {
		_, err := a.SendRequest(in)
		if err == nil {
			t.Errorf("%s was sent", name)
			continue
		}
		mustNotContain(t, name, err.Error(), netPlain, "pw", "/etc/passwd")
	}
	if hits.Load() != 0 {
		t.Fatalf("a refused request reached the server %d time(s)", hits.Load())
	}
	// A body that is a path is text: nothing is read from a file.
	if _, err := a.SendRequest(SendInput{Method: "POST", URL: srv.URL, Body: "/etc/passwd"}); err != nil || hits.Load() != 1 {
		t.Fatalf("a path as a body: %v", err)
	}
}

// The address policy: the metadata addresses are never allowed; in the headless build an address on this machine is refused unless the
// test allow-list names the host; the native app allows it (the person was warned and allowed the host).
func TestSendPolicy(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	h, p := hostPort(srv)
	key := h + ":" + p

	// A name that resolves to a metadata address is refused after the lookup, with no connection made.
	meta := &App{}
	meta.send.resolver = sendResolver{"meta.example.test": {"169.254.169.254"}, "any.example.test": {"0.0.0.0"}, "multi.example.test": {"239.1.1.1"}}
	for _, host := range []string{"meta.example.test", "any.example.test", "multi.example.test"} {
		forceAllow(meta, host+":80", levelLocal) // as if the name had led somewhere else when it was allowed
		info, err := meta.CheckTarget("http://" + host + "/")
		if err != nil || info.Refused == "" {
			t.Errorf("%s: %+v %v", host, info, err)
		}
		if _, err := meta.SendRequest(SendInput{Method: "GET", URL: "http://" + host + "/"}); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("%s: %v", host, err)
		}
	}

	// Headless: this machine is refused, then allowed for a host on the test list.
	hl := &App{}
	hl.send.headless = true
	forceAllow(hl, key, levelLocal)
	if info, _ := hl.CheckTarget(srv.URL); info.Refused == "" {
		t.Errorf("headless, no list: %+v", info)
	}
	if _, err := hl.SendRequest(SendInput{Method: "GET", URL: srv.URL}); err == nil || hits.Load() != 0 {
		t.Fatalf("headless without the test list: %v, %d hit(s)", err, hits.Load())
	}
	hl.send.testAllowLocal = parseTestAllowLocal(true, "Other.Test, "+h)
	if info, _ := hl.CheckTarget(srv.URL); info.Refused != "" {
		t.Errorf("headless with the list: %+v", info)
	}
	if _, err := hl.SendRequest(SendInput{Method: "GET", URL: srv.URL}); err != nil || hits.Load() != 1 {
		t.Fatalf("headless with the list: %v", err)
	}
	// The list is read only by a headless build.
	if got := parseTestAllowLocal(false, h); got != nil {
		t.Errorf("a native build read the test list: %v", got)
	}
	if got := parseTestAllowLocal(true, ""); got != nil {
		t.Errorf("an empty list: %v", got)
	}
}

func TestSendRateAndOneAtATime(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	h, p := hostPort(srv)
	a := &App{}
	mustAllow(t, a, "http://"+h+":"+p+"/")
	now := time.Unix(1_000_000, 0)
	a.send.now = func() time.Time { return now }
	for i := 0; i < sendMaxPerMinute; i++ {
		if _, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL}); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL}); !errors.Is(err, errSendRate) {
		t.Fatalf("the eleventh call: %v", err)
	}
	if hits.Load() != int64(sendMaxPerMinute) {
		t.Fatalf("%d hits", hits.Load())
	}
	// A refused request (an unallowed host) does not count against the limit.
	now = now.Add(61 * time.Second)
	for i := 0; i < 3; i++ {
		_, _ = a.SendRequest(SendInput{Method: "GET", URL: "http://not-allowed.test/"})
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL}); err != nil {
		t.Fatalf("after a minute: %v", err)
	}

	// One at a time: a second call while one waits is refused, and a call after it ends is not.
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseNow() // runs before slow.Close, so a failed assertion below cannot leave the handler waiting
	sh, sp := hostPort(slow)
	mustAllow(t, a, "http://"+sh+":"+sp+"/")
	a.send.now = nil
	a.send.recent = nil
	done := make(chan error, 1)
	go func() { _, err := a.SendRequest(SendInput{Method: "GET", URL: slow.URL}); done <- err }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.send.mu.Lock()
		busy := a.send.inflight
		a.send.mu.Unlock()
		if busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the slow request never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL}); !errors.Is(err, errSendBusy) {
		t.Fatalf("a second request while one runs: %v", err)
	}
	releaseNow()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the slow request did not finish within 5 s after it was released")
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL}); err != nil {
		t.Fatalf("after it ended: %v", err)
	}
}

// A redirect to another origin is shown and not followed; one on the same origin is followed and listed.
func TestSendRedirects(t *testing.T) {
	var elsewhere atomic.Int64
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, "/b?token="+netPlain, http.StatusFound)
		case "/b":
			_, _ = w.Write([]byte("end"))
		case "/away":
			http.Redirect(w, r, other.URL+"/steal?token="+netPlain, http.StatusFound)
		}
	}))
	defer srv.Close()
	h, p := hostPort(srv)
	a := &App{}
	mustAllow(t, a, "http://"+h+":"+p+"/")
	res, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/a"})
	if err != nil || string(res.ResponseBody.Text) != "end" || len(res.Hops) != 1 || res.Hops[0].Status != 302 || res.Stopped != nil {
		t.Fatalf("same origin: %+v %v", res, err)
	}
	mustNotContain(t, "hops", asJSON(res), netPlain)
	res, err = a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/away"})
	oh, op := hostPort(other)
	if err != nil || res.Stopped == nil || res.Status != 302 || res.StoppedHost != oh+":"+op || len(res.Hops) != 0 {
		t.Fatalf("another origin: %+v %v", res, err)
	}
	mustNotContain(t, "the stopped redirect", asJSON(res), netPlain)
	if elsewhere.Load() != 0 {
		t.Fatalf("a redirect to another origin was followed")
	}
	// The next call to that host needs its own allow.
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: other.URL}); !errors.Is(err, errSendHostNotAllowed) {
		t.Fatalf("the redirect target was allowed by the redirect: %v", err)
	}
}

func TestSendResponseIsCappedAndHostileTextStaysText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(strings.Repeat("x ", 3<<20)))
		case "/bin":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\x00\x00"))
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>x</script><input type="password" value="` + netPlain + `"><meta name="csrf-token" content="` + netPlain + `">`))
		}
	}))
	defer srv.Close()
	h, p := hostPort(srv)
	a := &App{}
	mustAllow(t, a, "http://"+h+":"+p+"/")
	big, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/big"})
	if err != nil || !big.Truncated || !big.ResponseBody.Truncated || len([]rune(big.ResponseBody.Text)) > netBodyShown {
		t.Fatalf("big: %v truncated %v shown %d", err, big.Truncated, len([]rune(big.ResponseBody.Text)))
	}
	bin, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/bin"})
	if err != nil || bin.ResponseBody.Kind != "binary" || bin.ResponseBody.Text != "" {
		t.Fatalf("binary: %+v %v", bin.ResponseBody, err)
	}
	html, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/html"})
	if err != nil || strings.Contains(html.ResponseBody.Text, netPlain) || !strings.Contains(html.ResponseBody.Text, "<script>x</script>") {
		t.Fatalf("html: %q %v", html.ResponseBody.Text, err)
	}
}

func TestSendTimeoutIsASentenceWithNoAddress(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) }))
	defer slow.Close()
	h, p := hostPort(slow)
	a := &App{}
	mustAllow(t, a, "http://"+h+":"+p+"/")
	// A refused connection is a short sentence.
	mustAllow(t, a, "http://127.0.0.1:1/")
	_, err := a.SendRequest(SendInput{Method: "GET", URL: "http://127.0.0.1:1/secret-path?token=" + netPlain})
	if err == nil {
		t.Fatal("a refused connection succeeded")
	}
	mustNotContain(t, "the error", err.Error(), "secret-path", netPlain, "127.0.0.1")
}

// CheckTarget reads an address with the same rules as the call: only http and https, no user info, a host.
func TestCheckTargetRefusesWhatACallWould(t *testing.T) {
	a := &App{}
	for _, raw := range []string{"file:///etc/passwd", "ftp://h.example.test/", "http://u:pw@h.example.test/", "http://u@h.example.test/", "//h.example.test/", "http:///x", "", "h.example.test", "javascript:alert(1)", "http://" + strings.Repeat("a", 9000)} {
		if info, err := a.CheckTarget(raw); err == nil {
			t.Errorf("%q: %+v", raw, info)
		}
	}
	a.send.resolver = sendResolver{"ok.example.test": {"93.184.216.34"}}
	info, err := a.CheckTarget("https://OK.example.test/x")
	if err != nil || info.Host != "ok.example.test" || info.Port != "443" || info.Local || info.Classes[0] != "the internet" {
		t.Errorf("%+v %v", info, err)
	}
	if info, _ = a.CheckTarget("http://ok.example.test:8080/"); info.Port != "8080" {
		t.Errorf("port %q", info.Port)
	}
	if info, err = a.CheckTarget("http://nowhere.example.test/"); err != nil || info.Refused == "" || len(info.Addresses) != 0 {
		t.Errorf("a name that does not resolve: %+v %v", info, err)
	}
}

func TestTestSendRateIsHeadlessOnlyAndBounded(t *testing.T) {
	for _, c := range []struct {
		headless bool
		env      string
		want     int
	}{{true, "50", 50}, {true, " 1000 ", 1000}, {false, "50", 0}, {true, "0", 0}, {true, "1001", 0}, {true, "-3", 0}, {true, "x", 0}, {true, "", 0}} {
		if got := parseTestSendRate(c.headless, c.env); got != c.want {
			t.Errorf("parseTestSendRate(%v, %q) = %d, want %d", c.headless, c.env, got, c.want)
		}
	}
	st := sendState{maxPerMinute: 2, now: time.Now}
	for i := 0; i < 2; i++ {
		if err := st.begin(); err != nil {
			t.Fatal(err)
		}
		st.end()
	}
	if err := st.begin(); err != errSendRate {
		t.Fatalf("third call = %v, want the rate error", err)
	}
}

// The policy that httpcall asks is the decision, not the window: a host and port that were not allowed are refused when called through
// it directly, an allow is for the kind that was shown, and the kinds that are never allowed are refused whatever was allowed.
func TestThePolicyChecksTheAllowAndTheClass(t *testing.T) {
	st := &sendState{}
	pub := net.ParseIP("93.184.216.34")
	if err := st.policy("example.test", 443, pub); err == nil || st.takeRefusal() == "" {
		t.Fatalf("an unallowed host: %v", err)
	}
	st.note("example.test:443", levelInternet)
	if err := st.allow("example.test:443"); err != nil {
		t.Fatal(err)
	}
	if err := st.policy("example.test", 443, pub); err != nil {
		t.Fatalf("an allowed host: %v", err)
	}
	if err := st.policy("example.test", 8443, pub); err == nil {
		t.Fatal("another port of an allowed host was allowed")
	}
	// An allow for the internet does not cover this machine, a private network or a reserved range.
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "198.18.0.1", "240.0.0.1", "64:ff9b::7f00:1", "::1"} {
		err := st.policy("example.test", 443, net.ParseIP(ip))
		if err == nil || !strings.Contains(st.takeRefusal(), "allowed as the internet") {
			t.Errorf("%s was covered by an allow for the internet: %v", ip, err)
		}
	}
	// An allow for local addresses covers them, and still not the kinds that are never allowed.
	st.note("lan.test:80", levelLocal)
	if err := st.allow("lan.test:80"); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "198.18.0.1", "93.184.216.34"} {
		if err := st.policy("lan.test", 80, net.ParseIP(ip)); err != nil {
			t.Errorf("%s under a local allow: %v", ip, err)
		}
	}
	for _, ip := range []string{"169.254.169.254", "64:ff9b::a9fe:a9fe", "0.0.0.0", "239.1.1.1", "224.0.0.1", "ff02::1", "2002:a9fe:a9fe::1"} {
		if err := st.policy("lan.test", 80, net.ParseIP(ip)); err == nil || !strings.Contains(st.takeRefusal(), "never allowed") {
			t.Errorf("%s was allowed: %v", ip, err)
		}
	}
	// A host whose check found an address that is never allowed cannot be allowed, and one that was not checked cannot either.
	st.note("meta.test:80", levelNever)
	if err := st.allow("meta.test:80"); err == nil {
		t.Error("a host that leads to a never-allowed address was allowed")
	}
	if err := st.allow("unchecked.test:80"); !errors.Is(err, errSendNotChecked) {
		t.Errorf("an unchecked host: %v", err)
	}
	// The headless build refuses local and reserved kinds unless the host is on the test list.
	hl := &sendState{headless: true}
	hl.note("example.test:443", levelLocal)
	_ = hl.allow("example.test:443")
	for _, ip := range []string{"198.18.0.1", "10.0.0.1", "64:ff9b::7f00:1"} {
		if err := hl.policy("example.test", 443, net.ParseIP(ip)); err == nil {
			t.Errorf("%s headless was allowed", ip)
		}
	}
}

// DNS rebinding after the yes: a name that was allowed while it led to the internet and leads to this machine afterwards is refused, the
// person is asked again at the stronger level, and no test here connects to anything but a fake on loopback.
func TestAnAllowIsForTheKindThatWasShown(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	_, p := hostPort(srv)
	res := sendResolver{"flip.test": {"93.184.216.34"}}
	a := &App{}
	a.send.resolver = res
	a.send.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+p) // the fake answers whatever address was checked
	}
	url := "http://flip.test:" + p + "/"
	info, err := a.CheckTarget(url)
	if err != nil || info.Local || info.Allowed {
		t.Fatalf("%+v %v", info, err)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: url}); err != nil || hits.Load() != 1 {
		t.Fatalf("while it leads to the internet: %v, %d hit(s)", err, hits.Load())
	}
	// The name now leads to this machine.
	res["flip.test"] = []string{"127.0.0.1"}
	_, err = a.SendRequest(SendInput{Method: "GET", URL: url})
	if err == nil || !strings.Contains(err.Error(), "allowed as the internet") || strings.Contains(err.Error(), "127.0.0.1") || hits.Load() != 1 {
		t.Fatalf("after the flip: %v, %d hit(s)", err, hits.Load())
	}
	// The check shows it again and it is not allowed yet; the person allows it at the stronger level and then it goes.
	info, _ = a.CheckTarget(url)
	if !info.Local || info.Allowed {
		t.Fatalf("the check after the flip: %+v", info)
	}
	if err := a.AllowHost(info.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: url}); err != nil || hits.Load() != 2 {
		t.Fatalf("after the stronger allow: %v, %d hit(s)", err, hits.Load())
	}
	// A change to a kind that is never allowed is refused whatever was allowed.
	res["flip.test"] = []string{"169.254.169.254"}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: url}); err == nil || !strings.Contains(err.Error(), "never allowed") || hits.Load() != 2 {
		t.Fatalf("after a flip to metadata: %v, %d hit(s)", err, hits.Load())
	}
	// And back to the internet is covered by the stronger allow.
	res["flip.test"] = []string{"93.184.216.34"}
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: url}); err != nil || hits.Load() != 3 {
		t.Fatalf("back to the internet: %v, %d hit(s)", err, hits.Load())
	}
}

// A refusal by address kind says which kind, and a response that ends early says so and shows nothing of it.
func TestSendSaysTheClassAndRefusesAnIncompleteResponse(t *testing.T) {
	a := &App{}
	a.send.resolver = sendResolver{"meta.example.test": {"169.254.169.254"}, "nat.example.test": {"64:ff9b::a9fe:a9fe"}}
	for _, host := range []string{"meta.example.test", "nat.example.test"} {
		forceAllow(a, host+":80", levelLocal)
		_, err := a.SendRequest(SendInput{Method: "GET", URL: "http://" + host + "/"})
		if err == nil || !strings.Contains(err.Error(), "a cloud metadata address") || strings.Contains(err.Error(), "169.254") {
			t.Errorf("%s: %v", host, err)
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				buf := make([]byte, 4096)
				_, _ = c.Read(buf)
				_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 500\r\n\r\n{\"password\":\"x\""))
			}()
		}
	}()
	b := &App{}
	b.send.maxPerMinute = 100
	mustAllow(t, b, "http://"+ln.Addr().String()+"/")
	res, err := b.SendRequest(SendInput{Method: "GET", URL: "http://" + ln.Addr().String() + "/"})
	if err == nil || !strings.Contains(err.Error(), "none of it is shown") || res.ResponseBody.Text != "" || res.Status != 0 {
		t.Fatalf("%+v %v", res, err)
	}
}

// A host written with a trailing dot is the same host, for the allow and for the call.
func TestAHostWithATrailingDotIsTheSameHost(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	h, p := hostPort(srv)
	a := &App{}
	a.send.resolver = sendResolver{"app.example.test": {h}}
	mustAllow(t, a, "http://app.example.test:"+p+"/")
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: "http://App.Example.Test.:" + p + "/"}); err != nil || hits.Load() != 1 {
		t.Fatalf("%v hits %d", err, hits.Load())
	}
}

// What was typed is replaced wherever the answer echoes it: a value under a name the redactor does not know, with no shape of its own,
// in the body, a header, the address and a redirect, in another case, split by hidden characters, and percent-encoded.
func TestWhatWasTypedIsNotEchoedBack(t *testing.T) {
	const (
		apiKey = "plainvalueqq1234"
		auth   = "plainauthvalue5678"
		bodyTk = "bodysecret9z8y7x6w"
		qv     = "urlquery4321abcd"
		quirky = "weird value/with+chars9" // typed under a credential name, with characters a URL escapes
	)
	soft := func(s string) string { return strings.Join(strings.Split(s, ""), "­") }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop2" {
			http.Redirect(w, r, "/echo/"+r.Header.Get("X-Api-Key"), http.StatusFound)
			return
		}
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, "/echo?back="+url.QueryEscape(r.Header.Get("X-Api-Key"))+"&q="+r.URL.Query().Get("q"), http.StatusFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Seen", r.Header.Get("X-Api-Key"))
		w.Header().Set("X-Seen-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Seen-Other", "see "+strings.ToUpper(r.Header.Get("X-Api-Key"))+" end")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("key=" + r.Header.Get("X-Api-Key") + "\nauth=" + r.Header.Get("Authorization") + "\nbody=" + string(b) + "\nq=" + r.URL.Query().Get("q") +
			"\nupper=" + strings.ToUpper(apiKey) + "\nsoft=" + soft(apiKey) + "\nsoft2=" + soft(auth) + "\nenc=" + url.QueryEscape(quirky) + "\nplain=" + quirky + "\nkept=hello"))
	}))
	defer srv.Close()
	h, p := hostPort(srv)
	a := &App{}
	mustAllow(t, a, "http://"+h+":"+p+"/")
	in := SendInput{Method: "POST", URL: srv.URL + "/echo?q=" + qv, Body: `{"note":"` + bodyTk + `","password":"` + quirky + `","x":1}`, Headers: []SendHeader{
		{Name: "X-Api-Key", Value: apiKey}, {Name: "Authorization", Value: "Token " + auth}, {Name: "X-Note", Value: "hello"}}}
	res, err := a.SendRequest(in)
	if err != nil {
		t.Fatal(err)
	}
	all := asJSON(res)
	mustNotContain(t, "the result", all, apiKey, strings.ToUpper(apiKey), auth, bodyTk, qv, quirky, url.QueryEscape(quirky), "weird value", "plainvalue", "plainauth", "bodysecret", "urlquery")
	if !strings.Contains(res.ResponseBody.Text, "kept=hello") {
		t.Errorf("a value that was not a credential was hidden too: %q", res.ResponseBody.Text)
	}
	if !strings.Contains(res.ResponseBody.Text, "soft=[redacted]") || !strings.Contains(res.ResponseBody.Text, "upper=[redacted]") {
		t.Errorf("body %q", res.ResponseBody.Text)
	}
	// A typed value under a name the redactor does not know is hidden in the request table too.
	res3, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/echo", Headers: []SendHeader{{Name: "X-Trace", Value: "trace1234abcd"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res3.RequestHeaders {
		if h.Name == "X-Trace" && h.Value != redactedMark {
			t.Errorf("a typed value that looks like a credential is shown in the request table: %q", h.Value)
		}
	}
	// The address and the redirect carry it too: a value in a path segment survives the URL redactor, which hides only query values.
	res4, err := a.SendRequest(SendInput{Method: "GET", URL: srv.URL + "/hop2", Headers: []SendHeader{{Name: "X-Api-Key", Value: apiKey}}})
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, "the path redirect", asJSON(res4), apiKey, "plainvalue")
	if len(res4.Hops) != 1 || !strings.Contains(res4.URL, "[redacted]") {
		t.Errorf("hops %+v url %q", res4.Hops, res4.URL)
	}
	// The address and the redirect carry it too.
	in2 := SendInput{Method: "GET", URL: srv.URL + "/hop?q=" + qv, Headers: []SendHeader{{Name: "X-Api-Key", Value: apiKey}}}
	res2, err := a.SendRequest(in2)
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, "the redirected result", asJSON(res2), apiKey, qv, "plainvalue", "urlquery")
	if len(res2.Hops) != 1 {
		t.Errorf("hops %+v", res2.Hops)
	}
}

// The values to hide are chosen narrowly enough that an ordinary value stays readable.
func TestTypedValuesAreChosenNarrowly(t *testing.T) {
	tv := collectTyped("https://api.example.test/v1/items/abc?page=2&sort=name&token=tok-valueXYZ9", map[string]string{
		"Accept": "application/json", "Authorization": "Bearer abc123def456", "X-Request-Id": "req-7", "X-Api-Key": "k99"}, `{"name":"Jane","pin":"4821","note":"hello world","secretish":"s3cr3tValue1"}`)
	for _, want := range []string{"abc123def456", "tok-valueXYZ9", "4821", "s3cr3tValue1", "k99"} {
		if !tv.items[want] {
			t.Errorf("%q is not remembered: %v", want, tv.items)
		}
	}
	for _, not := range []string{"application/json", "Bearer", "req-7", "2", "name", "hello world", "Jane", "abc"} {
		if tv.items[not] {
			t.Errorf("%q is remembered", not)
		}
	}
	// Whole values from a JSON body (spaces and all), the forms a server may echo, text bodies, and short values.
	more := collectTyped("https://x.test/", map[string]string{"X-Api-Key": "k9"}, `{"pass":"two words here","deep":{"api_key":12345678}}`)
	if !more.items["two words here"] || !more.items["two%20words%20here"] || !more.items["two+words+here"] || !more.items["12345678"] || more.items["k9"] {
		t.Errorf("%v", more.items)
	}
	text := collectTyped("https://x.test/", nil, "please use abcd1234efgh now, or the form a=b")
	if !text.items["abcd1234efgh"] || text.items["please"] || text.items["now"] {
		t.Errorf("%v", text.items)
	}
	form := collectTyped("https://x.test/", nil, "user=jane&password=s3cretValue1&next=%2Fhome")
	if !form.items["s3cretValue1"] || form.items["jane"] {
		t.Errorf("%v", form.items)
	}
	form2 := collectTyped("https://x.test/reset/tok4567abcd/x", nil, "password=two+words+here&pin=%2F12")
	if !form2.items["two words here"] || !form2.items["tok4567abcd"] || form2.items["reset"] {
		t.Errorf("%v", form2.items)
	}
	if got := tv.scrub("Bearer ABC123DEF456 and k99 but not k98 and application/json"); got != "[redacted] and [redacted] but not k98 and application/json" {
		t.Errorf("scrub = %q", got)
	}
}

// A key is read back only in the normal form: a host in ASCII, lower case, no trailing dot, a number from 1 to 65535 with no padding.
func TestParseKeyOnlyTakesTheNormalForm(t *testing.T) {
	for _, k := range []string{"example.test:80", "[::1]:8080", "[2001:db8::1]:65535", "xn--bcher-kva.test:443", "127.0.0.1:1"} {
		if _, _, err := parseKey(k); err != nil {
			t.Errorf("%q: %v", k, err)
		}
	}
	for _, k := range []string{"example.test:0", "example.test:65536", "example.test:-1", "example.test:080", "example.test:+80", "Example.test:80", "example.test.:80",
		"b\u00fccher.test:443", "::1:80", "[::1]", "[::1]:", "[fe80::1%25eth0]:80", "a@b:80", "example.test", ":80", ""} {
		if _, _, err := parseKey(k); err == nil {
			t.Errorf("%q was accepted", k)
		}
	}
}
