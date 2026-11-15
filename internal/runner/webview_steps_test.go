package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

func webviewSteps() []parser.Step {
	css := &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#f", Page: "checkout"}
	text := &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay"}
	return []parser.Step{
		parser.ActionStep{Verb: parser.VerbTapWebView, WebView: text},
		parser.ActionStep{Verb: parser.VerbTypeWebView, Text: "hello", WebView: css},
		parser.AssertStep{WebView: text},
		parser.AssertStep{Negated: true, WebView: css},
		parser.WaitStep{Kind: parser.WaitWebView, WebView: text},
		parser.WaitStep{Kind: parser.WaitWebView, WebView: css},
	}
}

func TestWebViewStepDescriptions(t *testing.T) {
	want := []string{
		`tap "Pay" in webview`,
		`type "hello" into css:"#f" in webview "checkout"`,
		`see "Pay" in webview`,
		`don't see css:"#f" in webview "checkout"`,
		`wait until "Pay" appears in webview`,
		`wait until css:"#f" appears in webview "checkout"`,
	}
	e := newTestExecutor()
	for i, step := range webviewSteps() {
		if got := e.stepDescription(step); got != want[i] {
			t.Errorf("step %d: %q, want %q", i, got, want[i])
		}
	}
}

// fakePage records what a step asks of the page and answers from a script.
type fakePage struct {
	calls   []string
	sels    []device.Selector
	typed   []string
	waits   []time.Duration
	title   string
	seeErr  error
	tapErr  error
	typeErr error
	waitErr error
}

func (f *fakePage) Tap(_ context.Context, s device.Selector) error {
	f.calls = append(f.calls, "tap")
	f.sels = append(f.sels, s)
	return f.tapErr
}
func (f *fakePage) Type(_ context.Context, s device.Selector, text string) error {
	f.calls = append(f.calls, "type")
	f.sels = append(f.sels, s)
	f.typed = append(f.typed, text)
	return f.typeErr
}
func (f *fakePage) See(_ context.Context, s device.Selector) error {
	f.calls = append(f.calls, "see")
	f.sels = append(f.sels, s)
	return f.seeErr
}
func (f *fakePage) WaitFor(_ context.Context, s device.Selector, d time.Duration) error {
	f.calls = append(f.calls, "wait")
	f.sels = append(f.sels, s)
	f.waits = append(f.waits, d)
	return f.waitErr
}
func (f *fakePage) Title() string { return f.title }

// webViewRig is an executor on an Android device context whose opener hands out one fakePage and records every open and close.
type webViewRig struct {
	e      *Executor
	page   *fakePage
	opens  []device.WebViewOptions
	closed int
}

func newWebViewRig() *webViewRig {
	r := &webViewRig{page: &fakePage{title: "Checkout"}}
	r.e = &Executor{vars: map[string]string{}, timeout: 7 * time.Second}
	r.e.deviceCtx = &DeviceContext{
		Platform: device.PlatformAndroid, Serial: "emulator-5600", AppID: "com.example.app",
		WebViewADB: noAdb{},
		WebViewOpen: func(_ context.Context, o device.WebViewOptions, fn func(WebViewPage) error) error {
			r.opens = append(r.opens, o)
			defer func() { r.closed++ }()
			return fn(r.page)
		},
	}
	return r
}

type noAdb struct{}

func (noAdb) RunArgs(context.Context, ...string) ([]byte, error) {
	return nil, errors.New("no adb in this test")
}

func TestWebViewStepsReachThePageWithTheKindFromTheSyntax(t *testing.T) {
	r := newWebViewRig()
	for i, step := range webviewSteps() {
		if i == 3 { // `don't see` passes when the page has no such element
			r.page.seeErr = &device.WebViewError{Kind: "not found", Message: "No element"}
		}
		if err := r.e.runStep(context.Background(), step); err != nil {
			t.Fatalf("%T: %v", step, err)
		}
	}
	if got := strings.Join(r.page.calls, ","); got != "tap,type,see,see,wait,wait" {
		t.Fatalf("calls = %s", got)
	}
	wantSel := []device.Selector{device.Text("Pay"), device.CSS("#f"), device.Text("Pay"), device.CSS("#f"), device.Text("Pay"), device.CSS("#f")}
	for i, s := range wantSel {
		if r.page.sels[i] != s {
			t.Errorf("call %d selector = %+v, want %+v", i, r.page.sels[i], s)
		}
	}
	if len(r.page.typed) != 1 || r.page.typed[0] != "hello" {
		t.Errorf("typed = %v", r.page.typed)
	}
	// the step's own time is the call timeout and the wait total
	if r.page.waits[0] != 7*time.Second || r.opens[0].CallTimeout != 7*time.Second {
		t.Errorf("waits = %v, call timeout = %v", r.page.waits, r.opens[0].CallTimeout)
	}
	// the page name reaches the session, the device and the app are the run's own
	o := r.opens[1]
	if o.Page != "checkout" || o.Serial != "emulator-5600" || o.AppID != "com.example.app" || r.opens[0].Page != "" {
		t.Errorf("options = %+v", o)
	}
	// one session per step, every one closed
	if len(r.opens) != 6 || r.closed != 6 {
		t.Errorf("opens = %d, closed = %d", len(r.opens), r.closed)
	}
}

func TestWebViewStepsAlwaysPassTheRunsSecretMasking(t *testing.T) {
	sysdialog.NoteSecret("webview-secret-Pq4x")
	r := newWebViewRig()
	if err := r.e.runStep(context.Background(), webviewSteps()[0]); err != nil {
		t.Fatal(err)
	}
	redact := r.opens[0].Redact
	if redact == nil {
		t.Fatal("no Redact was passed")
	}
	if got := redact("the page says webview-secret-Pq4x"); strings.Contains(got, "webview-secret-Pq4x") {
		t.Fatalf("Redact does not mask the run's secrets: %q", got)
	}
}

func TestWebViewStepsResolveVariablesAndMaskThemInLinesAndErrors(t *testing.T) {
	t.Setenv("PROBE_TEST_WV", "alice-secret-Zk39")
	r := newWebViewRig()
	r.e.vars["who"] = r.e.resolve("${PROBE_TEST_WV}") // registers the value as known
	r.e.vars["pg"] = "checkout"
	// the script's variables are expanded for the page, and the text typed is the expanded value
	tgt := &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#<who>", Page: "<pg>"}
	if err := r.e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbTypeWebView, Text: "<who>", WebView: tgt}); err != nil {
		t.Fatal(err)
	}
	if r.page.sels[0].Value != "#alice-secret-Zk39" || r.opens[0].Page != "checkout" || r.page.typed[0] != "alice-secret-Zk39" {
		t.Fatalf("sel=%+v page=%q typed=%q", r.page.sels[0], r.opens[0].Page, r.page.typed[0])
	}
	// the step line masks the target, the page and the text
	line := r.e.stepDescription(parser.ActionStep{Verb: parser.VerbTypeWebView, Text: "<who>", WebView: tgt})
	if strings.Contains(line, "alice-secret-Zk39") || !strings.Contains(line, `into css:"#****" in webview "checkout"`) {
		t.Fatalf("step line: %s", line)
	}
	secretPage := &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Pay", Page: "<who>"}
	if l := r.e.stepDescription(parser.ActionStep{Verb: parser.VerbTapWebView, WebView: secretPage}); strings.Contains(l, "alice-secret-Zk39") || !strings.Contains(l, `in webview "****"`) {
		t.Fatalf("a page name that holds a secret: %s", l)
	}
	for _, st := range []parser.Step{
		parser.ActionStep{Verb: parser.VerbTapWebView, WebView: tgt},
		parser.AssertStep{WebView: tgt},
		parser.WaitStep{Kind: parser.WaitWebView, WebView: tgt},
	} {
		if l := r.e.stepDescription(st); strings.Contains(l, "alice-secret-Zk39") {
			t.Errorf("%T line leaked: %s", st, l)
		}
	}
	// an error that quotes the value is masked
	r.page.tapErr = fmt.Errorf("No element matches %q", "alice-secret-Zk39")
	err := r.e.runWebViewTap(context.Background(), parser.ActionStep{Verb: parser.VerbTapWebView, WebView: tgt})
	if err == nil || strings.Contains(err.Error(), "alice-secret-Zk39") {
		t.Fatalf("err = %v", err)
	}
}

func TestDontSeeInAWebViewDistinguishesAbsentFromUnreachable(t *testing.T) {
	neg := parser.AssertStep{Negated: true, WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "Oops"}}
	cases := []struct {
		name    string
		seeErr  error
		wantErr string // "" = the step passes
	}{
		{"absent", &device.WebViewError{Kind: "not found", Message: "No element"}, ""},
		{"hidden", &device.WebViewError{Kind: "not visible", Message: "hidden"}, ""},
		{"present", nil, "expected not to see"},
		{"no webview at all", &device.WebViewError{Kind: "no webview", Message: "The app is not running on the device"}, "not running"},
		{"several pages", &device.WebViewError{Kind: "several pages", Message: "Found 2 WebView pages"}, "Found 2"},
		{"connection lost", &device.WebViewError{Kind: "connection lost", Message: "The WebView page changed"}, "changed"},
		{"plain error", errors.New("boom"), "boom"},
	}
	for _, c := range cases {
		r := newWebViewRig()
		r.page.seeErr = c.seeErr
		err := r.e.runStep(context.Background(), neg)
		if c.wantErr == "" && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.wantErr)
		}
	}
	// the positive form fails on every error
	r := newWebViewRig()
	r.page.seeErr = &device.WebViewError{Kind: "not found", Message: "No element in the WebView page"}
	if err := r.e.runStep(context.Background(), parser.AssertStep{WebView: neg.WebView}); err == nil {
		t.Error("see passed with no element")
	}
}

func TestWebViewStepFailuresNameTheStepAndKeepTheMessage(t *testing.T) {
	r := newWebViewRig()
	r.page.waitErr = &device.WebViewError{Kind: "not found", Message: `Timed out waiting for "Pay" in the WebView page "Checkout"`}
	err := r.e.runStep(context.Background(), webviewSteps()[4])
	var we *device.WebViewError
	if err == nil || !strings.Contains(err.Error(), "wait until in webview") || !strings.Contains(err.Error(), "Timed out waiting") || !errors.As(err, &we) {
		t.Fatalf("err = %v", err)
	}
}

func TestWebViewStepsNeedAnAndroidDevice(t *testing.T) {
	// cloud mode: no device context, the step is skipped like the native ones
	e := &Executor{vars: map[string]string{}}
	if err := e.runStep(context.Background(), webviewSteps()[0]); err != nil {
		t.Errorf("cloud mode: %v", err)
	}
	// iOS: a clear refusal, nothing opened
	r := newWebViewRig()
	r.e.deviceCtx.Platform = device.PlatformIOS
	for _, st := range webviewSteps() {
		err := r.e.runStep(context.Background(), st)
		if err == nil || !strings.Contains(err.Error(), "only supported on Android") {
			t.Errorf("%T on iOS: %v", st, err)
		}
	}
	if len(r.opens) != 0 {
		t.Errorf("a session was opened on iOS")
	}
	// no adb at all
	r = newWebViewRig()
	r.e.deviceCtx.WebViewADB = nil
	if err := r.e.runStep(context.Background(), webviewSteps()[0]); err == nil || !strings.Contains(err.Error(), "no adb") {
		t.Errorf("no adb: %v", err)
	}
}

func TestWebViewWaitWithoutAStepTimeoutUsesTheDefaultTotal(t *testing.T) {
	r := newWebViewRig()
	r.e.timeout = 0
	if err := r.e.runStep(context.Background(), webviewSteps()[4]); err != nil {
		t.Fatal(err)
	}
	if r.page.waits[0] != defaultWebViewWait {
		t.Errorf("wait total = %v", r.page.waits[0])
	}
}

// Until the runner part exists the step reports one clear error, sets no variable and shows no value in its line.
func TestStoreTextOfAnElementReportsNotImplementedAndSetsNothing(t *testing.T) {
	r := newWebViewRig()
	step := parser.ActionStep{Verb: parser.VerbStoreWebView, Name: "total", WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#total", Page: "checkout"}}
	err := r.e.runStep(context.Background(), step)
	if err == nil || !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("err = %v", err)
	}
	if _, set := r.e.vars["total"]; set || len(r.opens) != 0 {
		t.Errorf("the step set a variable or opened a page: vars=%v opens=%d", r.e.vars, len(r.opens))
	}
	if got := r.e.stepDescription(step); got != `store text of css:"#total" in webview "checkout" as total` {
		t.Errorf("line = %q", got)
	}
}
