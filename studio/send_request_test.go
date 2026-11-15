package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAllowHostRefusesNonsense(t *testing.T) {
	a := &App{}
	for _, k := range []string{"", "host", ":80", "host:", "a/b:80", "u@h:80", "h :80", "h:80\n"} {
		if err := a.AllowHost(k); err == nil {
			t.Errorf("%q was accepted", k)
		}
	}
	if err := a.AllowHost("Example.TEST:443"); err != nil {
		t.Fatal(err)
	}
	if info, _ := a.CheckTarget("https://example.test/x"); !info.Allowed {
		t.Error("host names are matched without case")
	}
}

// The request is checked as the engine's function checks it, and the answers are fixed sentences.
func TestSendRequestRefusalsAreFixedSentences(t *testing.T) {
	var hits atomic.Int64
	srv := backend(t, &hits)
	a := &App{}
	h, p := hostPort(srv)
	_ = a.AllowHost(h + ":" + p)
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
		_ = meta.AllowHost(host + ":80")
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
	_ = hl.AllowHost(key)
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
	_ = a.AllowHost(h + ":" + p)
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
	sh, sp := hostPort(slow)
	_ = a.AllowHost(sh + ":" + sp)
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
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
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
	_ = a.AllowHost(h + ":" + p)
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
	_ = a.AllowHost(h + ":" + p)
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
	_ = a.AllowHost(h + ":" + p)
	// A refused connection is a short sentence.
	_ = a.AllowHost("127.0.0.1:1")
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
// it directly, and the class rules are those of CheckTarget.
func TestThePolicyChecksTheAllowAndTheClass(t *testing.T) {
	st := &sendState{}
	pub := net.ParseIP("93.184.216.34")
	if err := st.policy("example.test", 443, pub); !errors.Is(err, errSendHostNotAllowed) {
		t.Fatalf("an unallowed host: %v", err)
	}
	st.allow("example.test:443")
	if err := st.policy("example.test", 443, pub); err != nil {
		t.Fatalf("an allowed host: %v", err)
	}
	if err := st.policy("example.test", 8443, pub); !errors.Is(err, errSendHostNotAllowed) {
		t.Fatalf("another port of an allowed host: %v", err)
	}
	// Never classes, whatever was allowed; reserved and local ones are allowed natively and refused headless.
	for _, ip := range []string{"169.254.169.254", "64:ff9b::a9fe:a9fe", "0.0.0.0", "239.1.1.1", "2002:a9fe:a9fe::1"} {
		if err := st.policy("example.test", 443, net.ParseIP(ip)); err == nil {
			t.Errorf("%s was allowed", ip)
		}
	}
	for _, ip := range []string{"198.18.0.1", "240.0.0.1", "10.0.0.1", "64:ff9b::7f00:1"} {
		if err := st.policy("example.test", 443, net.ParseIP(ip)); err != nil {
			t.Errorf("%s natively: %v", ip, err)
		}
	}
	hl := &sendState{headless: true}
	hl.allow("example.test:443")
	for _, ip := range []string{"198.18.0.1", "10.0.0.1", "64:ff9b::7f00:1"} {
		if err := hl.policy("example.test", 443, net.ParseIP(ip)); err == nil {
			t.Errorf("%s headless was allowed", ip)
		}
	}
}

// A refusal by address kind says which kind, and a response that ends early says so and shows nothing of it.
func TestSendSaysTheClassAndRefusesAnIncompleteResponse(t *testing.T) {
	a := &App{}
	a.send.resolver = sendResolver{"meta.example.test": {"169.254.169.254"}, "nat.example.test": {"64:ff9b::a9fe:a9fe"}}
	for _, host := range []string{"meta.example.test", "nat.example.test"} {
		_ = a.AllowHost(host + ":80")
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
	_ = b.AllowHost(ln.Addr().String())
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
	_ = a.AllowHost("app.example.test:" + p)
	if _, err := a.SendRequest(SendInput{Method: "GET", URL: "http://App.Example.Test.:" + p + "/"}); err != nil || hits.Load() != 1 {
		t.Fatalf("%v hits %d", err, hits.Load())
	}
}
