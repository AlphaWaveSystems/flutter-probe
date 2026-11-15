package runner

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// recClient counts every call a sink could make through the agent connection.
type recClient struct {
	fakeAIClient
	n atomic.Int32
}

func (r *recClient) OpenLink(ctx context.Context, url string) error { r.n.Add(1); return nil }
func (r *recClient) Screenshot(ctx context.Context, name string) (string, error) {
	r.n.Add(1)
	return "", nil
}
func (r *recClient) RegisterMock(ctx context.Context, m probelink.MockParam) error {
	r.n.Add(1)
	return nil
}

// taintRig is an executor with a recording agent client and a device context that has no manager: a sink that got past the guard would panic or call.
func taintRig() (*Executor, *recClient) {
	rc := &recClient{}
	e := &Executor{vars: map[string]string{}, client: rc, deviceCtx: &DeviceContext{}, timeout: time.Second}
	return e, rc
}

const pageValue = "http://evil.example/steal?x=1"

func tainted(e *Executor, name, value string) { e.setVar(name, value, true) }

func TestEverySinkRefusesATaintedValueAndSendsNothing(t *testing.T) {
	srvHits := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { srvHits.Add(1) }))
	defer srv.Close()
	steps := map[string]parser.Step{
		"open link":              parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<t>"},
		"open link into the app": parser.ActionStep{Verb: parser.VerbOpenLink, Name: "myapp://<t>", DeepLink: true},
		"add media":              parser.ActionStep{Verb: parser.VerbAddMedia, Name: "<t>"},
		"set language":           parser.ActionStep{Verb: parser.VerbSetLanguage, Name: "<t>"},
		"type native (query)":    parser.ActionStep{Verb: parser.VerbTypeNative, Name: "<t>", Text: "x"},
		"type native (text)":     parser.ActionStep{Verb: parser.VerbTypeNative, Name: "q", Text: "a <t> b"},
		"take screenshot":        parser.ActionStep{Verb: parser.VerbTakeShot, Name: "shot-<t>"},
		"compare screenshot":     parser.ActionStep{Verb: parser.VerbCompareShot, Name: "<t>"},
		"call (url)":             parser.HTTPCallStep{Method: "GET", URL: "<t>"},
		"call (body)":            parser.HTTPCallStep{Method: "POST", URL: srv.URL, Body: `{"x":"<t>"}`},
		"mock (path)":            parser.MockBlock{Method: "GET", Path: "/<t>", Status: 200},
		"mock (body)":            parser.MockBlock{Method: "GET", Path: "/x", Status: 200, Body: "<t>"},
		"start measuring":        parser.PerfStep{Kind: parser.PerfStart, Name: "<t>"},
	}
	for name, step := range steps {
		e, rc := taintRig()
		tainted(e, "t", pageValue)
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: the step went past the guard (panic: %v)", name, r)
				}
			}()
			err = e.runStep(context.Background(), step)
		}()
		if err == nil || !strings.Contains(err.Error(), "<t> holds text read from a page") {
			t.Errorf("%s: err = %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "evil.example") || strings.Contains(err.Error(), "steal") {
			t.Errorf("%s: the error repeats the value: %v", name, err)
		}
		if rc.n.Load() != 0 || srvHits.Load() != 0 {
			t.Errorf("%s: something was sent (agent calls %d, http hits %d)", name, rc.n.Load(), srvHits.Load())
		}
	}
}

// the same steps with no tainted variable still work as before
func TestTheSinksWorkWithCleanValues(t *testing.T) {
	e, rc := taintRig()
	e.setVar("t", "http://example.test/ok", false)
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<t>"}); err != nil {
		t.Fatal(err)
	}
	if err := e.runStep(context.Background(), parser.MockBlock{Method: "GET", Path: "/<t>", Status: 200}); err != nil {
		t.Fatal(err)
	}
	if rc.n.Load() != 2 {
		t.Errorf("agent calls = %d", rc.n.Load())
	}
}

func TestATaintedValueStillWorksWhereTheScreenIsTheOnlyTarget(t *testing.T) {
	e, _ := taintRig()
	tainted(e, "t", "Total: 42")
	got, by := e.resolveT("see <t>")
	if got != "see Total: 42" || !reflect.DeepEqual(by, []string{"t"}) {
		t.Fatalf("resolveT = %q, %v", got, by)
	}
	if e.resolve("<t>") != "Total: 42" || e.resolveVars("<t>") != "Total: 42" {
		t.Errorf("resolve and resolveVars no longer expand it")
	}
}

func TestTaintFollowsComposition(t *testing.T) {
	e, _ := taintRig()
	tainted(e, "t", "v")
	for _, text := range []string{"<t>", "pre <t>", "<t> post", "a <t> b <t> c", "${HOME_UNSET_X}<t>"} {
		if _, err := e.forSink("open link", text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
	// a variable whose value mentions the tainted one
	e.setVar("wrap", "x <t> y", false)
	if _, err := e.forSink("open link", "<wrap>"); err == nil {
		t.Errorf("a variable that mentions a tainted one was accepted")
	}
	// text that does not mention it
	if out, err := e.forSink("open link", "plain <wrap2> text"); err != nil || out != "plain <wrap2> text" {
		t.Errorf("unrelated text: %q, %v", out, err)
	}
}

func TestStoreCopiesAndClearsTaint(t *testing.T) {
	e, _ := taintRig()
	ctx := context.Background()
	tainted(e, "t", "page text")
	store := func(name, text string) {
		if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbStore, Name: name, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	store("copy", "<t>")
	store("deeper", "<copy>!")
	store("clean", "just text")
	for name, want := range map[string]bool{"copy": true, "deeper": true, "clean": false} {
		_, err := e.forSink("open link", "<"+name+">")
		if (err != nil) != want {
			t.Errorf("%s: refused = %v, want %v", name, err != nil, want)
		}
	}
	// a clean store over a tainted name clears the flag; a data-driven row value and an environment value are clean
	store("copy", "now clean")
	if _, err := e.forSink("open link", "<copy>"); err != nil {
		t.Errorf("a clean store did not clear the taint: %v", err)
	}
	tainted(e, "row", "x")
	e.SetVar("row", "from a data row")
	if _, err := e.forSink("open link", "<row>"); err != nil {
		t.Errorf("SetVar did not clear the taint: %v", err)
	}
	t.Setenv("PROBE_TAINT_ENV_OK", "env value")
	if out, err := e.forSink("open link", "${PROBE_TAINT_ENV_OK}"); err != nil || out != "env value" {
		t.Errorf("an environment value was refused or changed: %q, %v", out, err)
	}
}

// A recipe parameter is bound to its argument text: a parameter whose argument mentions a tainted variable is tainted where it is used, inside the recipe.
func TestTaintFollowsARecipeParameterIntoTheRecipe(t *testing.T) {
	e, rc := taintRig()
	e.recipes = map[string]parser.RecipeDef{"visit": {Name: "visit", Params: []string{"p"}, Body: []parser.Step{parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<p>"}}}}
	tainted(e, "t", pageValue)
	err := e.runRecipeCall(context.Background(), parser.RecipeCall{Name: "visit", Args: []string{"<t>"}})
	if err == nil || !strings.Contains(err.Error(), "<t> holds text read from a page") || rc.n.Load() != 0 {
		t.Fatalf("err = %v, agent calls %d", err, rc.n.Load())
	}
	// a clean argument works, and rebinding the parameter clears what it was
	if err := e.runRecipeCall(context.Background(), parser.RecipeCall{Name: "visit", Args: []string{"http://example.test/"}}); err != nil || rc.n.Load() != 1 {
		t.Fatalf("a clean argument: %v, agent calls %d", err, rc.n.Load())
	}
}

// A tainted value is inserted as it is: it is not searched again for placeholders, ${ENV} references or l10n keys.
func TestATaintedValueIsInsertedInOnePass(t *testing.T) {
	t.Setenv("PROBE_TAINT_ENV", "env-secret")
	e, _ := taintRig()
	e.vars["token"] = "TOKEN-VALUE"
	page := "<token> ${PROBE_TAINT_ENV} \x1el10n:greeting\x1e <t> <random.number>"
	tainted(e, "t", page)
	got, by := e.resolveT("[<t>]")
	if got != "["+page+"]" || !reflect.DeepEqual(by, []string{"t"}) {
		t.Fatalf("got %q (%v), want the value untouched", got, by)
	}
	if strings.Contains(e.resolveVars("<t>"), "TOKEN-VALUE") || strings.Contains(e.resolve("<t>"), "env-secret") {
		t.Errorf("the value was expanded")
	}
	// through another variable, and with the placeholder produced by an expansion
	e.setVar("a", "<t>", false)
	if got, _ := e.resolveT("<a>"); got != page {
		t.Errorf("through another variable: %q", got)
	}
	// clean variables around it still expand
	if got, _ := e.resolveT("<token>|<t>|${PROBE_TAINT_ENV}"); got != "TOKEN-VALUE|"+page+"|env-secret" {
		t.Errorf("around it: %q", got)
	}
}

func TestAScriptTextCannotForgeATaintToken(t *testing.T) {
	e, _ := taintRig()
	tainted(e, "t", "SECRET-PAGE-VALUE")
	got, by := e.resolveT("a\x00T0\x00b")
	if strings.Contains(got, "SECRET-PAGE-VALUE") || len(by) != 0 {
		t.Errorf("a forged token was replaced: %q (%v)", got, by)
	}
}

// With a tainted variable present, text that does not mention it expands exactly as it did.
func TestTaintedVariablesDoNotChangeOtherExpansions(t *testing.T) {
	t.Setenv("PROBE_TAINT_ENV2", "e2")
	e, _ := taintRig()
	e.vars["a"] = "A"
	base, _ := taintRig()
	base.vars["a"] = "A"
	tainted(e, "t", "tv")
	// (a variable whose value mentions another one is left out: the order in which the map is read decides that case, with or without taint)
	for _, s := range []string{"<a>", "x ${PROBE_TAINT_ENV2} y", "no vars", "<unset>", "<a><a>", "<a> and <unset> and ${PROBE_TAINT_ENV2}"} {
		if got, want := e.resolve(s), base.resolve(s); got != want {
			t.Errorf("%q: %q, want %q", s, got, want)
		}
	}
}

// Every string of every example script expands as it did before the guard existed: resolve is the old expansion when nothing is tainted.
func TestExampleScriptsExpandAsBefore(t *testing.T) {
	var files []string
	_ = filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".worktrees" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".probe") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) < 20 {
		t.Fatalf("found only %d example scripts", len(files))
	}
	checked := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var lits []string
		for _, line := range strings.Split(string(src), "\n") {
			lits = append(lits, strings.Split(line, `"`)[1:]...)
		}
		e := newTestExecutor()
		e.vars["x"] = "VX"
		for _, s := range lits {
			if strings.Contains(s, "<random") {
				continue
			}
			want := e.finishResolve(e.expandVars(s, nil))
			if got := e.resolve(s); got != want {
				t.Fatalf("%s: %q expands to %q, was %q", f, s, got, want)
			}
			checked++
		}
	}
	if checked < 500 {
		t.Fatalf("only %d strings were compared", checked)
	}
}

// ---- the review of the first version: system dialogs, the clipboard, cloud mode

// dialogRec is a system dialog driver that records everything it is asked to tap or type.
type dialogRec struct {
	sysdialog.Driver // unused methods stay nil
	taps             []string
	typed            [][2]string
}

func (d *dialogRec) Tap(ctx context.Context, button, title string) (string, error) {
	d.taps = append(d.taps, button)
	return button, nil
}
func (d *dialogRec) Type(ctx context.Context, field, text, title string) error {
	d.typed = append(d.typed, [2]string{field, text})
	return nil
}

// A value read from a page must not choose which environment variable a system dialog types: "$NAME" is read by ResolveSecret.
func TestASystemDialogRefusesATaintedTextAndNeverReadsTheEnvironmentForIt(t *testing.T) {
	t.Setenv("PROBE_TAINT_DIALOG_SECRET", "TOPSECRET-VALUE")
	for name, page := range map[string]string{"an env reference": "$PROBE_TAINT_DIALOG_SECRET", "a braced reference": "${PROBE_TAINT_DIALOG_SECRET}", "a literal": "typed-by-the-page"} {
		rec := &dialogRec{}
		e := &Executor{vars: map[string]string{}, deviceCtx: &DeviceContext{sysDriver: rec}, timeout: time.Second}
		tainted(e, "t", page)
		for stepName, step := range map[string]parser.SystemDialogStep{
			"type text":  {Op: parser.SysType, Text: "<t>", Field: "Password"},
			"type field": {Op: parser.SysType, Text: "ok", Field: "<t>"},
			"tap":        {Op: parser.SysTap, Button: "<t>"},
		} {
			err := e.runStep(context.Background(), step)
			if err == nil || !strings.Contains(err.Error(), "<t> holds text read from a page") {
				t.Errorf("%s / %s: err = %v", name, stepName, err)
			}
			if err != nil && strings.Contains(err.Error(), "TOPSECRET-VALUE") {
				t.Errorf("%s / %s: the error shows the secret", name, stepName)
			}
		}
		if len(rec.typed) != 0 || len(rec.taps) != 0 {
			t.Errorf("%s: the dialog was driven: %v %v", name, rec.typed, rec.taps)
		}
	}
	// a script's own "$NAME" still works, and so does a clean tap
	rec := &dialogRec{}
	e := &Executor{vars: map[string]string{}, deviceCtx: &DeviceContext{sysDriver: rec}, timeout: time.Second}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysType, Text: "$PROBE_TAINT_DIALOG_SECRET", Field: "Password"}); err != nil {
		t.Fatal(err)
	}
	if err := e.runStep(context.Background(), parser.SystemDialogStep{Op: parser.SysTap, Button: "Allow"}); err != nil {
		t.Fatal(err)
	}
	if len(rec.typed) != 1 || rec.typed[0] != [2]string{"Password", "TOPSECRET-VALUE"} || len(rec.taps) != 1 || rec.taps[0] != "Allow" {
		t.Errorf("clean steps: typed %v, taps %v", rec.typed, rec.taps)
	}
}

// clipRec is an agent client with a clipboard.
type clipRec struct {
	recClient
	clip string
}

func (c *clipRec) CopyToClipboard(ctx context.Context, text string) error { c.clip = text; return nil }
func (c *clipRec) PasteFromClipboard(ctx context.Context) (string, error) { return c.clip, nil }

// copy and paste are a propagation: a page value that goes through the clipboard comes back as a page value.
func TestTheClipboardCarriesTheTaintOfWhatWasCopied(t *testing.T) {
	clip := &clipRec{}
	e := &Executor{vars: map[string]string{}, client: clip, deviceCtx: &DeviceContext{}, timeout: time.Second}
	ctx := context.Background()
	run := func(step parser.Step) {
		t.Helper()
		if err := e.runStep(ctx, step); err != nil {
			t.Fatal(err)
		}
	}
	tainted(e, "t", pageValue)
	run(parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "<t>"})
	run(parser.ActionStep{Verb: parser.VerbPasteClipboard})
	if clip.clip != pageValue || e.vars["clipboard"] != pageValue {
		t.Fatalf("the clipboard did not carry the text: %q %q", clip.clip, e.vars["clipboard"])
	}
	if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<clipboard>"}); err == nil || !strings.Contains(err.Error(), "<clipboard> holds text read from a page") || clip.n.Load() != 0 {
		t.Fatalf("a link built from the pasted clipboard: err = %v, calls %d", err, clip.n.Load())
	}
	// a copy of clean text makes the next paste clean again
	run(parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "http://example.test/ok"})
	run(parser.ActionStep{Verb: parser.VerbPasteClipboard})
	if err := e.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<clipboard>"}); err != nil {
		t.Errorf("a clean clipboard was refused: %v", err)
	}
	// a copy of a text that merely mentions the tainted variable is tainted too
	run(parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "go to <t>"})
	run(parser.ActionStep{Verb: parser.VerbPasteClipboard})
	if _, err := e.forSink("open link", "<clipboard>"); err == nil {
		t.Errorf("a composed copy was clean after the paste")
	}
}

// In cloud mode (no device context) a tainted value is still refused: the guard comes before the skip.
func TestTheGuardComesBeforeTheCloudModeSkip(t *testing.T) {
	for name, step := range map[string]parser.Step{
		"add media":              parser.ActionStep{Verb: parser.VerbAddMedia, Name: "<t>"},
		"open link into the app": parser.ActionStep{Verb: parser.VerbOpenLink, Name: "myapp://<t>", DeepLink: true},
		"set language":           parser.ActionStep{Verb: parser.VerbSetLanguage, Name: "<t>"},
		"type native":            parser.ActionStep{Verb: parser.VerbTypeNative, Name: "q", Text: "<t>"},
	} {
		rc := &recClient{}
		e := &Executor{vars: map[string]string{}, client: rc, timeout: time.Second} // no deviceCtx: cloud mode
		tainted(e, "t", pageValue)
		if err := e.runStep(context.Background(), step); err == nil || !strings.Contains(err.Error(), "holds text read from a page") {
			t.Errorf("%s in cloud mode: err = %v", name, err)
		}
	}
	// clean values are skipped there as before
	e := &Executor{vars: map[string]string{"t": "ok"}, client: &recClient{}, timeout: time.Second}
	if err := e.runStep(context.Background(), parser.ActionStep{Verb: parser.VerbAddMedia, Name: "<t>"}); err != nil {
		t.Errorf("a clean add media in cloud mode: %v", err)
	}
}

// ---- the clipboard belongs to the device, not to a test

func clipExecutor(clip *clipRec, dc *DeviceContext) *Executor {
	return &Executor{vars: map[string]string{}, client: clip, deviceCtx: dc, timeout: time.Second}
}

// Every test attempt gets a new Executor, but the device clipboard outlives it: a page value copied in one test and pasted in the next is still a page value.
func TestAClipboardTaintSurvivesTheExecutorThatSetIt(t *testing.T) {
	ctx := context.Background()
	clip := &clipRec{}
	dc := &DeviceContext{}
	copyTainted := clipExecutor(clip, dc)
	tainted(copyTainted, "t", pageValue)
	if err := copyTainted.runStep(ctx, parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "<t>"}); err != nil {
		t.Fatal(err)
	}
	// a later test, or a retry attempt: a new executor on the same device
	later := clipExecutor(clip, dc)
	if err := later.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
		t.Fatal(err)
	}
	if err := later.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<clipboard>"}); err == nil || !strings.Contains(err.Error(), "<clipboard> holds text read from a page") || clip.n.Load() != 0 {
		t.Fatalf("CROSS-TEST: err = %v, agent calls %d", err, clip.n.Load())
	}
	// pasting again, in yet another executor, is still a page value (a paste does not clear it)
	for i := 0; i < 2; i++ {
		again := clipExecutor(clip, dc)
		if err := again.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
			t.Fatal(err)
		}
		if _, err := again.forSink("open link", "<clipboard>"); err == nil {
			t.Fatalf("paste %d: the clipboard was clean", i+2)
		}
	}
	// a clean copy, in any executor, clears it for all
	if err := clipExecutor(clip, dc).runStep(ctx, parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "http://example.test/ok"}); err != nil {
		t.Fatal(err)
	}
	last := clipExecutor(clip, dc)
	if err := last.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
		t.Fatal(err)
	}
	if err := last.runStep(ctx, parser.ActionStep{Verb: parser.VerbOpenLink, Name: "<clipboard>"}); err != nil {
		t.Errorf("a clean clipboard was refused: %v", err)
	}
	// another device has its own clipboard
	other := clipExecutor(&clipRec{clip: pageValue}, &DeviceContext{})
	if err := other.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
		t.Fatal(err)
	}
	if _, err := other.forSink("open link", "<clipboard>"); err != nil {
		t.Errorf("a clipboard the script never copied to is clean by design: %v", err)
	}
}

// A cloud run has no device context: the executors of one Runner share the flag.
func TestAClipboardTaintSurvivesTheExecutorsOfOneRunnerWithNoDeviceContext(t *testing.T) {
	ctx := context.Background()
	clip := &clipRec{}
	r := &Runner{cfg: &config.Config{}, client: clip}
	first, second := r.newExecutor(), r.newExecutor()
	if first == second {
		t.Fatal("the runner returned one executor twice")
	}
	tainted(first, "t", pageValue)
	if err := first.runStep(ctx, parser.ActionStep{Verb: parser.VerbCopyClipboard, Text: "<t>"}); err != nil {
		t.Fatal(err)
	}
	if err := second.runStep(ctx, parser.ActionStep{Verb: parser.VerbPasteClipboard}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.forSink("open link", "<clipboard>"); err == nil {
		t.Errorf("CROSS-TEST without a device context: the pasted clipboard was clean")
	}
	// a second runner is a second run
	if other := (&Runner{cfg: &config.Config{}, client: clip}).newExecutor(); other.clipboard().get() {
		t.Errorf("a new runner started with a tainted clipboard")
	}
}

func TestTheStopListWordsAreNotInTheResolverList(t *testing.T) {
	for _, r := range plainResolvers {
		if stopAtFuncs["Executor."+r] {
			t.Errorf("%s is both a resolver and stop-listed", r)
		}
	}
}
