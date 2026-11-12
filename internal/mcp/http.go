package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

// The HTTP transport is the MCP "Streamable HTTP" transport in its smallest form: one endpoint, a
// client message per POST, one JSON object back. The server never starts a stream of its own, so
// GET answers 405, and it keeps no sessions, so no session id header is issued or required.
//
// It is off unless the binary is started with --http. An MCP server runs tests and reads and writes
// files, so the transport is closed by default on every side:
//
//   - the listener is loopback only unless the caller passes the explicit non-loopback switch;
//   - every request needs the bearer token (httptoken.go), compared in constant time;
//   - a request that carries an Origin header is a browser request and is refused, and the Host
//     header must name the address the server listens on (DNS rebinding);
//   - POST bodies must be application/json, which a cross-site form cannot send, and are size capped.
//
// The JSON-RPC message is passed to the same Server.handle the stdio loop uses.

const (
	// HTTPEndpointPath is the one MCP endpoint.
	HTTPEndpointPath = "/mcp"

	// maxHTTPBody caps a POST body. write_test and run_script carry file-sized text, nothing larger.
	maxHTTPBody = 4 << 20
	// maxHTTPInFlight caps the tool calls that run at once; a further call is answered 503.
	maxHTTPInFlight = 8

	httpReadHeaderTimeout = 5 * time.Second
	httpReadTimeout       = 30 * time.Second
	httpIdleTimeout       = 60 * time.Second
	// httpWriteTimeout bounds one call from the end of its headers to the end of its answer. run_tests
	// blocks for the whole run, so this is long; a longer run uses start_tests and test_status.
	httpWriteTimeout = 30 * time.Minute
	httpMaxHeader    = 16 << 10
)

// httpProtocolVersions are the MCP-Protocol-Version header values the endpoint accepts. A request
// without the header is accepted (the spec's backwards-compatibility rule); any other value is 400.
var httpProtocolVersions = map[string]bool{
	"2024-11-05": true, "2025-03-26": true, "2025-06-18": true, "2025-11-25": true,
}

// HTTPOptions configures RunHTTP.
type HTTPOptions struct {
	// Addr is the host:port to listen on. The host must be an IP address or "localhost".
	Addr string
	// AllowNonLoopback permits a listener other hosts can reach. Off, such an address is refused.
	AllowNonLoopback bool
	// Token is a caller-supplied bearer token (from the environment). Empty means: generate one and
	// write it to the token file.
	Token string
	// TokenDir is the directory of the token file. Empty means the default (DefaultTokenDir).
	TokenDir string
	// Log receives the start-up lines. The token is never written to it.
	Log io.Writer
}

// resolveHTTPAddr checks addr and returns the host:port to listen on. The host must be an IP
// literal or "localhost" (taken as 127.0.0.1, without asking a resolver), and it must be a
// loopback address unless allowNonLoopback is set. An empty host means every interface in Go, so
// it counts as non-loopback.
func resolveHTTPAddr(addr string, allowNonLoopback bool) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("--http wants host:port, for example 127.0.0.1:7391: %v", err)
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if host != "" && ip == nil {
		return "", fmt.Errorf("--http host %q must be an IP address or localhost", host)
	}
	if ip == nil || !ip.IsLoopback() {
		if !allowNonLoopback {
			return "", fmt.Errorf("refusing to listen on %q: it is not a loopback address, so other machines could reach a server that runs tests and reads and writes files. Use 127.0.0.1, or pass --dangerously-allow-non-loopback if you accept that risk", addr)
		}
	}
	return net.JoinHostPort(host, port), nil
}

// httpTransport is the http.Handler of the endpoint.
type httpTransport struct {
	server *Server
	// tokenSum is the SHA-256 of the bearer token; the presented token is hashed and the two
	// digests are compared in constant time, so neither content nor length shows in the timing.
	tokenSum [sha256.Size]byte
	// bound is the listener address. Host must name it.
	bound *net.TCPAddr
	// allowNonLoopback mirrors the start-up switch: off, a peer that is not loopback is refused.
	allowNonLoopback bool
	inFlight         chan struct{}
}

func newHTTPTransport(s *Server, token string, bound *net.TCPAddr, allowNonLoopback bool) *httpTransport {
	return &httpTransport{
		server:           s,
		tokenSum:         sha256.Sum256([]byte(token)),
		bound:            bound,
		allowNonLoopback: allowNonLoopback,
		inFlight:         make(chan struct{}, maxHTTPInFlight),
	}
}

// tokenMatches reports whether the presented token is the server's, in constant time.
func tokenMatches(want [sha256.Size]byte, presented string) bool {
	got := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// bearerToken returns the token of an "Authorization: Bearer <token>" header, or "" when the header
// is missing or of another scheme. "" never matches: the server's token is never empty.
func bearerToken(r *http.Request) string {
	scheme, tok, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(tok)
}

// hostAllowed reports whether a Host header names the listener. A page that reaches the port through
// a DNS name it controls (DNS rebinding) sends that name as Host, so only an IP literal or
// "localhost" with the listening port passes: the bound address itself, "localhost" for a loopback
// or all-interfaces listener, and any IP literal for an all-interfaces listener.
func (t *httpTransport) hostAllowed(hostport string) bool {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		host, port = strings.Trim(hostport, "[]"), "80" // a Host without a port means port 80
	}
	if port != fmt.Sprint(t.bound.Port) {
		return false
	}
	unspecified := t.bound.IP.IsUnspecified()
	if strings.EqualFold(host, "localhost") {
		return unspecified || t.bound.IP.IsLoopback()
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return unspecified || ip.Equal(t.bound.IP)
}

// peerAllowed reports whether the connection's remote address may talk to the endpoint. Without the
// non-loopback switch only a loopback peer may; the listener is loopback-bound already, this is a
// second check on the same rule. With the switch every peer may, whatever the bind.
func (t *httpTransport) peerAllowed(remote string) bool {
	if t.allowNonLoopback {
		return true
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// emptyReply answers with a status and no body. Refusals carry no detail on purpose.
func emptyReply(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(status)
}

// refuse answers a caller that has not shown the token (or may not ask at all) and ends the
// connection. Without "Connection: close" the server would first read what the caller sends as a
// body, up to 256 KiB, before it answers; a caller without the token gets nothing read.
func refuse(w http.ResponseWriter, status int) {
	w.Header().Set("Connection", "close")
	emptyReply(w, status)
}

func (t *httpTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// 1. Where the request comes from and what it names. None of this looks at the token.
	if !t.peerAllowed(r.RemoteAddr) || !t.hostAllowed(r.Host) {
		refuse(w, http.StatusForbidden)
		return
	}
	// A browser sends Origin on every cross-origin request and on every POST; MCP clients (an IDE,
	// curl) do not. No page is served from this address, so no Origin is an allowed one. No CORS
	// header is ever sent, so a preflight (OPTIONS) ends here as well.
	if len(r.Header.Values("Origin")) > 0 {
		refuse(w, http.StatusForbidden)
		return
	}
	// Sec-Fetch-Site "none" is what a browser sends for a request no page started (an address typed
	// by the user) and what some non-browser clients send; it goes on to the token check.
	if f := r.Header.Get("Sec-Fetch-Site"); f != "" && f != "none" {
		refuse(w, http.StatusForbidden)
		return
	}

	// 2. The token. Missing, malformed and wrong take the same path and get the same answer.
	if !tokenMatches(t.tokenSum, bearerToken(r)) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		refuse(w, http.StatusUnauthorized)
		return
	}

	// 3. The shape of the request.
	if r.URL.Path != HTTPEndpointPath {
		emptyReply(w, http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		// GET would open a server-to-client stream and DELETE would end a session; this server has
		// neither, and the spec's answer for both is 405.
		w.Header().Set("Allow", http.MethodPost)
		emptyReply(w, http.StatusMethodNotAllowed)
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		emptyReply(w, http.StatusUnsupportedMediaType)
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !httpProtocolVersions[v] {
		emptyReply(w, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxHTTPBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			emptyReply(w, http.StatusRequestEntityTooLarge)
			return
		}
		emptyReply(w, http.StatusBadRequest)
		return
	}

	// 4. Dispatch, through the same handler as stdio.
	select {
	case t.inFlight <- struct{}{}:
		defer func() { <-t.inFlight }()
	default:
		w.Header().Set("Retry-After", "1")
		emptyReply(w, http.StatusServiceUnavailable)
		return
	}

	// A message without "method" is a client's response to a server request; this server sends
	// none, so there is nothing to do with it. A message without "id" is a notification. Both get
	// 202 and no body. A body that is not one JSON object goes to the handler, which answers with
	// the JSON-RPC parse error, as it does on stdio.
	var shape struct {
		ID     json.RawMessage `json:"id"`
		Method *string         `json:"method"`
	}
	parsed := json.Unmarshal(body, &shape) == nil
	if parsed && shape.Method == nil {
		emptyReply(w, http.StatusAccepted)
		return
	}
	resp := t.server.handle(string(body))
	if resp == nil || (parsed && len(shape.ID) == 0) {
		emptyReply(w, http.StatusAccepted)
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		emptyReply(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if !parsed {
		w.WriteHeader(http.StatusBadRequest)
	}
	_, _ = w.Write(data)
}

// newHTTPServer is the http.Server of the endpoint, with every limit a slow or greedy client meets.
func newHTTPServer(h http.Handler, logw io.Writer) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaxHeader,
		ErrorLog:          log.New(logw, "probe-mcp http: ", 0),
	}
}

// RunHTTP serves the MCP tools over HTTP until ctx is done. It returns an error without listening
// when the address or the token is refused.
func RunHTTP(ctx context.Context, opts HTTPOptions) error {
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	addr, err := resolveHTTPAddr(opts.Addr, opts.AllowNonLoopback)
	if err != nil {
		return err
	}
	token := opts.Token
	if token != "" {
		if err := checkSuppliedToken(token); err != nil {
			return err
		}
	}

	// Listen before touching the token file: a start that fails because the port is taken must not
	// replace, and then remove, the token file of the server that holds the port.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("mcp http: listen: %w", err)
	}
	defer ln.Close() // a second Close after Serve has closed it is harmless
	bound := ln.Addr().(*net.TCPAddr)

	tokenFile, replaced := "", false
	if opts.Token == "" {
		dir := opts.TokenDir
		if dir == "" {
			if dir, err = DefaultTokenDir(); err != nil {
				return err
			}
		}
		if token, err = newToken(); err != nil {
			return err
		}
		if tokenFile, replaced, err = writeTokenFile(dir, token); err != nil {
			return err
		}
		defer removeTokenFile(tokenFile, token)
	}

	s := &Server{}
	// Never leave a background test run behind when the server stops.
	defer s.stopJobs()
	srv := newHTTPServer(newHTTPTransport(s, token, bound, opts.AllowNonLoopback), logw)

	fmt.Fprintf(logw, "probe-mcp: MCP over HTTP at http://%s%s\n", bound.String(), HTTPEndpointPath)
	if tokenFile != "" {
		fmt.Fprintf(logw, "probe-mcp: bearer token written to %s (a new one on every start)\n", tokenFile)
		if replaced {
			fmt.Fprintf(logw, "probe-mcp: note: a token file was already there and has been replaced. If another probe-mcp HTTP server is still running, its token is no longer on disk; run one per user config directory, or start the second with %s.\n", TokenEnv)
		}
	} else {
		fmt.Fprintf(logw, "probe-mcp: bearer token taken from %s\n", TokenEnv)
	}
	if !bound.IP.IsLoopback() {
		fmt.Fprintf(logw, "probe-mcp: WARNING: %s is not a loopback address. Other machines can reach this server; the connection is not encrypted, so the token and every test file cross the network in the clear.\n", bound.String())
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	select {
	case err := <-done:
		return fmt.Errorf("mcp http: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
		}
		return nil
	}
}
