package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// WebViewPage is what the WebView steps use of an open page; *device.WebViewSession is one, and tests replace it.
type WebViewPage interface {
	Tap(ctx context.Context, sel device.Selector) error
	Type(ctx context.Context, sel device.Selector, text string) error
	See(ctx context.Context, sel device.Selector) error
	WaitFor(ctx context.Context, sel device.Selector, timeout time.Duration) error
	Title() string
}

// WebViewOpener opens the page for one step, runs fn with it and always closes it again. A nil WebViewOpener in the DeviceContext
// means device.WithWebView.
type WebViewOpener func(ctx context.Context, o device.WebViewOptions, fn func(WebViewPage) error) error

func openWebViewPage(ctx context.Context, o device.WebViewOptions, fn func(WebViewPage) error) error {
	return device.WithWebView(ctx, o, func(s *device.WebViewSession) error { return fn(s) })
}

// defaultWebViewWait is the total a `wait until ... in webview` waits when the step has no timeout of its own.
const defaultWebViewWait = 30 * time.Second

// runWebViewStep runs one WebView step. The page is opened for this step only and always closed (fail closed, nothing to leak between
// steps). Every text that came from the script is resolved here (variables, ${ENV}) and every text that came from the page is masked by
// the session with the run's secret masking, which is required.
func (e *Executor) runWebViewStep(ctx context.Context, what string, t *parser.WebViewTarget, fn func(p WebViewPage, sel device.Selector) error) error {
	dc := e.deviceCtx
	if dc == nil {
		fmt.Printf("    \033[33m⚠\033[0m  Skipping %s (cloud mode)\n", what)
		return nil
	}
	if dc.Platform != device.PlatformAndroid {
		return fmt.Errorf("%s: only supported on Android (the WebView of an iOS app is not reachable yet, see docs/proposals/webview-content-automation.md)", what)
	}
	var runner device.AdbRunner = dc.WebViewADB
	if runner == nil && dc.Manager != nil {
		runner = dc.Manager.ADB()
	}
	if runner == nil {
		return fmt.Errorf("%s: no adb is available for this device", what)
	}
	if dc.AppID == "" {
		return fmt.Errorf("%s: the app id of this run is not known; set `app` under `project` in probe.yaml", what)
	}
	sel := device.Selector{Kind: device.SelectText, Value: e.resolve(t.Value)}
	if t.Kind == parser.WebViewCSS {
		sel.Kind = device.SelectCSS
	}
	opts := device.WebViewOptions{
		Runner:      runner,
		Serial:      dc.Serial,
		AppID:       dc.AppID,
		Page:        e.resolve(t.Page),
		CallTimeout: e.timeout,
		Redact:      sysdialog.ScrubKnown,
	}
	open := dc.WebViewOpen
	if open == nil {
		open = openWebViewPage
	}
	err := open(ctx, opts, func(p WebViewPage) error { return fn(p, sel) })
	if err != nil {
		return &webViewStepError{what: what, err: e.scrubEnv(err)}
	}
	return nil
}

// webViewStepError is a failure of a WebView step. The message holds text the page supplied and the script's own selector, so nothing may
// classify the error by its text: isConnectionError, isNotFoundError and the retry paths check for this type and leave it alone. A WebView
// step never reconnects the agent and is never retried by the implicit wait; the error keeps its device.WebViewError for errors.As.
type webViewStepError struct {
	what string
	err  error
}

func (e *webViewStepError) Error() string { return e.what + ": " + e.err.Error() }
func (e *webViewStepError) Unwrap() error { return e.err }

// isWebViewStepError reports whether err comes from a WebView step.
func isWebViewStepError(err error) bool {
	var w *webViewStepError
	return errors.As(err, &w)
}

// isWebViewStep reports whether step is one of the WebView steps (also inside a timed step).
func isWebViewStep(step parser.Step) bool {
	switch s := step.(type) {
	case parser.ActionStep:
		return s.Verb == parser.VerbTapWebView || s.Verb == parser.VerbTypeWebView
	case parser.AssertStep:
		return s.WebView != nil
	case parser.WaitStep:
		return s.Kind == parser.WaitWebView
	case parser.TimedStep:
		return isWebViewStep(s.Inner)
	}
	return false
}

func (e *Executor) runWebViewTap(ctx context.Context, a parser.ActionStep) error {
	return e.runWebViewStep(ctx, "tap in webview", a.WebView, func(p WebViewPage, sel device.Selector) error {
		return p.Tap(ctx, sel)
	})
}

func (e *Executor) runWebViewType(ctx context.Context, a parser.ActionStep) error {
	text := e.resolve(a.Text)
	return e.runWebViewStep(ctx, "type in webview", a.WebView, func(p WebViewPage, sel device.Selector) error {
		return p.Type(ctx, sel, text)
	})
}

// runWebViewSee is `see` and `don't see`. A page that cannot be reached is an error for both: "not seen" must not be the answer to a
// WebView that was never looked at.
func (e *Executor) runWebViewSee(ctx context.Context, a parser.AssertStep) error {
	what := "see in webview"
	if a.Negated {
		what = "don't see in webview"
	}
	return e.runWebViewStep(ctx, what, a.WebView, func(p WebViewPage, sel device.Selector) error {
		err := p.See(ctx, sel)
		if !a.Negated {
			return err
		}
		var we *device.WebViewError
		if errors.As(err, &we) && (we.Kind == "not found" || we.Kind == "not visible") {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("expected not to see %q in the WebView page %q, but a visible element matches", capShown(e.scrubText(sel.Value)), p.Title())
	})
}

func (e *Executor) runWebViewWait(ctx context.Context, w parser.WaitStep) error {
	total := e.timeout
	if total <= 0 {
		total = defaultWebViewWait
	}
	return e.runWebViewStep(ctx, "wait until in webview", w.WebView, func(p WebViewPage, sel device.Selector) error {
		err := p.WaitFor(ctx, sel, total)
		// the step's own deadline can end the wait first (the wait's timer starts after the page is opened): say what was waited for
		if err != nil && errors.Is(err, context.DeadlineExceeded) {
			return &device.WebViewError{Kind: "not found", Message: fmt.Sprintf("Timed out waiting for %q in the WebView page %q", capShown(e.scrubText(sel.Value)), p.Title())}
		}
		return err
	})
}

// scrubText masks the values the run knows are secrets in a text that is about to be shown.
func (e *Executor) scrubText(s string) string { return sysdialog.ScrubKnown(s) }

// webViewShown is the target as a step line shows it: variables resolved, then the values the run knows are secrets masked, like the text of
// a `type` step. The page name gets the same treatment, since a variable can hold anything.
func (e *Executor) webViewShown(t *parser.WebViewTarget) parser.WebViewTarget {
	return parser.WebViewTarget{
		Kind:  t.Kind,
		Value: capShown(sysdialog.ScrubKnown(e.resolveVars(t.Value))),
		Page:  capShown(sysdialog.ScrubKnown(e.resolveVars(t.Page))),
	}
}

// maxShownTarget is the longest target or page name a step line or message shows; a variable can hold any amount of text.
const maxShownTarget = 200

func capShown(s string) string {
	r := []rune(s)
	if len(r) <= maxShownTarget {
		return s
	}
	return string(r[:maxShownTarget]) + "…"
}

// errStoreWebViewNotImplemented is what `store text of css:"..." in webview as name` reports until its runner part exists: the step parses, it does not run yet.
var errStoreWebViewNotImplemented = errors.New("store text of ... in webview is not implemented yet in this version of probe")
