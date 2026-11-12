package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// httpFixture is a transport on a loopback listener, with its token.
type httpFixture struct {
	url   string // http://127.0.0.1:PORT
	token string
	tr    *httpTransport
}

func newHTTPFixture(t *testing.T) *httpFixture {
	t.Helper()
	token, err := newToken()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tr := newHTTPTransport(&Server{}, token, ln.Addr().(*net.TCPAddr), false)
	ts := httptest.NewUnstartedServer(tr)
	ts.Listener.Close()
	ts.Listener = ln
	ts.Start()
	t.Cleanup(ts.Close)
	return &httpFixture{url: "http://" + ln.Addr().String(), token: token, tr: tr}
}

// do sends a well-formed, authorized request; edit changes it before it is sent.
func (f *httpFixture) do(t *testing.T, method, path, body string, edit func(*http.Request)) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, f.url+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if edit != nil {
		edit(req)
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (f *httpFixture) post(t *testing.T, body string, edit func(*http.Request)) (*http.Response, string) {
	t.Helper()
	return f.do(t, http.MethodPost, HTTPEndpointPath, body, edit)
}

const listReq = `{"jsonrpc":"2.0","id":7,"method":"tools/list"}`

// stdioLine runs one message through the stdio loop and returns the bytes it wrote.
func stdioLine(t *testing.T, msg string) string {
	t.Helper()
	var out bytes.Buffer
	s := &Server{in: bufio.NewReader(strings.NewReader(msg + "\n")), out: &out}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestHTTPRightTokenIsServed(t *testing.T) {
	f := newHTTPFixture(t)
	resp, body := f.post(t, listReq, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(body, `"read_test"`) {
		t.Errorf("tools/list did not list the tools: %.200s", body)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("a CORS header was sent: %q", got)
	}
}

// TestHTTPMissingAndWrongTokenLookTheSame: no header, another scheme, an empty token, a wrong token
// of the right length, a wrong token of another length and the right token with a suffix all get
// 401, an empty body and the same headers.
func TestHTTPMissingAndWrongTokenLookTheSame(t *testing.T) {
	f := newHTTPFixture(t)
	other, _ := newToken()
	cases := map[string]func(*http.Request){
		"no header":      func(r *http.Request) { r.Header.Del("Authorization") },
		"basic scheme":   func(r *http.Request) { r.Header.Set("Authorization", "Basic "+f.token) },
		"empty bearer":   func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") },
		"wrong token":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+other) },
		"short token":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") },
		"token prefix":   func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.token[:len(f.token)-1]) },
		"token suffixed": func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+f.token+"0") },
		"token in query": func(r *http.Request) {
			r.Header.Del("Authorization")
			r.URL.RawQuery = "token=" + f.token
		},
	}
	var firstHeaders string
	for name, edit := range cases {
		resp, body := f.post(t, listReq, edit)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, resp.StatusCode)
		}
		if body != "" {
			t.Errorf("%s: body = %q, want none", name, body)
		}
		resp.Header.Del("Date")
		h := fmt.Sprint(resp.Header)
		if firstHeaders == "" {
			firstHeaders = h
		} else if h != firstHeaders {
			t.Errorf("%s: headers differ from another refusal:\n%s\n%s", name, h, firstHeaders)
		}
	}
	// The token is checked on every path and method, before either is looked at.
	for _, c := range [][2]string{{http.MethodGet, HTTPEndpointPath}, {http.MethodPost, "/"}, {http.MethodDelete, "/other"}} {
		resp, _ := f.do(t, c[0], c[1], "", func(r *http.Request) { r.Header.Del("Authorization") })
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: status = %d, want 401", c[0], c[1], resp.StatusCode)
		}
	}
}

func TestTokenMatches(t *testing.T) {
	token, _ := newToken()
	tr := newHTTPTransport(&Server{}, token, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}, false)
	if !tokenMatches(tr.tokenSum, token) {
		t.Error("the right token does not match")
	}
	for _, bad := range []string{"", "x", token[:len(token)-1], token + "0", strings.ToUpper(token), " " + token} {
		if tokenMatches(tr.tokenSum, bad) {
			t.Errorf("a wrong token of length %d matches", len(bad))
		}
	}
}

// TestTokenCompareIsConstantTime reads the source: the token is compared with
// crypto/subtle on fixed-size digests, and no ordinary comparison of it exists.
func TestTokenCompareIsConstantTime(t *testing.T) {
	src, err := os.ReadFile("http.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(src)
	_, fn, ok := strings.Cut(code, "func tokenMatches(")
	if !ok {
		t.Fatal("tokenMatches not found")
	}
	fn, _, _ = strings.Cut(fn, "\n}\n")
	if !strings.Contains(fn, "subtle.ConstantTimeCompare(got[:], want[:]) == 1") || !strings.Contains(fn, "sha256.Sum256(") {
		t.Errorf("tokenMatches does not compare SHA-256 digests with subtle.ConstantTimeCompare:\n%s", fn)
	}
	for _, banned := range []string{"bytes.Equal", "== token", "token ==", "presented ==", "== presented", "strings.EqualFold(tok", "reflect.DeepEqual"} {
		if strings.Contains(code, banned) {
			t.Errorf("http.go contains %q: the token must only be compared in tokenMatches", banned)
		}
	}
}

func TestHTTPOriginIsRefused(t *testing.T) {
	f := newHTTPFixture(t)
	for _, origin := range []string{"https://evil.example", "http://localhost:3000", f.url, "null", ""} {
		resp, body := f.post(t, listReq, func(r *http.Request) { r.Header["Origin"] = []string{origin} })
		if resp.StatusCode != http.StatusForbidden || body != "" {
			t.Errorf("Origin %q: status = %d body = %q, want 403 and no body", origin, resp.StatusCode, body)
		}
	}
	for _, site := range []string{"cross-site", "same-site", "same-origin"} {
		resp, _ := f.post(t, listReq, func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", site) })
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Sec-Fetch-Site %s: status = %d, want 403", site, resp.StatusCode)
		}
	}
	// A preflight gets no permission: no CORS header, whatever it asks for.
	resp, _ := f.do(t, http.MethodOptions, HTTPEndpointPath, "", func(r *http.Request) {
		r.Header.Del("Authorization")
		r.Header.Set("Origin", "https://evil.example")
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("preflight: status = %d, want 403", resp.StatusCode)
	}
	for h := range resp.Header {
		if strings.HasPrefix(h, "Access-Control-") {
			t.Errorf("preflight answered with %s", h)
		}
	}
}

func TestHTTPHostMustNameTheListener(t *testing.T) {
	f := newHTTPFixture(t)
	port := f.tr.bound.Port
	bad := []string{
		"evil.example", fmt.Sprintf("evil.example:%d", port), fmt.Sprintf("127.0.0.1.evil.example:%d", port),
		fmt.Sprintf("127.0.0.1:%d", port+1), "127.0.0.1", "localhost",
		fmt.Sprintf("127.0.0.2:%d", port), fmt.Sprintf("[::1]:%d", port), fmt.Sprintf("192.168.1.5:%d", port),
	}
	for _, host := range bad {
		resp, body := f.post(t, listReq, func(r *http.Request) { r.Host = host })
		if resp.StatusCode != http.StatusForbidden || body != "" {
			t.Errorf("Host %q: status = %d body = %q, want 403 and no body", host, resp.StatusCode, body)
		}
	}
	for _, host := range []string{fmt.Sprintf("127.0.0.1:%d", port), fmt.Sprintf("localhost:%d", port), fmt.Sprintf("LOCALHOST:%d", port)} {
		if resp, _ := f.post(t, listReq, func(r *http.Request) { r.Host = host }); resp.StatusCode != http.StatusOK {
			t.Errorf("Host %q: status = %d, want 200", host, resp.StatusCode)
		}
	}
}

func TestHostAllowedByBoundAddress(t *testing.T) {
	cases := []struct {
		bound net.IP
		host  string
		want  bool
	}{
		{net.IPv6loopback, "[::1]:9", true},
		{net.IPv6loopback, "localhost:9", true},
		{net.IPv6loopback, "127.0.0.1:9", false},
		{net.IPv4(192, 168, 1, 5), "192.168.1.5:9", true},
		{net.IPv4(192, 168, 1, 5), "localhost:9", false},
		{net.IPv4(192, 168, 1, 5), "mybox.lan:9", false},
		{net.IPv4zero, "192.168.1.5:9", true},
		{net.IPv4zero, "localhost:9", true},
		{net.IPv4zero, "evil.example:9", false},
		{net.IPv4zero, "192.168.1.5:10", false},
	}
	for _, c := range cases {
		tr := &httpTransport{bound: &net.TCPAddr{IP: c.bound, Port: 9}}
		if got := tr.hostAllowed(c.host); got != c.want {
			t.Errorf("bound %s, Host %q: allowed = %v, want %v", c.bound, c.host, got, c.want)
		}
	}
}

// TestHTTPPeerMustBeLoopback: without the switch a peer that is not loopback is refused even with
// the right token and Host.
func TestHTTPPeerMustBeLoopback(t *testing.T) {
	token, _ := newToken()
	bound := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}
	serve := func(allow bool, remote string) int {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9"+HTTPEndpointPath, strings.NewReader(listReq))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		newHTTPTransport(&Server{}, token, bound, allow).ServeHTTP(rec, req)
		return rec.Code
	}
	if got := serve(false, "192.168.1.9:5555"); got != http.StatusForbidden {
		t.Errorf("LAN peer without the switch: %d, want 403", got)
	}
	if got := serve(false, "garbage"); got != http.StatusForbidden {
		t.Errorf("unparsable peer: %d, want 403", got)
	}
	if got := serve(false, "127.0.0.1:5555"); got != http.StatusOK {
		t.Errorf("loopback peer: %d, want 200", got)
	}
	if got := serve(false, "[::1]:5555"); got != http.StatusOK {
		t.Errorf("IPv6 loopback peer: %d, want 200", got)
	}
	if got := serve(true, "192.168.1.9:5555"); got != http.StatusOK {
		t.Errorf("LAN peer with the switch: %d, want 200", got)
	}
}

func TestHTTPNonLoopbackBindIsRefused(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", ":0", "[::]:0", "192.168.1.5:7391", "10.0.0.1:1", "[2001:db8::1]:1"} {
		if _, err := resolveHTTPAddr(addr, false); err == nil || !strings.Contains(err.Error(), "--dangerously-allow-non-loopback") {
			t.Errorf("resolveHTTPAddr(%q) = %v, want a refusal that names the switch", addr, err)
		}
		if _, err := resolveHTTPAddr(addr, true); err != nil {
			t.Errorf("resolveHTTPAddr(%q) with the switch: %v", addr, err)
		}
	}
	for addr, want := range map[string]string{"127.0.0.1:7391": "127.0.0.1:7391", "localhost:0": "127.0.0.1:0", "[::1]:5": "[::1]:5", "127.9.9.9:1": "127.9.9.9:1"} {
		if got, err := resolveHTTPAddr(addr, false); err != nil || got != want {
			t.Errorf("resolveHTTPAddr(%q) = %q, %v, want %q", addr, got, err, want)
		}
	}
	// A name is never resolved, with or without the switch: what it points at can change.
	for _, addr := range []string{"example.com:80", "mybox.lan:7391", "7391", ""} {
		if _, err := resolveHTTPAddr(addr, true); err == nil {
			t.Errorf("resolveHTTPAddr(%q) was accepted", addr)
		}
	}
	// RunHTTP itself refuses before it listens or writes a token. The address is from the
	// documentation range (192.0.2.0/24), which no interface here has: if the refusal were ever
	// missing, the listen would fail instead of opening a port to the network.
	dir := filepath.Join(t.TempDir(), "tok")
	err := RunHTTP(context.Background(), HTTPOptions{Addr: "192.0.2.1:0", TokenDir: dir})
	if err == nil || !strings.Contains(err.Error(), "not a loopback address") {
		t.Fatalf("RunHTTP on a non-loopback address = %v, want a refusal", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Errorf("a refused start left %s behind", dir)
	}
}

func TestHTTPContentTypeMustBeJSON(t *testing.T) {
	f := newHTTPFixture(t)
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", "application/json-seq", "text/json", "application/json; charset"} {
		resp, body := f.post(t, listReq, func(r *http.Request) {
			r.Header.Del("Content-Type")
			if ct != "" {
				r.Header.Set("Content-Type", ct)
			}
		})
		if resp.StatusCode != http.StatusUnsupportedMediaType || body != "" {
			t.Errorf("Content-Type %q: status = %d body = %q, want 415 and no body", ct, resp.StatusCode, body)
		}
	}
	resp, _ := f.post(t, listReq, func(r *http.Request) { r.Header.Set("Content-Type", "application/json; charset=utf-8") })
	if resp.StatusCode != http.StatusOK {
		t.Errorf("application/json with a charset: status = %d, want 200", resp.StatusCode)
	}
}

func TestHTTPBodySizeIsCapped(t *testing.T) {
	f := newHTTPFixture(t)
	big := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"pad":"` + strings.Repeat("a", maxHTTPBody) + `"}}`
	resp, body := f.post(t, big, nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || body != "" {
		t.Errorf("oversize body: status = %d, body %d bytes, want 413 and no body", resp.StatusCode, len(body))
	}
	ok := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"pad":"` + strings.Repeat("a", maxHTTPBody/2) + `"}}`
	if resp, _ := f.post(t, ok, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("a body under the cap: status = %d, want 200", resp.StatusCode)
	}
}

func TestHTTPMethodsAndPath(t *testing.T) {
	f := newHTTPFixture(t)
	for _, m := range []string{http.MethodGet, http.MethodDelete, http.MethodPut, http.MethodPatch, http.MethodHead, http.MethodOptions} {
		resp, body := f.do(t, m, HTTPEndpointPath, "", nil)
		if resp.StatusCode != http.StatusMethodNotAllowed || body != "" {
			t.Errorf("%s: status = %d, want 405 and no body", m, resp.StatusCode)
		}
		if resp.Header.Get("Allow") != http.MethodPost {
			t.Errorf("%s: Allow = %q", m, resp.Header.Get("Allow"))
		}
	}
	for _, p := range []string{"/", "/rpc", "/mcp/", "/mcp/x", "/MCP"} {
		if resp, _ := f.do(t, http.MethodPost, p, listReq, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s: status = %d, want 404", p, resp.StatusCode)
		}
	}
}

func TestHTTPProtocolVersionHeader(t *testing.T) {
	f := newHTTPFixture(t)
	for _, v := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"} {
		if resp, _ := f.post(t, listReq, func(r *http.Request) { r.Header.Set("MCP-Protocol-Version", v) }); resp.StatusCode != http.StatusOK {
			t.Errorf("MCP-Protocol-Version %s: status = %d, want 200", v, resp.StatusCode)
		}
	}
	for _, v := range []string{"1999-01-01", "latest", "2025-06-18x"} {
		if resp, _ := f.post(t, listReq, func(r *http.Request) { r.Header.Set("MCP-Protocol-Version", v) }); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("MCP-Protocol-Version %q: status = %d, want 400", v, resp.StatusCode)
		}
	}
}

// TestHTTPNotificationsGet202: a notification or a client response is accepted with no body; a
// request gets its JSON answer; a body that is not JSON gets the stdio parse error with 400.
func TestHTTPNotificationsGet202(t *testing.T) {
	f := newHTTPFixture(t)
	for _, msg := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":3}}`,
		`{"jsonrpc":"2.0","id":4,"result":{}}`,
	} {
		resp, body := f.post(t, msg, nil)
		if resp.StatusCode != http.StatusAccepted || body != "" {
			t.Errorf("%s: status = %d body = %q, want 202 and no body", msg, resp.StatusCode, body)
		}
	}
	resp, body := f.post(t, `not json`, nil)
	if resp.StatusCode != http.StatusBadRequest || body+"\n" != stdioLine(t, `not json`) {
		t.Errorf("bad JSON: status = %d body = %q, want 400 and the stdio parse error", resp.StatusCode, body)
	}
}

func TestHTTPInFlightCap(t *testing.T) {
	f := newHTTPFixture(t)
	for i := 0; i < maxHTTPInFlight; i++ {
		f.tr.inFlight <- struct{}{}
	}
	resp, body := f.post(t, listReq, nil)
	if resp.StatusCode != http.StatusServiceUnavailable || body != "" {
		t.Errorf("at the cap: status = %d, want 503 and no body", resp.StatusCode)
	}
	<-f.tr.inFlight
	if resp, _ := f.post(t, listReq, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("one slot free: status = %d, want 200", resp.StatusCode)
	}
	if got := len(f.tr.inFlight); got != maxHTTPInFlight-1 {
		t.Errorf("a finished call did not give its slot back: %d in flight", got)
	}
}

// TestHTTPAndStdioGiveTheSameJSON: the same message gives the same bytes on both transports
// (stdio ends each message with a newline; HTTP does not).
func TestHTTPAndStdioGiveTheSameJSON(t *testing.T) {
	f := newHTTPFixture(t)
	file := filepath.Join(t.TempDir(), "a.probe")
	if err := os.WriteFile(file, []byte("test \"a\"\n  see \"Hello\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	call := func(name string, args map[string]string) string {
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "c1", "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		return string(b)
	}
	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		listReq,
		call("read_test", map[string]string{"path": file}),
		call("read_test", map[string]string{"path": filepath.Join(t.TempDir(), "missing.probe")}),
		call("write_test", map[string]string{"path": filepath.Join(t.TempDir(), "b.probe"), "content": "this is not ProbeScript ((("}),
		call("no_such_tool", nil),
		`{"jsonrpc":"2.0","id":9,"method":"no/such/method"}`,
	}
	for _, msg := range msgs {
		want := stdioLine(t, msg)
		resp, got := f.post(t, msg, nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%.60s: status = %d", msg, resp.StatusCode)
		}
		if got+"\n" != want {
			t.Errorf("%.60s:\n http  %.300s\n stdio %.300s", msg, got, want)
		}
		if strings.Contains(got, f.token) {
			t.Errorf("%.60s: the answer contains the token", msg)
		}
	}
}

// TestStdioBytesUnchanged pins the stdio answers byte for byte.
func TestStdioBytesUnchanged(t *testing.T) {
	old := Version
	Version = "dev"
	defer func() { Version = old }()
	got := stdioLine(t, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	want := `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"probe-mcp","version":"dev"}}}` + "\n"
	if got != want {
		t.Errorf("initialize over stdio:\n got  %q\n want %q", got, want)
	}
	if got := stdioLine(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); got != "" {
		t.Errorf("a notification over stdio wrote %q", got)
	}
	got = stdioLine(t, `{"jsonrpc":"2.0","id":2,"method":"nope"}`)
	want = `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"Method not found: nope"}}` + "\n"
	if got != want {
		t.Errorf("unknown method over stdio:\n got  %q\n want %q", got, want)
	}
	for _, tool := range tools {
		if strings.Contains(strings.ToLower(tool.Name+tool.Description), "http transport") {
			t.Errorf("tool %s mentions the HTTP transport: the tool list must not change", tool.Name)
		}
	}
}

// ---- token file ----

func skipWithoutFileModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not describe access on Windows")
	}
}

func TestTokenFileModes(t *testing.T) {
	skipWithoutFileModes(t)
	dir := filepath.Join(t.TempDir(), "cfg", tokenDirName)
	token, _ := newToken()
	path, err := writeTokenFile(dir, token)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 {
		t.Errorf("token is %d hex characters, want 64 (256 bits)", len(token))
	}
	if fi, _ := os.Lstat(path); fi.Mode() != 0o600 {
		t.Errorf("token file mode = %v, want -rw-------", fi.Mode())
	}
	if di, _ := os.Lstat(dir); di.Mode().Perm() != 0o700 {
		t.Errorf("token dir mode = %v, want 0700", di.Mode().Perm())
	}
	if b, _ := os.ReadFile(path); string(b) != token+"\n" {
		t.Error("the token file does not hold the token")
	}
	// A second start replaces the token and leaves nothing else in the directory.
	second, _ := newToken()
	if _, err := writeTokenFile(dir, second); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != second+"\n" || second == token {
		t.Error("a second start did not replace the token")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("token dir holds %d entries, want 1", len(entries))
	}
	// Removal leaves a newer server's token alone.
	removeTokenFile(path, token)
	if _, err := os.Stat(path); err != nil {
		t.Error("an old server removed a newer server's token file")
	}
	removeTokenFile(path, second)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the token file was not removed")
	}
}

// privateDir returns a new 0700 directory. (t.TempDir itself is made 0755.)
func privateDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// wantRefusal checks that err is the refusal for that path and that reason, by its full wording:
// the temp paths contain the test's name, so a loose match would always pass.
func wantRefusal(t *testing.T, err error, what, path, reason string) {
	t.Helper()
	want := "mcp http: " + what + " " + path + " " + reason
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("err = %v\nwant a refusal starting %q", err, want)
	}
}

func TestTokenFileRefusals(t *testing.T) {
	skipWithoutFileModes(t)
	token, _ := newToken()
	t.Run("file 0644", func(t *testing.T) {
		dir := privateDir(t)
		path := filepath.Join(dir, tokenFileName)
		for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o660} {
			if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			_, err := writeTokenFile(dir, token)
			wantRefusal(t, err, "token file", path, "is open to other users")
			if b, _ := os.ReadFile(path); string(b) != "old\n" {
				t.Errorf("mode %04o: the refused file was written", mode)
			}
		}
	})
	t.Run("file link", func(t *testing.T) {
		dir := privateDir(t)
		path := filepath.Join(dir, tokenFileName)
		target := filepath.Join(privateDir(t), "elsewhere")
		if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		_, err := writeTokenFile(dir, token)
		wantRefusal(t, err, "token file", path, "is a symlink or not a regular file")
		if b, _ := os.ReadFile(target); string(b) != "keep\n" {
			t.Error("the link target was written")
		}
		if fi, _ := os.Lstat(path); fi == nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Error("the refused link was replaced")
		}
	})
	t.Run("dangling link", func(t *testing.T) {
		dir := privateDir(t)
		path := filepath.Join(dir, tokenFileName)
		target := filepath.Join(privateDir(t), "not-there")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		_, err := writeTokenFile(dir, token)
		wantRefusal(t, err, "token file", path, "is a symlink or not a regular file")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Error("the token was written through the link")
		}
	})
	t.Run("wide dir", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o755, 0o750, 0o701} {
			dir := privateDir(t)
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			_, err := writeTokenFile(dir, token)
			wantRefusal(t, err, "token directory", dir, "is open to other users")
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("mode %04o: the token was written into the refused directory", mode)
			}
		}
	})
	t.Run("dir link", func(t *testing.T) {
		real := privateDir(t)
		link := filepath.Join(privateDir(t), "link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		_, err := writeTokenFile(link, token)
		wantRefusal(t, err, "token directory", link, "is a symlink or not a directory")
		if entries, _ := os.ReadDir(real); len(entries) != 0 {
			t.Error("the token was written through the directory link")
		}
	})
}

func TestSuppliedTokenRules(t *testing.T) {
	long := strings.Repeat("k", minSuppliedToken)
	if err := checkSuppliedToken(long); err != nil {
		t.Errorf("a %d-character token was refused: %v", len(long), err)
	}
	for _, bad := range []string{"short", long[1:], long + " x", long + "\n", long + "é"} {
		err := checkSuppliedToken(bad)
		if err == nil {
			t.Errorf("token of length %d was accepted", len(bad))
			continue
		}
		if len(bad) > 8 && strings.Contains(err.Error(), bad[:8]) {
			t.Errorf("the refusal repeats the token: %v", err)
		}
	}
}

// ---- the whole server ----

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

var listenLine = regexp.MustCompile(`MCP over HTTP at (http://\S+)/mcp`)

// startRunHTTP runs RunHTTP on a free loopback port and returns its URL, its log and a stop
// function that waits for it to return.
func startRunHTTP(t *testing.T, opts HTTPOptions) (string, *syncBuffer, func() error) {
	t.Helper()
	logs := &syncBuffer{}
	opts.Addr, opts.Log = "127.0.0.1:0", logs
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunHTTP(ctx, opts) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if m := listenLine.FindStringSubmatch(logs.String()); m != nil {
			stopped := false
			stop := func() error {
				if stopped {
					return nil
				}
				stopped = true
				cancel()
				return <-done
			}
			t.Cleanup(func() { _ = stop() })
			return m[1], logs, stop
		}
		select {
		case err := <-done:
			cancel()
			t.Fatalf("RunHTTP returned early: %v\n%s", err, logs.String())
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("RunHTTP did not start:\n%s", logs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRunHTTPNeverLogsTheToken starts the real server, sends good and bad requests, stops it, and
// checks that the token is in the token file and nowhere in the log.
func TestRunHTTPNeverLogsTheToken(t *testing.T) {
	skipWithoutFileModes(t)
	dir := filepath.Join(t.TempDir(), tokenDirName)
	url, logs, stop := startRunHTTP(t, HTTPOptions{TokenDir: dir})
	path := filepath.Join(dir, tokenFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	if len(token) != 64 {
		t.Fatalf("token file holds %d characters, want 64", len(token))
	}
	f := &httpFixture{url: url, token: token}
	if resp, _ := f.post(t, listReq, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("the token from the file was not accepted: %d", resp.StatusCode)
	}
	other, _ := newToken()
	f.post(t, listReq, func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+other) })
	f.post(t, listReq, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") })
	f.post(t, "not json", nil)
	f.post(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_test","arguments":{"path":"`+filepath.ToSlash(path)+`.probe"}}}`, nil)
	if err := stop(); err != nil {
		t.Fatalf("RunHTTP: %v", err)
	}
	out := logs.String()
	if strings.Contains(out, token) || strings.Contains(out, other) {
		t.Errorf("the log contains a token:\n%s", strings.ReplaceAll(strings.ReplaceAll(out, token, "<token>"), other, "<other>"))
	}
	if !strings.Contains(out, path) {
		t.Errorf("the log does not name the token file %s:\n%s", path, out)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the token file is still there after the server stopped")
	}
}

// TestRunHTTPNewTokenOnEveryStart: two starts, two tokens; the first stops working.
func TestRunHTTPNewTokenOnEveryStart(t *testing.T) {
	skipWithoutFileModes(t)
	dir := filepath.Join(t.TempDir(), tokenDirName)
	read := func() string {
		b, err := os.ReadFile(filepath.Join(dir, tokenFileName))
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(b))
	}
	// A token file left by a server that was killed is replaced, never reused.
	stale, _ := newToken()
	if _, err := writeTokenFile(dir, stale); err != nil {
		t.Fatal(err)
	}
	firstURL, _, stop := startRunHTTP(t, HTTPOptions{TokenDir: dir})
	first := read()
	if first == stale {
		t.Fatal("a token file from an earlier server was reused")
	}
	if resp, _ := (&httpFixture{url: firstURL, token: stale}).post(t, listReq, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the token left on disk by an earlier server: status = %d, want 401", resp.StatusCode)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	url, _, _ := startRunHTTP(t, HTTPOptions{TokenDir: dir})
	second := read()
	if first == second {
		t.Fatal("the token did not change between starts")
	}
	if resp, _ := (&httpFixture{url: url, token: first}).post(t, listReq, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the previous start's token: status = %d, want 401", resp.StatusCode)
	}
	if resp, _ := (&httpFixture{url: url, token: second}).post(t, listReq, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the current token: status = %d, want 200", resp.StatusCode)
	}
}

// TestRunHTTPSuppliedToken: a supplied token is used, nothing is written to disk, a weak one is
// refused, and neither shows in the log or the error.
func TestRunHTTPSuppliedToken(t *testing.T) {
	dir := filepath.Join(t.TempDir(), tokenDirName)
	supplied, _ := newToken()
	url, logs, stop := startRunHTTP(t, HTTPOptions{TokenDir: dir, Token: supplied})
	if resp, _ := (&httpFixture{url: url, token: supplied}).post(t, listReq, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("the supplied token: status = %d, want 200", resp.StatusCode)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("a token file directory was made although the token was supplied")
	}
	if strings.Contains(logs.String(), supplied) {
		t.Error("the log contains the supplied token")
	}
	weak := "hunter2-hunter2"
	err := RunHTTP(context.Background(), HTTPOptions{Addr: "127.0.0.1:0", TokenDir: dir, Token: weak})
	if err == nil || strings.Contains(err.Error(), weak) {
		t.Errorf("a weak supplied token: err = %v, want a refusal that does not repeat it", err)
	}
}

// TestHTTPServerLimitsAreSet pins the timeouts and caps a slow or greedy client meets.
func TestHTTPServerLimitsAreSet(t *testing.T) {
	src, err := os.ReadFile("http.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ReadHeaderTimeout: httpReadHeaderTimeout", "ReadTimeout:       httpReadTimeout", "WriteTimeout:      httpWriteTimeout", "IdleTimeout:       httpIdleTimeout", "MaxHeaderBytes:    httpMaxHeader"} {
		if !strings.Contains(string(src), field) {
			t.Errorf("the http.Server does not set %q", field)
		}
	}
	if httpReadHeaderTimeout <= 0 || httpReadTimeout <= 0 || httpWriteTimeout <= 0 || httpIdleTimeout <= 0 {
		t.Error("a timeout is zero, which means none")
	}
}
