package httpcall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// echo answers with what it got, so a test can see what was sent.
func echo(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Auth", r.Header.Get("Authorization"))
		w.Header().Set("X-Type", r.Header.Get("Content-Type"))
		_, _ = w.Write([]byte("body:" + string(b)))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestDoSendsAndReceives(t *testing.T) {
	srv, hits := echo(t)
	res, err := Do(context.Background(), Request{Method: "post", URL: srv.URL + "/p?a=1", Header: map[string]string{"Authorization": "Bearer x", "Content-Type": "application/json"}, Body: `{"a":1}`}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || string(res.Body) != `body:{"a":1}` || res.Header.Get("X-Method") != "POST" || res.Header.Get("X-Auth") != "Bearer x" || res.Header.Get("X-Type") != "application/json" || hits.Load() != 1 {
		t.Fatalf("%+v", res)
	}
	if res.RemoteIP != "127.0.0.1" || res.Truncated || res.Stopped != nil || len(res.Hops) != 0 || !strings.HasSuffix(res.URL, "/p?a=1") {
		t.Fatalf("%+v", res)
	}
	if head, err := Do(context.Background(), Request{Method: "HEAD", URL: srv.URL}, Options{}); err != nil || len(head.Body) != 0 || head.Status != 200 {
		t.Fatalf("HEAD: %+v %v", head, err)
	}
}

// The request is checked before anything is dialed: the scheme, the address, the method, the headers and the sizes.
func TestDoRefusesARequestBeforeDialing(t *testing.T) {
	srv, hits := echo(t)
	long := srv.URL + "/" + strings.Repeat("a", MaxURLBytes)
	manyHeaders := map[string]string{}
	for i := 0; i <= MaxHeaders; i++ {
		manyHeaders[fmt.Sprintf("X-H-%d", i)] = "v"
	}
	cases := []struct {
		name string
		req  Request
		want error
	}{
		{"file", Request{Method: "GET", URL: "file:///etc/passwd"}, ErrScheme},
		{"ftp", Request{Method: "GET", URL: "ftp://example.test/x"}, ErrScheme},
		{"gopher", Request{Method: "GET", URL: "gopher://example.test/x"}, ErrScheme},
		{"javascript", Request{Method: "GET", URL: "javascript:alert(1)"}, ErrScheme},
		{"data", Request{Method: "GET", URL: "data:text/plain,hi"}, ErrScheme},
		{"no scheme", Request{Method: "GET", URL: "example.test/x"}, ErrURL},
		{"scheme relative", Request{Method: "GET", URL: "//example.test/x"}, ErrURL},
		{"empty", Request{Method: "GET", URL: ""}, ErrURL},
		{"no host", Request{Method: "GET", URL: "http:///x"}, ErrURL},
		{"opaque", Request{Method: "GET", URL: "http:example.test"}, ErrURL},
		{"user info", Request{Method: "GET", URL: "http://user:pw@example.test/"}, ErrUserInfo},
		{"user name only", Request{Method: "GET", URL: "http://user@example.test/"}, ErrUserInfo},
		{"a space", Request{Method: "GET", URL: "http://example.test/a b"}, ErrURL},
		{"a line break", Request{Method: "GET", URL: "http://example.test/a\r\nHost: x"}, ErrURL},
		{"too long", Request{Method: "GET", URL: long}, ErrURL},
		{"CONNECT", Request{Method: "CONNECT", URL: srv.URL}, ErrMethod},
		{"TRACE", Request{Method: "TRACE", URL: srv.URL}, ErrMethod},
		{"a custom method", Request{Method: "FOO", URL: srv.URL}, ErrMethod},
		{"no method", Request{Method: "", URL: srv.URL}, ErrMethod},
		{"a Host header", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"host": "evil.test"}}, ErrHeader},
		{"Content-Length", Request{Method: "POST", URL: srv.URL, Header: map[string]string{"Content-Length": "5"}}, ErrHeader},
		{"Transfer-Encoding", Request{Method: "POST", URL: srv.URL, Header: map[string]string{"Transfer-Encoding": "chunked"}}, ErrHeader},
		{"Connection", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"Connection": "upgrade"}}, ErrHeader},
		{"Proxy-Authorization", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"Proxy-Authorization": "x"}}, ErrHeader},
		{"a name with a space", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"A B": "x"}}, ErrHeader},
		{"a name with a colon", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"A:B": "x"}}, ErrHeader},
		{"an empty name", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"": "x"}}, ErrHeader},
		{"a CRLF in a value", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"X-A": "a\r\nX-B: b"}}, ErrHeader},
		{"a NUL in a value", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"X-A": "a\x00b"}}, ErrHeader},
		{"a long value", Request{Method: "GET", URL: srv.URL, Header: map[string]string{"X-A": strings.Repeat("a", MaxHeaderBytes+1)}}, ErrHeader},
		{"too many headers", Request{Method: "GET", URL: srv.URL, Header: manyHeaders}, ErrHeader},
		{"a body over the limit", Request{Method: "POST", URL: srv.URL, Body: strings.Repeat("a", DefaultMaxRequest+1)}, ErrRequestTooLarge},
		{"a body with GET", Request{Method: "GET", URL: srv.URL, Body: "x"}, ErrBodyNotAllowed},
		{"a body with HEAD", Request{Method: "HEAD", URL: srv.URL, Body: "x"}, ErrBodyNotAllowed},
	}
	for _, c := range cases {
		if _, err := Do(context.Background(), c.req, Options{}); !errors.Is(err, c.want) {
			t.Errorf("%s: error = %v, want %v", c.name, err, c.want)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("a refused request reached the server %d time(s)", hits.Load())
	}
	// A body of exactly the limit is sent.
	if _, err := Do(context.Background(), Request{Method: "POST", URL: srv.URL, Body: strings.Repeat("a", DefaultMaxRequest)}, Options{}); err != nil {
		t.Errorf("a body of exactly the limit: %v", err)
	}
	// The fragment is not sent.
	if res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/x#secret"}, Options{}); err != nil || strings.Contains(res.URL, "secret") {
		t.Errorf("fragment: %v %q", err, res.URL)
	}
}

func TestResponseIsCappedAndMarked(t *testing.T) {
	for _, c := range []struct {
		size      int
		limit     int64
		truncated bool
	}{{1000, 1000, false}, {1001, 1000, true}, {999, 1000, false}, {5 << 20, 0, true}, {1 << 20, 0, false}, {1<<20 + 1, 0, true}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("a", c.size)))
		}))
		res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{MaxResponseBytes: c.limit})
		srv.Close()
		want := c.limit
		if want == 0 {
			want = DefaultMaxResponse
		}
		if err != nil || res.Truncated != c.truncated || int64(len(res.Body)) > want || (!c.truncated && len(res.Body) != c.size) {
			t.Errorf("size %d limit %d: %d bytes, truncated %v, %v", c.size, c.limit, len(res.Body), res.Truncated, err)
		}
	}
}

// A server that never stops writing cannot fill memory or hold the call past its time.
func TestEndlessBodyIsStoppedAndATimeoutHolds(t *testing.T) {
	var written atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := []byte(strings.Repeat("x", 64<<10))
		for i := 0; i < 2048; i++ {
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()
	start := time.Now()
	res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{})
	if err != nil || !res.Truncated || len(res.Body) != DefaultMaxResponse || time.Since(start) > 5*time.Second {
		t.Fatalf("%d bytes, truncated %v, %v, %v", len(res.Body), res.Truncated, err, time.Since(start))
	}
	time.Sleep(200 * time.Millisecond)
	if written.Load() > 32<<20 {
		t.Fatalf("the server wrote %d MB before the client stopped", written.Load()>>20)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(3 * time.Second) }))
	defer slow.Close()
	start = time.Now()
	_, err = Do(context.Background(), Request{Method: "GET", URL: slow.URL}, Options{Timeout: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "time limit") || time.Since(start) > 2*time.Second {
		t.Fatalf("timeout: %v after %v", err, time.Since(start))
	}
}

func TestOptionsAreBounded(t *testing.T) {
	o := Options{Timeout: time.Hour, MaxResponseBytes: 1 << 40, MaxRedirects: 99}.normalized()
	if o.Timeout != MaxTimeout || o.MaxResponseBytes != MaxMaxResponse || o.MaxRedirects != MaxRedirects {
		t.Errorf("%+v", o)
	}
	d := Options{}.normalized()
	if d.Timeout != DefaultTimeout || d.MaxResponseBytes != DefaultMaxResponse || d.MaxRequestBytes != DefaultMaxRequest || d.MaxRedirects != DefaultRedirects || d.Resolver == nil {
		t.Errorf("%+v", d)
	}
	if DefaultTimeout != 30*time.Second || MaxTimeout != 120*time.Second || DefaultMaxResponse != 1048576 || DefaultMaxRequest != 262144 || DefaultRedirects != 5 || MaxHeaders != 50 {
		t.Error("a limit changed: it is a decision, not an accident")
	}
}

func TestRedirects(t *testing.T) {
	var other atomic.Int64
	otherSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { other.Add(1) }))
	defer otherSrv.Close()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/mid", http.StatusFound)
		case "/mid":
			http.Redirect(w, r, "/end", http.StatusMovedPermanently)
		case "/end":
			_, _ = w.Write([]byte("end"))
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/count":
			n := r.URL.Query().Get("n")
			http.Redirect(w, r, "/count?n="+n+"x", http.StatusFound)
		case "/away":
			http.Redirect(w, r, otherSrv.URL+"/x", http.StatusFound)
		case "/scheme":
			http.Redirect(w, r, "https://"+r.Host+"/x", http.StatusFound)
		case "/file":
			w.Header().Set("Location", "file:///etc/passwd")
			w.WriteHeader(http.StatusFound)
		case "/userinfo":
			w.Header().Set("Location", "http://u:p@"+r.Host+"/x")
			w.WriteHeader(http.StatusFound)
		case "/post307":
			b, _ := io.ReadAll(r.Body)
			if r.Method == "POST" && string(b) == "keep" && r.URL.Query().Get("again") == "" {
				http.Redirect(w, r, "/post307?again=1", http.StatusTemporaryRedirect)
				return
			}
			_, _ = w.Write([]byte(r.Method + ":" + string(b)))
		}
	}))
	defer srv.Close()

	res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/start"}, Options{})
	if err != nil || string(res.Body) != "end" || len(res.Hops) != 2 || res.Hops[0].Status != 302 || res.Hops[1].Status != 301 || res.Stopped != nil || !strings.HasSuffix(res.URL, "/end") {
		t.Fatalf("same origin: %+v %v", res, err)
	}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/loop"}, Options{}); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("loop: %v", err)
	}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/count?n=x"}, Options{MaxRedirects: 3}); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("a chain longer than the limit: %v", err)
	}
	// Another origin (another port, another scheme) is reported and never called.
	for _, path := range []string{"/away", "/scheme"} {
		res, err = Do(context.Background(), Request{Method: "GET", URL: srv.URL + path}, Options{})
		if err != nil || res.Stopped == nil || res.Status != 302 || res.Stopped.Status != 302 || !strings.HasPrefix(res.Stopped.To, map[string]string{"/away": otherSrv.URL, "/scheme": "https://"}[path]) || len(res.Hops) != 0 {
			t.Fatalf("%s: %+v %v", path, res, err)
		}
	}
	if other.Load() != 0 {
		t.Fatalf("a redirect to another origin was followed: %d call(s)", other.Load())
	}
	for _, path := range []string{"/file", "/userinfo"} {
		if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + path}, Options{}); err == nil {
			t.Errorf("%s: the redirect was followed", path)
		}
	}
	res, err = Do(context.Background(), Request{Method: "POST", URL: srv.URL + "/post307", Body: "keep"}, Options{})
	if err != nil || string(res.Body) != "POST:keep" {
		t.Fatalf("307 keeps the method and the body: %q %v", res.Body, err)
	}
}

type stubResolver struct {
	answers map[string][]net.IP
	calls   atomic.Int64
}

func (s *stubResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	s.calls.Add(1)
	var out []net.IPAddr
	for _, ip := range s.answers[host] {
		out = append(out, net.IPAddr{IP: ip})
	}
	if len(out) == 0 {
		return nil, errors.New("no such host")
	}
	return out, nil
}

// The policy is asked about the address that is dialed, after the name is resolved, so a name that looks harmless and resolves to this
// machine is refused, and the server behind it is never called.
func TestPolicyIsAskedAboutTheResolvedAddress(t *testing.T) {
	srv, hits := echo(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	noLocal := func(host string, ip net.IP) error {
		if ClassOf(ip).Local() {
			return fmt.Errorf("%s is %s", host, ClassOf(ip))
		}
		return nil
	}
	// An address literal.
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{Policy: noLocal}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("a literal loopback address: %v", err)
	}
	// A name that resolves to loopback (DNS rebinding): refused after the lookup.
	r := &stubResolver{answers: map[string][]net.IP{"rebind.test": {net.ParseIP("127.0.0.1")}}}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: "http://rebind.test:" + port + "/"}, Options{Policy: noLocal, Resolver: r}); !errors.Is(err, ErrPolicy) {
		t.Fatalf("a name that resolves to loopback: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the server was called %d time(s) through a refused address", hits.Load())
	}
	// A name with two answers: the refused one is skipped and the allowed one is dialed, and the name is resolved once.
	r2 := &stubResolver{answers: map[string][]net.IP{"two.test": {net.ParseIP("10.255.255.1"), net.ParseIP("127.0.0.1")}}}
	onlyLoopback := func(host string, ip net.IP) error {
		if ClassOf(ip) != Loopback {
			return errors.New("not loopback")
		}
		return nil
	}
	res, err := Do(context.Background(), Request{Method: "GET", URL: "http://two.test:" + port + "/"}, Options{Policy: onlyLoopback, Resolver: r2})
	if err != nil || res.RemoteIP != "127.0.0.1" || hits.Load() != 1 || r2.calls.Load() != 1 {
		t.Fatalf("two answers: %+v %v hits %d lookups %d", res, err, hits.Load(), r2.calls.Load())
	}
	// A name that does not resolve is an error with no detail.
	if _, err := Do(context.Background(), Request{Method: "GET", URL: "http://nowhere.test/"}, Options{Resolver: &stubResolver{}}); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("unresolved: %v", err)
	}
	// With no policy, every address is allowed.
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{}); err != nil {
		t.Fatalf("no policy: %v", err)
	}
}

// The environment's proxy is never used: the address is what the policy sees.
func TestTheEnvironmentsProxyIsNotUsed(t *testing.T) {
	var proxied atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxied.Add(1) }))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	srv, hits := echo(t)
	// httptest servers are on 127.0.0.1, which Go's proxy rules skip: use a name that resolves there through the stub.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	r := &stubResolver{answers: map[string][]net.IP{"app.example.test": {net.ParseIP("127.0.0.1")}}}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: "http://app.example.test:" + port + "/"}, Options{Resolver: r}); err != nil {
		t.Fatal(err)
	}
	if proxied.Load() != 0 || hits.Load() != 1 {
		t.Fatalf("proxy %d, server %d", proxied.Load(), hits.Load())
	}
}

func TestClassOf(t *testing.T) {
	for ip, want := range map[string]Class{
		"127.0.0.1": Loopback, "127.9.9.9": Loopback, "::1": Loopback, "::ffff:127.0.0.1": Loopback,
		"10.1.2.3": Private, "172.16.0.1": Private, "172.31.255.255": Private, "172.32.0.1": Public, "172.15.255.255": Public,
		"192.168.1.1": Private, "100.64.0.1": Private, "100.127.255.255": Private, "100.128.0.1": Public, "fc00::1": Private, "fd12::1": Private,
		"169.254.169.254": Metadata, "::ffff:169.254.169.254": Metadata, "fd00:ec2::254": Metadata, "169.254.1.1": LinkLocal, "fe80::1": LinkLocal,
		"0.0.0.0": Unspecified, "::": Unspecified, "224.0.0.1": LinkLocal, "239.1.1.1": Multicast, "ff02::1": LinkLocal, "ff05::1": Multicast,
		"8.8.8.8": Public, "1.1.1.1": Public, "2606:4700::1111": Public,
	} {
		if got := ClassOf(net.ParseIP(ip)); got != want {
			t.Errorf("%s is %v, want %v", ip, got, want)
		}
	}
	if Public.Local() || !Loopback.Local() || !Private.Local() || !Metadata.Local() {
		t.Error("Local")
	}
	for c := Public; c <= Multicast; c++ {
		if c.String() == "" {
			t.Errorf("class %d has no name", c)
		}
	}
}

// What comes back from the transport is not passed on: an error never holds an address or a header.
func TestErrorsCarryNoDetail(t *testing.T) {
	_, err := Do(context.Background(), Request{Method: "GET", URL: "http://127.0.0.1:1/secret-path?token=abc"}, Options{Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("a refused connection succeeded")
	}
	for _, bad := range []string{"secret-path", "token=abc", "127.0.0.1"} {
		if strings.Contains(err.Error(), bad) {
			t.Errorf("the error carries %q: %v", bad, err)
		}
	}
}

// The transport of a call has no proxy and does not keep connections: pinned on the transport itself, because the environment's proxy
// settings are read once per process and a test that sets them cannot tell.
func TestTheTransportHasNoProxyAndNoReuse(t *testing.T) {
	var remote string
	tr := newTransport(Options{Timeout: time.Second}.normalized(), &remote)
	if tr.Proxy != nil {
		t.Error("the transport has a proxy function")
	}
	if !tr.DisableKeepAlives || tr.DialContext == nil || tr.MaxResponseHeaderBytes != 64<<10 || tr.TLSHandshakeTimeout == 0 || tr.ResponseHeaderTimeout != time.Second {
		t.Errorf("%+v", tr)
	}
}

// A HEAD response has no body to read, even from a server that sends one.
func TestHeadReadsNoBody(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 4096)
		_, _ = c.Read(buf)
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"))
	}()
	res, err := Do(context.Background(), Request{Method: "HEAD", URL: "http://" + ln.Addr().String()}, Options{})
	if err != nil || len(res.Body) != 0 || res.Status != 200 {
		t.Fatalf("%+v %v", res, err)
	}
}
