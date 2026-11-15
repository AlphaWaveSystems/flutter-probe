package device

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appIDPattern is the shape of an Android application id; the id is passed to the device's shell, so nothing else is accepted.
var appIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)+$`)

// WebViewOptions says which WebView to reach. Runner and Serial name the device; the app is named by its process id (PID) or, when PID is 0, by its app id (AppID), whose process id is asked of the device.
// Page is an optional part of the title or the URL of the page to use when the app has more than one.
type WebViewOptions struct {
	Runner AdbRunner
	Serial string
	AppID  string
	PID    int
	Page   string
	// CallTimeout ends a command whose context has no deadline; zero means 30 s. Set it higher for a step that may take long.
	CallTimeout time.Duration
	// Redact is applied to every piece of text that came from the page (the page's error messages, titles and element text) before it is put in a message. The runner passes its secret masking here;
	// nil leaves the text as it is (already cleaned of control characters and shortened).
	Redact func(string) string
}

// WebViewSession is an open connection to one page of one app's WebView, with the adb forward it runs through. It fails closed: when it cannot be opened, whatever was created is removed, and Close removes
// both the connection and the forward.
type WebViewSession struct {
	conn   *CDPConn
	fwd    *WebViewForward
	title  string
	redact func(string) string

	mu     sync.Mutex
	closed bool
}

// OpenWebView finds the DevTools socket of the app's own process (never another process's), forwards it to a loopback port, lists the pages, picks one and connects to it. The messages follow
// docs/proposals/webview-content-automation.md and never contain the debugger address or page content beyond the matched title.
func OpenWebView(ctx context.Context, o WebViewOptions) (*WebViewSession, error) {
	if o.Runner == nil || strings.TrimSpace(o.Serial) == "" {
		return nil, fmt.Errorf("webview: a device serial and an adb runner are required")
	}
	pid := o.PID
	if pid == 0 {
		if o.AppID == "" {
			return nil, fmt.Errorf("webview: the app's process id or app id is required")
		}
		if !appIDPattern.MatchString(o.AppID) {
			return nil, fmt.Errorf("webview: %q is not an application id such as com.example.app", shown(o.AppID))
		}
		out, err := o.Runner.RunArgs(ctx, "-s", o.Serial, "shell", "pidof", o.AppID)
		if err != nil {
			return nil, &WebViewError{Kind: "no webview", Message: "The app is not running on the device, so it has no WebView to reach"}
		}
		f := strings.Fields(string(out))
		if len(f) == 0 {
			return nil, &WebViewError{Kind: "no webview", Message: "The app is not running on the device, so it has no WebView to reach"}
		}
		pid, err = strconv.Atoi(f[0])
		if err != nil || pid <= 0 {
			return nil, &WebViewError{Kind: "no webview", Message: "The app is not running on the device, so it has no WebView to reach"}
		}
	}
	table, err := o.Runner.RunArgs(ctx, "-s", o.Serial, "shell", "cat", "/proc/net/unix")
	if err != nil {
		return nil, fmt.Errorf("webview: the device's socket table could not be read")
	}
	sock, err := OwnWebViewSocket(string(table), pid)
	if err != nil {
		return nil, &WebViewError{Kind: "no webview", Message: err.Error()}
	}
	fwd, err := ForwardWebView(ctx, o.Runner, o.Serial, sock)
	if err != nil {
		return nil, err
	}
	redact := o.Redact
	if redact == nil {
		redact = func(s string) string { return s }
	}
	s, err := openForwarded(ctx, fwd, o.Page, redact, o.CallTimeout)
	if err != nil {
		// fail closed: the forward made for this attempt is removed; a failed removal is added to the error
		if cerr := fwd.Close(); cerr != nil {
			err = fmt.Errorf("%w; also: %v", err, cerr)
		}
		return nil, err
	}
	return s, nil
}

func openForwarded(ctx context.Context, fwd *WebViewForward, page string, redact func(string) string, callTimeout time.Duration) (*WebViewSession, error) {
	pages, err := ListTargets(ctx, fwd.Port())
	if err != nil {
		return nil, err
	}
	target, err := pickPage(pages, page, redact)
	if err != nil {
		return nil, err
	}
	conn, err := DialTarget(ctx, fwd.Port(), target)
	if err != nil {
		return nil, err
	}
	conn.Timeout = callTimeout
	return &WebViewSession{conn: conn, fwd: fwd, title: shown(redact(target.Title)), redact: redact}, nil
}

// pickPage chooses the page to use: the only one, or the one whose title or URL contains page (case-insensitive).
func pickPage(pages []Target, page string, redact func(string) string) (Target, error) {
	if len(pages) == 0 {
		return Target{}, &WebViewError{Kind: "no webview", Message: "No WebView page is open in the app on this screen"}
	}
	page = strings.TrimSpace(page)
	if page == "" {
		if len(pages) == 1 {
			return pages[0], nil
		}
		return Target{}, &WebViewError{Kind: "several pages", Message: fmt.Sprintf("Found %d WebView pages (%s); name one with in webview \"<part of the title or url>\"", len(pages), listTitles(pages, redact))}
	}
	needle := strings.ToLower(page)
	var hit []Target
	for _, p := range pages {
		if strings.Contains(strings.ToLower(p.Title), needle) || strings.Contains(strings.ToLower(p.URL), needle) {
			hit = append(hit, p)
		}
	}
	switch len(hit) {
	case 0:
		return Target{}, &WebViewError{Kind: "no match", Message: fmt.Sprintf("No WebView page matches %q (%d pages open: %s)", shown(redact(page)), len(pages), listTitles(pages, redact))}
	case 1:
		return hit[0], nil
	}
	return Target{}, &WebViewError{Kind: "several pages", Message: fmt.Sprintf("%q matches %d WebView pages (%s); use a longer part of the title or url", shown(redact(page)), len(hit), listTitles(hit, redact))}
}

// listTitles lists up to five page titles, each cleaned and shortened; the URL is never listed.
func listTitles(pages []Target, redact func(string) string) string {
	const most = 5
	var t []string
	for i, p := range pages {
		if i == most {
			t = append(t, fmt.Sprintf("and %d more", len(pages)-most))
			break
		}
		title := shown(redact(p.Title))
		if title == "" {
			title = "untitled"
		}
		t = append(t, strconv.Quote(title))
	}
	return strings.Join(t, ", ")
}

// Title is the page's title as the step messages show it.
func (s *WebViewSession) Title() string { return s.title }

// Close closes the connection and removes the forward. It is safe to call more than once; a removal that failed is tried again by the next Close.
func (s *WebViewSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		_ = s.conn.Close()
	}
	return s.fwd.Close()
}

// WithWebView opens a session, runs fn with it and always closes it, whether fn succeeded, failed or panicked. A failed close is added to fn's error.
func WithWebView(ctx context.Context, o WebViewOptions, fn func(*WebViewSession) error) (err error) {
	s, err := OpenWebView(ctx, o)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := s.Close(); cerr != nil {
			if err == nil {
				err = cerr
			} else {
				err = fmt.Errorf("%w; also: %v", err, cerr)
			}
		}
	}()
	return fn(s)
}
