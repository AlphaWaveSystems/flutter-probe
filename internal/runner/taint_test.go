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

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
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
