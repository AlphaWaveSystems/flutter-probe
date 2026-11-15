package main

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/httpcall"
)

// Studio's Send request panel: one request typed by a person, sent from this machine with the limits of internal/httpcall, and shown
// through the redactor of the Network tab. The host has to be allowed first (CheckTarget, then AllowHost), once per session. Nothing
// is written to disk and nothing is sent to an AI provider.

// SendHeader is one header typed in the panel. The table is Studio's only: it is not saved to a step.
type SendHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SendInput is the request.
type SendInput struct {
	Method  string       `json:"method"`
	URL     string       `json:"url"`
	Headers []SendHeader `json:"headers"`
	Body    string       `json:"body"`
}

// TargetInfo says where a request would go, for the confirmation shown before a host is allowed.
type TargetInfo struct {
	Host      string   `json:"host"`
	Label     string   `json:"label,omitempty"` // the name as typed, when it is written in unicode (Host is its ASCII form)
	Port      string   `json:"port"`
	Key       string   `json:"key"`       // host:port in the normal form (an IPv6 address in brackets), what AllowHost takes
	Allowed   bool     `json:"allowed"`   // already allowed in this session
	Addresses []string `json:"addresses"` // what the name resolves to now
	Classes   []string `json:"classes"`   // the kind of each address, in words
	Local     bool     `json:"local"`     // any address is on this machine or its network: show the stronger warning
	Refused   string   `json:"refused,omitempty"`
}

// SendHop is a redirect that was followed or reported.
type SendHop struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status"`
}

// SendResult is what came back, redacted like the Network tab shows an exchange.
type SendResult struct {
	Sent            string      `json:"sent"` // "GET https://host/path?name=[redacted]"
	RequestHeaders  []NetHeader `json:"requestHeaders"`
	Status          int         `json:"status"`
	DurationMs      int64       `json:"durationMs"`
	URL             string      `json:"url"` // the address that answered
	Hops            []SendHop   `json:"hops"`
	Stopped         *SendHop    `json:"stopped,omitempty"` // a redirect to another origin that was not followed
	StoppedHost     string      `json:"stoppedHost,omitempty"`
	ResponseHeaders []NetHeader `json:"responseHeaders"`
	MoreHeaders     int         `json:"moreHeaders,omitempty"`
	ResponseBody    NetBody     `json:"responseBody"`
	Truncated       bool        `json:"truncated,omitempty"` // the body was longer than the 1 MB that is read
	RemoteClass     string      `json:"remoteClass"`
}

func splitTarget(raw string) (u *url.URL, host, port string, err error) {
	if len(raw) > httpcall.MaxURLBytes {
		return nil, "", "", errors.New("the address is too long")
	}
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil || u.Host == "" || u.Hostname() == "" {
		return nil, "", "", errors.New("the address is not a valid http or https address")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, "", "", errors.New("only http and https can be called")
	}
	if u.User != nil {
		return nil, "", "", errors.New("an address with a user name or password is refused")
	}
	port = u.Port()
	if port == "" {
		port = map[bool]string{true: "443", false: "80"}[strings.EqualFold(u.Scheme, "https")]
	}
	if n, nerr := strconv.Atoi(port); nerr != nil || n < 1 || n > 65535 {
		return nil, "", "", errors.New("the address is not a valid http or https address")
	}
	host, herr := asciiHost(u.Hostname())
	if herr != nil {
		return nil, "", "", errors.New("the address is not a valid http or https address")
	}
	return u, host, port, nil
}

// CheckTarget says where a request to rawURL would go, and whether its host is allowed yet. It looks the name up once (at most thirty
// times a minute), so the person is shown what it resolves to before saying yes, and it remembers how strong the strongest address
// was: an allow is for what was shown. It sends no request.
func (a *App) CheckTarget(rawURL string) (TargetInfo, error) {
	_, host, port, err := splitTarget(rawURL)
	if err != nil {
		return TargetInfo{}, err
	}
	if err := a.send.checkAllowed(); err != nil {
		return TargetInfo{}, err
	}
	key := hostKey(host, port)
	info := TargetInfo{Host: host, Port: port, Key: key, Addresses: []string{}, Classes: []string{}}
	if u, perr := url.Parse(strings.TrimSpace(rawURL)); perr == nil && !strings.EqualFold(u.Hostname(), host) {
		info.Label = shortText(strings.ToLower(u.Hostname()), 200)
	}
	ips, err := a.send.resolveForDisplay(host)
	if err != nil {
		info.Refused = err.Error()
		return info, nil
	}
	level := 0
	for _, ip := range ips {
		class := httpcall.ClassOf(ip)
		info.Addresses = append(info.Addresses, ip.String())
		info.Classes = append(info.Classes, class.String())
		if class.Local() {
			info.Local = true
		}
		if l := levelOf(class); l > level {
			level = l
		}
		if perr := a.send.classPolicy(host, ip); perr != nil && info.Refused == "" {
			info.Refused = perr.Error()
		}
	}
	if level > 0 && level < levelNever && info.Refused == "" {
		a.send.note(key, level)
		info.Allowed = a.send.allowedLevel(key) >= level
	}
	return info, nil
}

// AllowHost allows requests to a host:port for the rest of the session, at the strength CheckTarget showed for it. It is the person's
// decision, made in the window after CheckTarget showed where the host leads; a host that was not checked, or that leads somewhere that
// is never allowed, is refused, and so is a key that is not in the normal form CheckTarget returns.
func (a *App) AllowHost(key string) error {
	host, port, err := parseKey(key)
	if err != nil {
		return err
	}
	return a.send.allow(hostKey(host, port))
}

var sendErrors = map[error]string{
	httpcall.ErrScheme:           "only http and https can be called",
	httpcall.ErrURL:              "the address is not a valid http or https address",
	httpcall.ErrUserInfo:         "an address with a user name or password is refused",
	httpcall.ErrMethod:           "that method is not allowed",
	httpcall.ErrHeader:           "a header is not allowed (Host, Content-Length, User-Agent, Accept-Encoding and the connection headers are set by Studio)",
	httpcall.ErrIncomplete:       "the response ended before it was complete, so none of it is shown",
	httpcall.ErrRequestTooLarge:  "the request is larger than 256 KB",
	httpcall.ErrBodyNotAllowed:   "this method sends no body",
	httpcall.ErrTooManyRedirects: "too many redirects",
	httpcall.ErrPolicy:           "that address is not allowed from here",
}

// SendRequest sends the request, if its host has been allowed, and returns the answer redacted. At most one request runs at a time and
// at most ten a minute. A refused request sends nothing. What was typed is replaced wherever the answer echoes it.
func (a *App) SendRequest(in SendInput) (SendResult, error) {
	u, host, port, err := splitTarget(in.URL)
	if err != nil {
		return SendResult{}, err
	}
	if !a.send.isAllowed(hostKey(host, port)) {
		return SendResult{}, errSendHostNotAllowed
	}
	headers := map[string]string{}
	for _, h := range in.Headers {
		if strings.TrimSpace(h.Name) != "" {
			headers[strings.TrimSpace(h.Name)] = h.Value
		}
	}
	if err := a.send.begin(); err != nil {
		return SendResult{}, err
	}
	defer a.send.end()
	a.send.takeRefusal()

	res, err := httpcall.Do(context.Background(), httpcall.Request{Method: in.Method, URL: u.String(), Header: headers, Body: in.Body},
		httpcall.Options{Policy: a.send.policy, Resolver: a.send.resolver, Dial: a.send.dial})
	if err != nil {
		var pe *httpcall.PolicyError
		if errors.As(err, &pe) {
			if reason := a.send.takeRefusal(); reason != "" {
				return SendResult{}, errors.New("that address is not allowed from here: " + reason)
			}
			return SendResult{}, errors.New("that address is " + pe.Class.String() + ", which is not allowed from here")
		}
		for sentinel, text := range sendErrors {
			if errors.Is(err, sentinel) {
				return SendResult{}, errors.New(text)
			}
		}
		return SendResult{}, errors.New(err.Error()) // already a short sentence without an address or a header
	}
	typed := collectTyped(u.String(), headers, in.Body)
	return viewSendResult(strings.ToUpper(strings.TrimSpace(in.Method)), u.String(), headers, res, typed), nil
}

func flattenHeader(h map[string][]string) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func viewSendResult(method, rawURL string, reqHeaders map[string]string, res httpcall.Response, typed *typedValues) SendResult {
	out := SendResult{
		Sent:        method + " " + redactURL(typed.scrub(rawURL)),
		Status:      res.Status,
		DurationMs:  res.Duration.Milliseconds(),
		URL:         redactURL(typed.scrub(res.URL)),
		Hops:        []SendHop{},
		Truncated:   res.Truncated,
		RemoteClass: remoteClass(res.RemoteIP),
	}
	scrubbed := make(map[string]string, len(reqHeaders))
	for k, v := range reqHeaders {
		scrubbed[k] = typed.scrub(v)
	}
	out.RequestHeaders, _ = redactHeaders(scrubbed)
	for _, h := range res.Hops {
		out.Hops = append(out.Hops, SendHop{From: redactURL(typed.scrub(h.From)), To: redactURL(typed.scrub(h.To)), Status: h.Status})
	}
	if res.Stopped != nil {
		out.Stopped = &SendHop{From: redactURL(typed.scrub(res.Stopped.From)), To: redactURL(typed.scrub(res.Stopped.To)), Status: res.Stopped.Status}
		if u, err := url.Parse(res.Stopped.To); err == nil {
			out.StoppedHost = shortText(u.Host, 200)
		}
	}
	budget := newNetBudget()
	echoed := flattenHeader(res.Header)
	for k, v := range echoed {
		echoed[k] = typed.scrub(v)
	}
	out.ResponseHeaders, out.MoreHeaders = redactHeadersBudget(echoed, budget)
	sort.SliceStable(out.ResponseHeaders, func(i, j int) bool { return out.ResponseHeaders[i].Name < out.ResponseHeaders[j].Name })
	out.ResponseBody = redactBodyBudget(typed.scrub(string(res.Body)), int64(len(res.Body)), res.Header.Get("Content-Type"), budget)
	return out
}

func remoteClass(ip string) string {
	if ip == "" {
		return ""
	}
	parsed, err := parseIP(ip)
	if err != nil {
		return ""
	}
	return httpcall.ClassOf(parsed).String()
}
