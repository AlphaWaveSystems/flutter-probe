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
)

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
	// Policy is called for each address that is about to be dialed, with the host name or literal of the request and the address, and
	// returns an error to refuse it. The first allowed address is dialed. A nil Policy allows all.
	Policy func(host string, ip net.IP) error
	// Resolver is used for names. The default is net.DefaultResolver. A name is resolved once per connection, and the answer is what is
	// checked and dialed.
	Resolver Resolver
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
	if u.Host == "" || u.Hostname() == "" || u.Opaque != "" {
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
		if rerr != nil && len(body) == 0 {
			return Response{}, unwrap(rerr)
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
	return errors.New("the request failed")
}

// newTransport is the transport of one call: no proxy, no reuse of connections, and a dialer that asks the policy about every address
// before it connects. The address that was dialed is written to remote.
func newTransport(o Options, remote *string) *http.Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		Proxy:                  nil, // never the environment's: a proxy would be asked the name instead of the policy being asked the address
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  o.Timeout,
		MaxResponseHeaderBytes: 64 << 10,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, ErrURL
			}
			var ips []net.IP
			if ip := net.ParseIP(host); ip != nil {
				ips = []net.IP{ip}
			} else {
				addrs, err := o.Resolver.LookupIPAddr(ctx, host) // once: what is checked is what is dialed
				if err != nil {
					return nil, fmt.Errorf("the name could not be resolved")
				}
				for _, a := range addrs {
					ips = append(ips, a.IP)
				}
			}
			var refused error
			for _, ip := range ips {
				if o.Policy != nil {
					if perr := o.Policy(host, ip); perr != nil {
						refused = perr
						continue
					}
				}
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err != nil {
					continue
				}
				*remote = ip.String()
				return conn, nil
			}
			if refused != nil {
				return nil, fmt.Errorf("%w: %v", ErrPolicy, refused)
			}
			return nil, fmt.Errorf("the connection failed")
		},
	}
}
