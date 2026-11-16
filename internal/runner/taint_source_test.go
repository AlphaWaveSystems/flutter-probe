package runner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// The read step is the first real source of tainted values. These tests use it instead of marking a variable by hand.

func storeStepFor(name, selector, page string) parser.ActionStep {
	return parser.ActionStep{Verb: parser.VerbStoreWebView, Name: name, WebView: &parser.WebViewTarget{Kind: parser.WebViewCSS, Value: selector, Page: page}}
}

// sourceRig is an executor on an Android device context whose page answers every read with text, and whose agent client records what a sink would send.
func sourceRig(text string) (*webViewRig, *recClient) {
	r := newWebViewRig()
	r.page.readRes = device.ReadResult{Text: text, Length: len(text)}
	rc := &recClient{}
	r.e.client = rc
	return r, rc
}

func TestStoreTextOfAnElementSetsATaintedVariableAndPrintsNoValue(t *testing.T) {
	const text = "Total: 42.00 http://evil.example/x"
	rig, _ := sourceRig(text)
	rig.page.readRes.Length = 40
	// capture what the step prints
	old := os.Stdout
	rd, wr, _ := os.Pipe()
	os.Stdout = wr
	err := rig.e.runStep(context.Background(), storeStepFor("total", "#total", "checkout"))
	wr.Close()
	os.Stdout = old
	out, _ := io.ReadAll(rd)
	if err != nil {
		t.Fatal(err)
	}
	if rig.e.vars["total"] != text || !rig.e.tainted["total"] {
		t.Fatalf("vars = %q, tainted = %v", rig.e.vars["total"], rig.e.tainted)
	}
	if strings.Contains(string(out), "Total") || strings.Contains(string(out), "evil") || !strings.Contains(string(out), `stored "total": 34 characters`) {
		t.Errorf("the step printed %q", out)
	}
	if rig.page.sels[0] != device.CSS("#total") || rig.opens[0].Page != "checkout" {
		t.Errorf("selector %+v, page %q", rig.page.sels[0], rig.opens[0].Page)
	}
	if len(rig.opens) != 1 || rig.closed != 1 {
		t.Errorf("one page per step: opens %d, closed %d", len(rig.opens), rig.closed)
	}
}

func TestStoreTextOfAnElementReportsACutAndAnEmptyText(t *testing.T) {
	rig, _ := sourceRig("")
	old := os.Stdout
	rd, wr, _ := os.Pipe()
	os.Stdout = wr
	err1 := rig.e.runStep(context.Background(), storeStepFor("empty", "#e", ""))
	rig.page.readRes = device.ReadResult{Text: strings.Repeat("x", 1000), Length: 4312, Cut: true}
	err2 := rig.e.runStep(context.Background(), storeStepFor("big", "#b", ""))
	wr.Close()
	os.Stdout = old
	out, _ := io.ReadAll(rd)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if v, ok := rig.e.vars["empty"]; !ok || v != "" || !rig.e.tainted["empty"] {
		t.Errorf("an empty text: %q %v %v", v, ok, rig.e.tainted)
	}
	if !strings.Contains(string(out), `stored "empty": 0 characters`) || !strings.Contains(string(out), `stored "big": 1000 characters, cut from 4312`) {
		t.Errorf("the step printed %q", out)
	}
}

func TestAFailedReadSetsNothingAndLeavesTheOldValue(t *testing.T) {
	rig, _ := sourceRig("")
	rig.e.setVar("total", "kept", false)
	rig.page.readErr = &device.WebViewError{Kind: "not found", Message: "No element in the WebView page matches"}
	err := rig.e.runStep(context.Background(), storeStepFor("total", "#none", ""))
	if err == nil || !strings.Contains(err.Error(), "No element") {
		t.Fatalf("err = %v", err)
	}
	if rig.e.vars["total"] != "kept" || rig.e.tainted["total"] {
		t.Errorf("a failed read changed the variable: %q %v", rig.e.vars["total"], rig.e.tainted)
	}
	// and a name the read never reached stays unset
	if _, set := rig.e.vars["other"]; set {
		t.Errorf("a variable was set")
	}
}

func TestStoreTextNeedsAnAndroidDeviceAndNeverSkips(t *testing.T) {
	e := &Executor{vars: map[string]string{}, timeout: time.Second} // cloud mode: no device context
	if err := e.runStep(context.Background(), storeStepFor("t", "#t", "")); err == nil || !strings.Contains(err.Error(), "local Android device") {
		t.Errorf("cloud: err = %v", err)
	}
	if _, set := e.vars["t"]; set {
		t.Errorf("a skipped read set a variable")
	}
	rig, _ := sourceRig("x")
	rig.e.deviceCtx.Platform = device.PlatformIOS
	if err := rig.e.runStep(context.Background(), storeStepFor("t", "#t", "")); err == nil || !strings.Contains(err.Error(), "only supported on Android") || len(rig.opens) != 0 {
		t.Errorf("iOS: err = %v, opens %d", err, len(rig.opens))
	}
}

// A store over a name replaces its taint, in both directions.
func TestStoringOverANameReplacesItsTaint(t *testing.T) {
	rig, _ := sourceRig("page text")
	ctx := context.Background()
	rig.e.setVar("n", "clean", false)
	if err := rig.e.runStep(ctx, storeStepFor("n", "#n", "")); err != nil || !rig.e.tainted["n"] {
		t.Fatalf("read over a clean name: %v %v", err, rig.e.tainted)
	}
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbStore, Name: "n", Text: "clean again"}); err != nil || rig.e.tainted["n"] {
		t.Fatalf("a script store over a tainted name: %v %v", err, rig.e.tainted)
	}
}

// The name decides whether the value is also registered as a secret: a variable called like one is masked everywhere, another is not.
func TestAVariableReadFromAPageIsMaskedByItsName(t *testing.T) {
	rig, _ := sourceRig("page-password-Zx93-secret")
	if err := rig.e.runStep(context.Background(), storeStepFor("password", "#p", "")); err != nil {
		t.Fatal(err)
	}
	if got := sysdialog.ScrubKnown("the value is page-password-Zx93-secret here"); strings.Contains(got, "Zx93") {
		t.Errorf("a value read into `password` is not masked: %q", got)
	}
	rig.page.readRes = device.ReadResult{Text: "Order 4711 total 42.00", Length: 22}
	if err := rig.e.runStep(context.Background(), storeStepFor("total", "#t", "")); err != nil {
		t.Fatal(err)
	}
	if got := sysdialog.ScrubKnown("saw Order 4711 total 42.00 on screen"); !strings.Contains(got, "Order 4711 total 42.00") {
		t.Errorf("an ordinary value was masked: %q", got)
	}
}

// Every sink refuses a value that really came from a page, with nothing sent.
func TestEverySinkRefusesAValueReadFromAPage(t *testing.T) {
	hits := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()
	steps := map[string]parser.Step{
		"open link":          parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<t>"},
		"open link in app":   parser.ActionStep{Verb: parser.VerbOpenLink, Name: "myapp://<t>", DeepLink: true},
		"add media":          parser.ActionStep{Verb: parser.VerbAddMedia, Name: "<t>"},
		"set language":       parser.ActionStep{Verb: parser.VerbSetLanguage, Name: "<t>"},
		"type native query":  parser.ActionStep{Verb: parser.VerbTypeNative, Name: "<t>", Text: "x"},
		"type native text":   parser.ActionStep{Verb: parser.VerbTypeNative, Name: "q", Text: "a <t> b"},
		"take screenshot":    parser.ActionStep{Verb: parser.VerbTakeShot, Name: "s-<t>"},
		"compare screenshot": parser.ActionStep{Verb: parser.VerbCompareShot, Name: "<t>"},
		"call url":           parser.HTTPCallStep{Method: "GET", URL: "<t>"},
		"call body":          parser.HTTPCallStep{Method: "POST", URL: srv.URL, Body: "<t>"},
		"mock path":          parser.MockBlock{Method: "GET", Path: "/<t>", Status: 200},
		"mock body":          parser.MockBlock{Method: "GET", Path: "/x", Status: 200, Body: "<t>"},
		"start measuring":    parser.PerfStep{Kind: parser.PerfStart, Name: "<t>"},
		"dialog type":        parser.SystemDialogStep{Op: parser.SysType, Text: "<t>", Field: "Password"},
		"dialog tap":         parser.SystemDialogStep{Op: parser.SysTap, Button: "<t>"},
	}
	for name, step := range steps {
		rig, rc := sourceRig("http://evil.example/steal")
		rig.e.deviceCtx.sysDriver = &dialogRec{}
		if err := rig.e.runStep(context.Background(), storeStepFor("t", "#t", "")); err != nil {
			t.Fatalf("%s: the read failed: %v", name, err)
		}
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: went past the guard (panic: %v)", name, r)
				}
			}()
			err = rig.e.runStep(context.Background(), step)
		}()
		if err == nil || !strings.Contains(err.Error(), "<t> holds text read from a page") || strings.Contains(err.Error(), "evil.example") {
			t.Errorf("%s: err = %v", name, err)
		}
		if rc.n.Load() != 0 || hits.Load() != 0 {
			t.Errorf("%s: something was sent (agent calls %d, http hits %d)", name, rc.n.Load(), hits.Load())
		}
	}
}

// Through a recipe, the clipboard and a copy of the variable: a value that really came from a page stays tainted.
func TestAValueReadFromAPageStaysTaintedThroughRecipesCopiesAndTheClipboard(t *testing.T) {
	ctx := context.Background()
	clip := &clipRec{}
	rig, _ := sourceRig("http://evil.example/steal")
	rig.e.client = clip
	rig.e.recipes = map[string]parser.RecipeDef{"visit": {Name: "visit", Params: []string{"p"}, Body: []parser.Step{parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<p>"}}}}
	if err := rig.e.runStep(ctx, storeStepFor("t", "#t", "")); err != nil {
		t.Fatal(err)
	}
	// a recipe parameter that mentions it
	if err := rig.e.runRecipeCall(ctx, parser.RecipeCall{Name: "visit", Args: []string{"<t>"}}); err == nil || !strings.Contains(err.Error(), "holds text read from a page") {
		t.Errorf("recipe: err = %v", err)
	}
	// a copy of the variable
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbStore, Name: "copy", Text: "<t>/more"}); err != nil {
		t.Fatal(err)
	}
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<copy>"}); err == nil {
		t.Errorf("a copy of a page value was opened")
	}
	// the clipboard
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "<t>"}); err != nil {
		t.Fatal(err)
	}
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
		t.Fatal(err)
	}
	if err := rig.e.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<clipboard>"}); err == nil || clip.n.Load() != 0 {
		t.Errorf("the pasted clipboard was opened: err = %v, calls %d", err, clip.n.Load())
	}
	// and what is allowed still works: a selector, a `see` text and typing use the value as text on the screen
	if got, by := rig.e.resolveT("<t>"); got != "http://evil.example/steal" || len(by) != 1 {
		t.Errorf("the value cannot be used as screen text: %q %v", got, by)
	}
	if err := rig.e.runStep(ctx, parser.AssertStep{WebView: &parser.WebViewTarget{Kind: parser.WebViewText, Value: "<t>"}}); err != nil {
		t.Errorf("a see with the value as its text: %v", err)
	}
}

func TestTheStoreStepLineQuotesAndCapsTheVariableNameAndShowsNoValue(t *testing.T) {
	rig, _ := sourceRig("page value")
	line := rig.e.stepDescription(storeStepFor("a\nb", "#total", ""))
	if strings.ContainsAny(line, "\n\r") || !strings.Contains(line, `as "a\nb"`) {
		t.Errorf("line = %q", line)
	}
	long := strings.Repeat("n", 500)
	if l := rig.e.stepDescription(storeStepFor(long, "#total", "")); len(l) > 400 {
		t.Errorf("a long variable name makes a %d byte line", len(l))
	}
}
