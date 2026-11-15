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
		return fmt.Errorf("%s: %w", what, e.scrubEnv(err))
	}
	return nil
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
		return fmt.Errorf("expected not to see %q in the WebView page %q, but a visible element matches", e.scrubText(sel.Value), p.Title())
	})
}

func (e *Executor) runWebViewWait(ctx context.Context, w parser.WaitStep) error {
	total := e.timeout
	if total <= 0 {
		total = defaultWebViewWait
	}
	return e.runWebViewStep(ctx, "wait until in webview", w.WebView, func(p WebViewPage, sel device.Selector) error {
		return p.WaitFor(ctx, sel, total)
	})
}

// scrubText masks the values the run knows are secrets in a text that is about to be shown.
func (e *Executor) scrubText(s string) string { return sysdialog.ScrubKnown(s) }

// webViewShown is the target as a step line shows it: variables resolved, then the values the run knows are secrets masked, like the text of
// a `type` step. The page name gets the same treatment, since a variable can hold anything.
func (e *Executor) webViewShown(t *parser.WebViewTarget) parser.WebViewTarget {
	return parser.WebViewTarget{
		Kind:  t.Kind,
		Value: sysdialog.ScrubKnown(e.resolveVars(t.Value)),
		Page:  sysdialog.ScrubKnown(e.resolveVars(t.Page)),
	}
}
