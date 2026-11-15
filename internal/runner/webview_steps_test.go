package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
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
	}
}

// Until the runner glue exists the steps parse and fail with one clear message; no device or session is touched (the executor here has no client).
func TestWebViewStepsReportNotImplemented(t *testing.T) {
	for _, step := range webviewSteps() {
		e := newTestExecutor()
		err := e.runStep(context.Background(), step)
		if err == nil || !strings.Contains(err.Error(), "webview steps are not implemented yet") {
			t.Errorf("%T: err = %v", step, err)
		}
	}
}

func TestWebViewStepDescriptions(t *testing.T) {
	want := []string{
		`tap "Pay" in webview`,
		`type "hello" into css:"#f" in webview "checkout"`,
		`see "Pay" in webview`,
		`don't see css:"#f" in webview "checkout"`,
		`wait until "Pay" appears in webview`,
	}
	e := newTestExecutor()
	for i, step := range webviewSteps() {
		if got := e.stepDescription(step); got != want[i] {
			t.Errorf("step %d: %q, want %q", i, got, want[i])
		}
	}
}

// The text typed into a WebView field is masked in the step line like any typed text: a variable can hold a value that came from the environment.
func TestWebViewTypeDescriptionMasksEnvValues(t *testing.T) {
	t.Setenv("PROBE_TEST_EMAIL", "alice@example.com")
	e := &Executor{vars: map[string]string{}}
	e.vars["who"] = e.resolve("${PROBE_TEST_EMAIL}") // the expansion registers the value as known; the variable then holds it
	step := parser.ActionStep{Verb: parser.VerbTypeWebView, Text: "<who>", WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: "#email"}}
	d := e.stepDescription(step)
	if strings.Contains(d, "alice@example.com") || !strings.Contains(d, "****") {
		t.Fatalf("description leaked the value or lost the step: %s", d)
	}
}
