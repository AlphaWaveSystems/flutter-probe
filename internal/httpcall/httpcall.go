// Package httpcall makes one HTTP request on behalf of a person, with the limits that a request typed into a tool needs: only http and
// https, a method and header allow-list, a size cap on what is sent and what comes back, a time limit, redirects that never leave the
// origin that was asked, no proxy from the environment, and a policy that is asked about every address that is about to be dialed, after
// the name is resolved, so a name that resolves somewhere else than it looked cannot get around it.
//
// It writes nothing to disk, logs nothing, and keeps nothing between calls. It does not decide who may be called: the Policy does, and
// a caller that has no policy allows every address.
package httpcall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The limits, and what the options default to.
const (
	DefaultTimeout     = 30 * time.Second
	MaxTimeout         = 120 * time.Second
	DefaultMaxResponse = 1 << 20 // 1 MB
	MaxMaxResponse     = 8 << 20
	DefaultMaxRequest  = 256 << 10
	DefaultRedirects   = 5
	MaxRedirects       = 10
	MaxURLBytes        = 8 << 10
	MaxHeaders         = 50
	MaxHeaderBytes     = 8 << 10

	// UserAgent is sent with every request. A caller cannot change it.
	UserAgent = "FlutterProbe-Studio"
)

// The errors a caller can tell apart. None of them carries what was sent or received.
var (
	ErrScheme           = errors.New("only http and https can be called")
	ErrURL              = errors.New("the address is not a valid http or https address")
	ErrUserInfo         = errors.New("an address with a user name or password is refused")
	ErrMethod           = errors.New("that method is not allowed")
	ErrHeader           = errors.New("a header is not allowed")
	ErrRequestTooLarge  = errors.New("the request is larger than the limit")
	ErrBodyNotAllowed   = errors.New("this method sends no body")
	ErrTooManyRedirects = errors.New("too many redirects")
	ErrPolicy           = errors.New("the address is not allowed")
	// ErrIncomplete: the connection ended or failed after the response began, so the body that arrived is not the whole of it. No
	// partial body is returned. A body cut at Options.MaxResponseBytes is not this: it is Response.Truncated.
	ErrIncomplete = errors.New("the response ended before it was complete")
)

// PolicyError is what Do returns when the Policy refused an address. It matches ErrPolicy, and says what kind of address was refused,
// so a caller can tell a person "a private network was refused". It holds no address and none of the policy's own text.
type PolicyError struct{ Class Class }

func (e *PolicyError) Error() string { return "the address is not allowed (" + e.Class.String() + ")" }

// Is makes errors.Is(err, ErrPolicy) true.
func (e *PolicyError) Is(target error) bool { return target == ErrPolicy }

// Request is what is sent.
type Request struct {
	Method string
	URL    string
	Header map[string]string
	Body   string
}

// Resolver turns a name into addresses. net.DefaultResolver is one.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// Options bound a call. Zero values choose the defaults above.
type Options struct {
	Timeout          time.Duration
	MaxResponseBytes int64
	MaxRequestBytes  int
	MaxRedirects     int
	// Policy is called for every address that is about to be dialed, with the host of the request (lower case, no trailing dot, a name in
	// its ASCII form), its port and the address, and returns an error to refuse it. A name with several addresses is dialed only when
	// the policy allows all of them. A nil Policy allows all.
	Policy func(host string, port int, ip net.IP) error
	// Resolver is used for names. The default is net.DefaultResolver. A name is resolved once per connection, and the answer is what is
	// checked and dialed.
	Resolver Resolver
	// Dial connects to an address the policy has allowed. The default is a net.Dialer with a 10 second timeout. It is called only after the
	// policy, with the checked address; a test uses it to answer without a real connection, and a caller must not use it to dial anything
	// else.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
}

func (o Options) normalized() Options {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Timeout > MaxTimeout {
		o.Timeout = MaxTimeout
	}
	if o.MaxResponseBytes <= 0 {
		o.MaxResponseBytes = DefaultMaxResponse
	}
	if o.MaxResponseBytes > MaxMaxResponse {
		o.MaxResponseBytes = MaxMaxResponse
	}
	if o.MaxRequestBytes <= 0 {
		o.MaxRequestBytes = DefaultMaxRequest
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = DefaultRedirects
	}
	if o.MaxRedirects > MaxRedirects {
		o.MaxRedirects = MaxRedirects
	}
	if o.Resolver == nil {
		o.Resolver = net.DefaultResolver
	}
	return o
}

// Hop is one redirect that was followed, or the one that was not.
type Hop struct {
	From   string
	To     string
	Status int
}

// Response is what came back.
type Response struct {
	Status    int
	Header    http.Header
	Body      []byte // at most Options.MaxResponseBytes
	Truncated bool   // the body was longer than that
	URL       string // the address that answered, after the redirects that were followed
	Hops      []Hop  // the redirects that were followed
	// Stopped is a redirect to another origin (another scheme, host or port), which is reported and not followed. Status and Header are
	// those of the response that asked for it.
	Stopped  *Hop
	Duration time.Duration
	RemoteIP string // the address that was dialed for the last connection
}

var allowedMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

var bodyMethods = map[string]bool{"POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// forbiddenHeaders are set by the transport or change how a request is framed or routed.
var forbiddenHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true, "connection": true, "upgrade": true, "te": true, "trailer": true,
	"proxy-connection": true, "proxy-authorization": true, "keep-alive": true, "expect": true,
	"user-agent":      true, // fixed
	"accept-encoding": true, // the transport asks for gzip and decodes it, under the size cap; a caller-set one would return raw bytes
}

var tokenPattern = regexp.MustCompile("^[!#$%&'*+\\-.^_`|~0-9A-Za-z]+$")

// validate checks a request and returns the URL it is for.
func validate(r Request, o Options) (*url.URL, string, error) {
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if !allowedMethods[method] {
		return nil, "", ErrMethod
	}
	if len(r.URL) > MaxURLBytes || strings.ContainsAny(r.URL, "\r\n\x00 \t") {
		return nil, "", ErrURL
	}
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, "", ErrURL
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	case "":
		return nil, "", ErrURL
	default:
		return nil, "", ErrScheme
	}
	if u.Host == "" || u.Hostname() == "" || u.Opaque != "" || strings.Contains(u.Hostname(), "%") {
		return nil, "", ErrURL // a zone (fe80::1%eth0) is refused: the zone is what selects the interface
	}
	if !validPort(u.Port()) {
		return nil, "", ErrURL
	}
	if u.User != nil || strings.Contains(u.Host, "@") {
		return nil, "", ErrUserInfo
	}
	u.Fragment = ""
	if len(r.Body) > o.MaxRequestBytes {
		return nil, "", ErrRequestTooLarge
	}
	if r.Body != "" && !bodyMethods[method] {
		return nil, "", ErrBodyNotAllowed
	}
	if len(r.Header) > MaxHeaders {
		return nil, "", ErrHeader
	}
	for name, value := range r.Header {
		switch {
		case !tokenPattern.MatchString(name), forbiddenHeaders[strings.ToLower(name)]:
			return nil, "", ErrHeader
		case len(value) > MaxHeaderBytes, strings.ContainsAny(value, "\r\n\x00"):
			return nil, "", ErrHeader
		}
	}
	return u, method, nil
}

// validPort accepts no port (the scheme's) or a number from 1 to 65535.
func validPort(p string) bool {
	if p == "" {
		return true
	}
	n, err := strconv.Atoi(p)
	return err == nil && n >= 1 && n <= 65535 && len(p) <= 5
}

func origin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

// Do makes the request. A refused request, a refused address, a timeout and a failed connection are errors; an HTTP error status is not.
func Do(ctx context.Context, req Request, opts Options) (Response, error) {
	o := opts.normalized()
	u, method, err := validate(req, o)
	if err != nil {
		return Response{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	var res Response
	transport := newTransport(o, &res.RemoteIP)
	defer transport.CloseIdleConnections()

	start := origin(u)
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			last := via[len(via)-1]
			hop := Hop{From: last.URL.String(), To: r.URL.String()}
			if r.Response != nil {
				hop.Status = r.Response.StatusCode
			}
			if s := strings.ToLower(r.URL.Scheme); s != "http" && s != "https" {
				return ErrScheme
			}
			if r.URL.User != nil {
				return ErrUserInfo
			}
			if !validPort(r.URL.Port()) {
				return ErrURL
			}
			if origin(r.URL) != start {
				res.Stopped = &hop
				return http.ErrUseLastResponse
			}
			if len(via) > o.MaxRedirects {
				return ErrTooManyRedirects
			}
			res.Hops = append(res.Hops, hop)
			return nil
		},
	}

	hreq, err := http.NewRequestWithContext(ctx, method, u.String(), strings.NewReader(req.Body))
	if err != nil {
		return Response{}, ErrURL
	}
	hreq.Header.Set("User-Agent", UserAgent)
	if req.Body == "" {
		hreq.Body = http.NoBody
		hreq.ContentLength = 0
	}
	names := make([]string, 0, len(req.Header))
	for n := range req.Header {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		hreq.Header.Set(n, req.Header[n])
	}

	began := time.Now()
	resp, err := client.Do(hreq)
	if err != nil {
		return Response{}, unwrap(err)
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	res.Header = resp.Header
	res.URL = resp.Request.URL.String()
	if method != "HEAD" {
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, o.MaxResponseBytes+1))
		if rerr != nil {
			// A read that failed part way (a stall until the time limit, a body shorter than its Content-Length, broken chunk framing) is
			// an error, never a shorter response that looks complete.
			if e := unwrap(rerr); e != errFailed {
				return Response{}, e
			}
			return Response{}, ErrIncomplete
		}
		if int64(len(body)) > o.MaxResponseBytes {
			body, res.Truncated = body[:o.MaxResponseBytes], true
		}
		res.Body = body
	}
	res.Duration = time.Since(began)
	return res, nil
}

// unwrap returns the sentinel an error holds, or a short sentence, so no address or text from the transport is passed on.
func unwrap(err error) error {
	var pe *PolicyError
	if errors.As(err, &pe) {
		return pe
	}
	for _, sentinel := range []error{ErrScheme, ErrURL, ErrUserInfo, ErrTooManyRedirects, ErrPolicy} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("the request took longer than the time limit")
	case errors.Is(err, context.Canceled):
		return errors.New("the request was cancelled")
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errors.New("the request took longer than the time limit")
	}
	return errFailed
}

var errFailed = errors.New("the request failed")

// newTransport is the transport of one call: no proxy, no reuse of connections, and a dialer that asks the policy about every address
// before it connects. The address that was dialed is written to remote.
func newTransport(o Options, remote *string) *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dial := o.Dial
	if dial == nil {
		dial = dialer.DialContext
	}
	return &http.Transport{
		Proxy:                  nil, // never the environment's: a proxy would be asked the name instead of the policy being asked the address
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  o.Timeout,
		MaxResponseHeaderBytes: 64 << 10,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, portText, err := net.SplitHostPort(addr)
			port, perr := strconv.Atoi(portText)
			if err != nil || perr != nil || port < 1 || port > 65535 {
				return nil, ErrURL
			}
			host = strings.TrimSuffix(strings.ToLower(host), ".")
			var ips []net.IP
			if ip := net.ParseIP(host); ip != nil {
				ips = []net.IP{ip}
			} else {
				addrs, err := o.Resolver.LookupIPAddr(ctx, host) // once: what is checked is what is dialed
				if err != nil || len(addrs) == 0 {
					return nil, fmt.Errorf("the name could not be resolved")
				}
				for _, a := range addrs {
					ips = append(ips, a.IP)
				}
			}
			// Every address is checked before any is dialed, and one refusal refuses the name: a name that answers with an allowed and a
			// refused address is not dialed at all.
			if o.Policy != nil {
				for _, ip := range ips {
					if o.Policy(host, port, ip) != nil {
						return nil, &PolicyError{Class: ClassOf(ip)}
					}
				}
			}
			for _, ip := range ips {
				conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), portText))
				if err != nil {
					continue
				}
				*remote = ip.String()
				return conn, nil
			}
			return nil, fmt.Errorf("the connection failed")
		},
	}
}
