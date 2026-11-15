package httpcall

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// rawServer answers each connection with whatever handle writes, so a test can send a response that a real server would not.
func rawServer(t *testing.T, handle func(c net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
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
				handle(c)
			}()
		}
	}()
	return "http://" + ln.Addr().String()
}

// A body that stops part way is an error and never a shorter response that looks whole.
func TestAnIncompleteBodyIsAnError(t *testing.T) {
	short := rawServer(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n0123456789"))
	}) // closes after 10 of 1000 bytes
	badChunk := rawServer(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\nZZ\r\nbroken"))
	})
	cutChunk := rawServer(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n"))
	}) // ends without the last chunk
	stall := rawServer(t, func(c net.Conn) {
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n0123456789"))
		time.Sleep(3 * time.Second)
	})
	for name, c := range map[string]struct {
		url  string
		want string
	}{"short Content-Length": {short, "ended before it was complete"}, "bad chunk": {badChunk, "ended before it was complete"},
		"missing last chunk": {cutChunk, "ended before it was complete"}, "stall until the time limit": {stall, "time limit"}} {
		start := time.Now()
		res, err := Do(context.Background(), Request{Method: "GET", URL: c.url}, Options{Timeout: 700 * time.Millisecond})
		if err == nil || !strings.Contains(err.Error(), c.want) || len(res.Body) != 0 || res.Status != 0 || time.Since(start) > 2*time.Second {
			t.Errorf("%s: %d bytes, status %d, err %v, after %v", name, len(res.Body), res.Status, err, time.Since(start))
		}
	}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: short}, Options{}); !errors.Is(err, ErrIncomplete) {
		t.Errorf("short Content-Length is not ErrIncomplete: %v", err)
	}
	// A body that is exactly as long as its Content-Length is whole, and one over the cap is cut, not an error.
	whole := rawServer(t, func(c net.Conn) { _, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello")) })
	if res, err := Do(context.Background(), Request{Method: "GET", URL: whole}, Options{}); err != nil || string(res.Body) != "hello" || res.Truncated {
		t.Errorf("a whole body: %+v %v", res, err)
	}
}

// The redirect limit counts followed redirects: the limit is followed and the next one is refused.
func TestTheRedirectLimitBoundary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if n >= 6 {
			_, _ = w.Write([]byte("end"))
			return
		}
		http.Redirect(w, r, "/"+strconv.Itoa(n+1), http.StatusFound)
	}))
	defer srv.Close()
	// /1 .. /6: five redirects to reach /6.
	res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/1"}, Options{})
	if err != nil || len(res.Hops) != 5 || string(res.Body) != "end" {
		t.Fatalf("five redirects: %d hops, %v", len(res.Hops), err)
	}
	// /0 needs six.
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/0"}, Options{}); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("six redirects: %v", err)
	}
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/0"}, Options{MaxRedirects: 6}); err != nil {
		t.Fatalf("six allowed: %v", err)
	}
}

// A certificate that is not trusted is refused: there is no switch to skip the check.
func TestAnUntrustedCertificateIsRefused(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{}); err == nil {
		t.Fatal("a self-signed server was accepted")
	}
	if hits.Load() != 0 {
		t.Fatalf("the handler ran %d time(s)", hits.Load())
	}
}

// There is no cookie jar: a cookie set on one hop is not sent on the next, and a connection is not reused between hops.
func TestNoCookiesAndNoConnectionReuse(t *testing.T) {
	var conns atomic.Int64
	var cookieSeen atomic.Value
	cookieSeen.Store("unset")
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "v"})
			http.Redirect(w, r, "/b", http.StatusFound)
		case "/b":
			cookieSeen.Store(r.Header.Get("Cookie"))
			_, _ = w.Write([]byte("ok"))
		}
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/a"}, Options{}); err != nil {
		t.Fatal(err)
	}
	if c := cookieSeen.Load().(string); c != "" {
		t.Errorf("a cookie was sent on the next hop: %q", c)
	}
	if conns.Load() != 2 {
		t.Errorf("two requests used %d connection(s)", conns.Load())
	}
}

// The User-Agent is fixed, a caller cannot change it, and a caller cannot turn compression off: a gzip answer is decoded under the cap.
func TestUserAgentAndCompression(t *testing.T) {
	big := bytes.Repeat([]byte("0"), 5<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-UA", r.Header.Get("User-Agent"))
		if r.URL.Path == "/gz" && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = zw.Write(big)
			_ = zw.Close()
			return
		}
		_, _ = w.Write([]byte("plain"))
	}))
	defer srv.Close()
	res, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{})
	if err != nil || res.Header.Get("X-UA") != UserAgent || !strings.HasPrefix(UserAgent, "FlutterProbe-Studio") {
		t.Fatalf("%q %v", res.Header.Get("X-UA"), err)
	}
	for _, h := range []string{"User-Agent", "user-agent", "Accept-Encoding"} {
		if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL, Header: map[string]string{h: "x"}}, Options{}); !errors.Is(err, ErrHeader) {
			t.Errorf("%s: %v", h, err)
		}
	}
	res, err = Do(context.Background(), Request{Method: "GET", URL: srv.URL + "/gz"}, Options{})
	if err != nil || !res.Truncated || len(res.Body) != DefaultMaxResponse || res.Body[0] != '0' {
		t.Fatalf("a gzip body: %d bytes, truncated %v, %v", len(res.Body), res.Truncated, err)
	}
}

// Ports 0 and 65536, a non-number and a zone are refused before anything is dialed, also in a redirect.
func TestPortsAndZonesAreRefused(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:0/", "http://127.0.0.1:65536/", "http://127.0.0.1:99999/", "http://127.0.0.1:-1/", "http://127.0.0.1:1e2/",
		"http://[fe80::1%25eth0]/", "http://[fe80::1%25eth0]:80/", "http://[::1%25lo]:80/"} {
		if _, err := Do(context.Background(), Request{Method: "GET", URL: u}, Options{Timeout: time.Second}); err == nil || !errors.Is(err, ErrURL) {
			t.Errorf("%s: %v", u, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+r.Host[:strings.LastIndex(r.Host, ":")]+":0/x", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{}); !errors.Is(err, ErrURL) {
		t.Errorf("a redirect to port 0: %v", err)
	}
}

// The policy sees the host in a normal form (lower case, no trailing dot) with its port, and the resolver is asked for that form.
func TestThePolicySeesAHostAndAPort(t *testing.T) {
	srv, _ := echo(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	r := &stubResolver{answers: map[string][]net.IP{"app.example.test": {net.ParseIP("127.0.0.1")}}}
	var gotHost string
	var gotPort int
	policy := func(host string, p int, ip net.IP) error { gotHost, gotPort = host, p; return nil }
	if _, err := Do(context.Background(), Request{Method: "GET", URL: "http://App.Example.TEST.:" + port + "/"}, Options{Policy: policy, Resolver: r}); err != nil {
		t.Fatal(err)
	}
	if want, _ := strconv.Atoi(port); gotHost != "app.example.test" || gotPort != want {
		t.Errorf("policy saw %q:%d", gotHost, gotPort)
	}
	// The refusal carries the class and nothing else.
	deny := func(host string, p int, ip net.IP) error { return errors.New("secret detail 127.0.0.1") }
	_, err := Do(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{Policy: deny})
	var pe *PolicyError
	if !errors.As(err, &pe) || pe.Class != Loopback || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Errorf("%v", err)
	}
}
