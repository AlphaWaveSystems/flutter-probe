package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// agentClient is an agent connection that records what the executor does with it.
type agentClient struct {
	fakeAIClient
	closes atomic.Int32
	calls  atomic.Int32
}

func (a *agentClient) Close() error { a.closes.Add(1); return nil }
func (a *agentClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	a.calls.Add(1)
	return nil, nil
}

// substrings the executor used to read as "the agent connection broke" or "the widget is not there yet"
var textThatLooksLikeTrouble = []string{
	"EOF", "broken pipe", "connection reset", "connection refused", "use of closed network connection", "i/o timeout",
	"probelink: write: boom", "rpc error", "Widget not found", `Expected to see "x" but it was not found`,
}

// A page title, an element text, a CDP message or the script's own selector can hold any of those words. None of them may take the agent
// reconnect path, retry the step, or panic when there is no agent connection.
func TestWebViewStepErrorsAreClassifiedByTypeNotByText(t *testing.T) {
	for _, text := range textThatLooksLikeTrouble {
		for _, withAgent := range []bool{false, true} {
			for _, inMessage := range []bool{true, false} {
				r := newWebViewRig()
				r.e.implicitWait = 2 * time.Second
				r.e.reconnectBackoff = time.Millisecond
				agent := &agentClient{}
				if withAgent {
					r.e.client = agent
				}
				target := &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay"}
				if inMessage {
					r.page.tapErr = &device.WebViewError{Kind: "page error", Message: `The page "` + text + ` page" failed`}
				} else {
					target.Value = text // the script's own selector appears in the message
					r.page.tapErr = &device.WebViewError{Kind: "not found", Message: fmt.Sprintf("No element in the WebView page %q matches %q", "Checkout", text)}
				}
				start := time.Now()
				err := r.e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbTapWebView, WebView: target})
				if err == nil || !strings.Contains(err.Error(), "tap in webview") {
					t.Fatalf("%q agent=%v: err = %v", text, withAgent, err)
				}
				if len(r.opens) != 1 || len(r.page.calls) != 1 {
					t.Errorf("%q agent=%v: the step ran %d times (%d opens)", text, withAgent, len(r.page.calls), len(r.opens))
				}
				if agent.closes.Load() != 0 || agent.calls.Load() != 0 {
					t.Errorf("%q agent=%v: the agent connection was touched (closes %d, calls %d)", text, withAgent, agent.closes.Load(), agent.calls.Load())
				}
				if time.Since(start) > time.Second {
					t.Errorf("%q agent=%v: the step took %v: it was retried", text, withAgent, time.Since(start))
				}
				var we *device.WebViewError
				if !errors.As(err, &we) {
					t.Errorf("%q: the WebViewError is not reachable with errors.As: %v", text, err)
				}
			}
		}
	}
}

// the same, for the other WebView steps, with the connection words in the CDP message
func TestEveryWebViewStepKindIgnoresConnectionWordsInItsError(t *testing.T) {
	for _, step := range webviewSteps() {
		r := newWebViewRig()
		r.e.client = &agentClient{}
		bad := &device.WebViewError{Kind: "page error", Message: "EOF broken pipe connection reset"}
		r.page.tapErr, r.page.typeErr, r.page.waitErr = bad, bad, bad
		if _, ok := step.(parser.AssertStep); ok {
			r.page.seeErr = bad
		}
		_ = r.e.runStep(context.Background(), step)
		if r.e.client.(*agentClient).closes.Load() != 0 || len(r.opens) != 1 {
			t.Errorf("%T: closes %d, opens %d", step, r.e.client.(*agentClient).closes.Load(), len(r.opens))
		}
	}
}

// a nil agent client must never panic, whatever error comes back
func TestAReconnectWithNoAgentClientReturnsAnErrorInsteadOfPanicking(t *testing.T) {
	e := &Executor{vars: map[string]string{}, deviceCtx: &DeviceContext{}}
	if err := e.tryReconnect(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "no agent connection") {
		t.Fatalf("err = %v", err)
	}
}

// the masking wrapper keeps the typed error reachable, masked or not, and keeps errors.Is as it was
func TestScrubbedErrorsKeepTheirTypesForErrorsAs(t *testing.T) {
	sysdialog.NoteSecret("scrub-as-secret-Jh6w")
	e := &Executor{vars: map[string]string{}}
	inner := &webViewStepError{what: "tap in webview", err: &device.WebViewError{Kind: "not found", Message: "No element matches scrub-as-secret-Jh6w"}}
	masked := e.scrubEnv(inner)
	plain := e.scrubEnv(&webViewStepError{what: "tap in webview", err: &device.WebViewError{Kind: "not found", Message: "No element matches x"}})
	if strings.Contains(masked.Error(), "scrub-as-secret-Jh6w") {
		t.Fatalf("not masked: %v", masked)
	}
	if _, isScrubbed := masked.(*scrubbedError); !isScrubbed {
		t.Fatalf("the test needs a masked error, got %T", masked)
	}
	for name, err := range map[string]error{"masked": masked, "unmasked": plain} {
		var we *device.WebViewError
		var ws *webViewStepError
		if !errors.As(err, &we) || we.Kind != "not found" || !errors.As(err, &ws) || !isWebViewStepError(err) {
			t.Errorf("%s: types are not reachable", name)
		}
	}
	// the step timeout type still works through the mask, and Is is unchanged
	ste := &stepTimeoutError{desc: "d", timeout: time.Second, cause: fmt.Errorf("scrub-as-secret-Jh6w: %w", context.DeadlineExceeded)}
	m2 := e.scrubEnv(ste)
	var got *stepTimeoutError
	if !errors.As(m2, &got) || got != ste {
		t.Errorf("the step timeout is not reachable through the mask")
	}
	if errors.Is(masked, context.DeadlineExceeded) {
		t.Errorf("errors.Is looks inside a masked error now")
	}
	// a type that is not inside is still not found
	var other *stepError
	if errors.As(masked, &other) {
		t.Errorf("found a type that is not in the chain")
	}
}

func TestAWaitThatTheStepDeadlineEndsNamesTheTargetAndThePage(t *testing.T) {
	r := newWebViewRig()
	r.page.waitErr = context.DeadlineExceeded
	err := r.e.runWebViewWait(context.Background(), parser.WaitStep{Kind: parser.WaitWebView, WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay"}})
	var we *device.WebViewError
	if err == nil || !errors.As(err, &we) || !strings.Contains(err.Error(), `Timed out waiting for "Pay" in the WebView page "Checkout"`) {
		t.Fatalf("err = %v", err)
	}
	// and with the real clock: a page that never answers before the step's deadline
	r = newWebViewRig()
	r.e.timeout = 80 * time.Millisecond
	r.e.client = &agentClient{}
	r.e.deviceCtx.WebViewOpen = func(ctx context.Context, o device.WebViewOptions, fn func(WebViewPage) error) error {
		return fn(blockingPage{r.page})
	}
	err = r.e.runStep(context.Background(), parser.WaitStep{Kind: parser.WaitWebView, WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay"}})
	if err == nil || !strings.Contains(err.Error(), `Timed out waiting for "Pay" in the WebView page "Checkout"`) {
		t.Fatalf("step timeout: %v", err)
	}
	// the Flutter screen is not asked about a page in a WebView
	if n := r.e.client.(*agentClient).calls.Load(); n != 0 {
		t.Errorf("the agent was asked %d times", n)
	}
}

// blockingPage waits until the context ends, like a page that never shows the element.
type blockingPage struct{ *fakePage }

func (b blockingPage) WaitFor(ctx context.Context, s device.Selector, d time.Duration) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestWebViewStepLinesAndMessagesAreCapped(t *testing.T) {
	r := newWebViewRig()
	big := strings.Repeat("x", 9000)
	r.e.vars["big"] = big
	tgt := &parser.WebViewTarget{Kind: parser.WebViewText, Value: "<big>", Page: "<big>"}
	for _, st := range []parser.Step{
		parser.ActionStep{Verb: parser.VerbTapWebView, WebView: tgt},
		parser.ActionStep{Verb: parser.VerbTypeWebView, Text: "<big>", WebView: tgt},
		parser.AssertStep{WebView: tgt},
		parser.AssertStep{Negated: true, WebView: tgt},
		parser.WaitStep{Kind: parser.WaitWebView, WebView: tgt},
	} {
		if l := r.e.stepDescription(st); len(l) > 1000 || !strings.Contains(l, "…") {
			t.Errorf("%T: the line is %d bytes", st, len(l))
		}
	}
	// the "present" message and the wait message
	r.page.seeErr = nil
	err := r.e.runStep(context.Background(), parser.AssertStep{Negated: true, WebView: tgt})
	if err == nil || len(err.Error()) > 1500 {
		t.Errorf("negated see: %d bytes", len(fmt.Sprint(err)))
	}
	r.page.waitErr = context.DeadlineExceeded
	err = r.e.runStep(context.Background(), parser.WaitStep{Kind: parser.WaitWebView, WebView: tgt})
	if err == nil || len(err.Error()) > 1500 {
		t.Errorf("wait: %d bytes", len(fmt.Sprint(err)))
	}
}

func TestWebViewStepLinesShowTheExpandedMaskedTarget(t *testing.T) {
	t.Setenv("PROBE_TEST_WV2", "line-secret-Bn84")
	r := newWebViewRig()
	r.e.vars["who"] = r.e.resolve("${PROBE_TEST_WV2}")
	r.e.vars["item"] = "Pay now"
	r.e.vars["pg"] = "checkout"
	tgt := &parser.WebViewTarget{Kind: parser.WebViewText, Value: "<item> <who>", Page: "<pg>"}
	css := &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#<item>", Page: "<pg>"}
	want := map[string]parser.Step{
		`see "Pay now ****" in webview "checkout"`:                parser.AssertStep{WebView: tgt},
		`don't see css:"#Pay now" in webview "checkout"`:          parser.AssertStep{Negated: true, WebView: css},
		`wait until "Pay now ****" appears in webview "checkout"`: parser.WaitStep{Kind: parser.WaitWebView, WebView: tgt},
		`wait until css:"#Pay now" appears in webview "checkout"`: parser.WaitStep{Kind: parser.WaitWebView, WebView: css},
		`tap "Pay now ****" in webview "checkout"`:                parser.ActionStep{Verb: parser.VerbTapWebView, WebView: tgt},
	}
	for line, st := range want {
		if got := r.e.stepDescription(st); got != line {
			t.Errorf("%T: %q, want %q", st, got, line)
		}
	}
}

func TestAWebViewStepNeedsTheAppID(t *testing.T) {
	r := newWebViewRig()
	r.e.deviceCtx.AppID = ""
	err := r.e.runStep(context.Background(), webviewSteps()[0])
	if err == nil || !strings.Contains(err.Error(), "set `app` under `project` in probe.yaml") || len(r.opens) != 0 {
		t.Fatalf("err = %v, opens = %d", err, len(r.opens))
	}
}

// Each classifier refuses a WebView step's error on its own, whatever else guards the same path.
func TestEveryClassifierRefusesAWebViewStepError(t *testing.T) {
	for _, msg := range []string{"EOF", "Widget not found", "but none of them is on screen", `Expected to see "x" but it was not found`,
		"Failed assertion: _renderObject != null", "Failed assertion: debugNeedsPaint"} {
		err := &webViewStepError{what: "see in webview", err: &device.WebViewError{Kind: "page error", Message: msg}}
		if isConnectionError(err) || isNotFoundError(err) || isTransientAssertion(err) {
			t.Errorf("%q: a classifier read the text of a WebView step error", msg)
		}
		// the same words in another error are still read as before
		plain := errors.New(msg)
		if !(isConnectionError(plain) || isNotFoundError(plain) || isTransientAssertion(plain)) {
			t.Errorf("%q: a plain error is no longer classified", msg)
		}
	}
	for _, st := range webviewSteps() {
		if implicitWaitable(st) {
			t.Errorf("%T is retried by the implicit wait", st)
		}
	}
	if !implicitWaitable(parser.AssertStep{}) {
		t.Errorf("the widget `see` is no longer retried by the implicit wait")
	}
}

// An ordinary step that fails with a connection word and has no agent connection to re-establish is reported as it failed, with no panic and no
// reconnect note.
func TestANonWebViewConnectionErrorWithNoAgentClientIsReportedAsIs(t *testing.T) {
	e := &Executor{vars: map[string]string{}, deviceCtx: &DeviceContext{}, reconnectBackoff: time.Millisecond}
	err := e.runStep(context.Background(), parser.RecipeCall{Name: "EOF", Line: 3}) // "unknown recipe ... EOF": reads as a connection error
	if err == nil || !isConnectionError(err) || strings.Contains(err.Error(), "auto-reconnect") {
		t.Fatalf("err = %v", err)
	}
}

// A WebView step that runs into the step timeout is not annotated with what the Flutter screen shows.
func TestAWebViewStepTimeoutIsNotAnnotatedWithTheFlutterScreen(t *testing.T) {
	r := newWebViewRig()
	r.e.timeout = 60 * time.Millisecond
	agent := &agentClient{}
	r.e.client = agent
	r.e.deviceCtx.WebViewOpen = func(ctx context.Context, o device.WebViewOptions, fn func(WebViewPage) error) error {
		return fn(blockingTap{r.page})
	}
	err := r.e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbTapWebView, WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay"}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if agent.calls.Load() != 0 {
		t.Errorf("the agent was asked %d times for the screen of a WebView step", agent.calls.Load())
	}
	var ste *stepTimeoutError
	if errors.As(err, &ste) {
		t.Errorf("a WebView step timeout carries the Flutter screen's items")
	}
}

type blockingTap struct{ *fakePage }

func (b blockingTap) Tap(ctx context.Context, s device.Selector) error {
	<-ctx.Done()
	return ctx.Err()
}
