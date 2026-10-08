package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// --dry-run must resolve every step: a step that is not built in and matches no
// recipe fails there instead of at runtime.
func TestDryRunReportsUnknownSteps(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.probe")
	src := "recipe \"clear search\"\n  wait 1 seconds\n\n" +
		"test \"bad\"\n  frobnicate the widget\n\n" +
		"test \"good\"\n  clear search\n  tap \"OK\"\n\n" +
		"test \"via recipe\"\n  nested bad\n\n" +
		"recipe \"nested bad\"\n  hide keyboard\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{
		Files: []string{file}, DryRun: true, Timeout: time.Second,
	})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TestResult{}
	for _, res := range results {
		got[res.TestName] = res
	}
	if res := got["bad"]; res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "frobnicate") {
		t.Errorf("bad: want an unknown-step failure, got %+v", res)
	}
	if res := got["good"]; !res.Passed {
		t.Errorf("good: want pass, got %+v", res)
	}
	if res := got["via recipe"]; res.Passed || res.Error == nil || !strings.Contains(res.Error.Error(), "hide keyboard") {
		t.Errorf("via recipe: want the unknown step inside the recipe reported, got %+v", res)
	}
}

// A bare number after a quoted argument is a recipe argument:
// `increment counter "x" 3` calls recipe "increment counter" (identifier, times).
// A number that is part of a recipe's name still resolves as before.
func TestRecipeCallWithBareNumberArgument(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.probe")
	src := "recipe \"increment counter\" (identifier, times)\n  tap on \"<identifier>\"\n\n" +
		"recipe \"step 2 of onboarding\"\n  wait 1 seconds\n\n" +
		"test \"t\"\n  increment counter \"post_form_beds_plus\" 3\n  step 2 of onboarding\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{
		Files: []string{file}, DryRun: true, Timeout: time.Second,
	})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("both calls must resolve, got %+v", results)
	}
}

// seeClient is a client whose See succeeds only for the texts in visible.
type seeClient struct {
	fakeAIClient
	visible map[string]bool
}

func (s *seeClient) See(ctx context.Context, params probelink.SeeParams) error {
	if s.visible[params.Selector.Text] {
		return nil
	}
	return errors.New("not found")
}

func TestWaitAnyReturnsWhenAnyAlternativeIsVisible(t *testing.T) {
	e := &Executor{client: &seeClient{visible: map[string]bool{"Home": true}}, timeout: 5 * time.Second, vars: map[string]string{}}
	if err := e.waitAny(context.Background(), []string{"Got it", "Login", "Home"}); err != nil {
		t.Fatalf("want success, got %v", err)
	}
	e = &Executor{client: &seeClient{}, timeout: 1 * time.Second, vars: map[string]string{}}
	err := e.waitAny(context.Background(), []string{"Got it", "Login"})
	if err == nil || !strings.Contains(err.Error(), "none of") {
		t.Fatalf("want a none-of error, got %v", err)
	}
}

// `use` resolves relative to the file that contains it, also for nested files, and
// --dry-run resolves each file on its own: another file's recipes must not hide an
// unresolved call.
func TestUseNestedAndDryRunIsolation(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("helpers/clear-state.probe", "recipe \"flow clear state\"\n  wait 1 seconds\n")
	write("helpers/login.probe", "use \"../helpers/clear-state.probe\"\n\nrecipe \"flow login\"\n  flow clear state\n")
	nested := write("flows/nav/drawer.probe", "use \"../../helpers/login.probe\"\n\ntest \"drawer\"\n  flow login\n")
	broken := write("flows/nav/broken.probe", "use \"../../helpers/nope.probe\"\n\ntest \"broken\"\n  flow login\n")
	other := write("flows/nav/other.probe", "use \"../../helpers/login.probe\"\n\ntest \"other\"\n  tap \"OK\"\n")

	// Run the whole directory in one dry-run: pooling must not hide `broken`.
	r := New(&config.Config{}, &fakeAIClient{}, &DeviceContext{}, RunOptions{
		Files: []string{nested, broken, other}, DryRun: true, Timeout: time.Second,
	})
	results, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TestResult{}
	for _, res := range results {
		got[res.TestName] = res
	}
	if !got["drawer"].Passed {
		t.Errorf("nested use must resolve relative to the containing file: %+v", got["drawer"])
	}
	if got["broken"].Passed || got["broken"].Error == nil || !strings.Contains(got["broken"].Error.Error(), "do not exist") {
		t.Errorf("a call that only another file's use could satisfy must fail and name the missing use: %+v", got["broken"])
	}
	if !got["other"].Passed {
		t.Errorf("other: %+v", got["other"])
	}
}

func TestEnvExpansionIsMaskedAndPartial(t *testing.T) {
	t.Setenv("PROBE_TEST_EMAIL", "alice@example.com")
	e := &Executor{vars: map[string]string{}}
	if got := e.resolve(`${PROBE_TEST_EMAIL} costs $5 and ${PROBE_UNSET_X}`); got != "alice@example.com costs $5 and ${PROBE_UNSET_X}" {
		t.Fatalf("got %q", got)
	}
	err := e.scrubEnv(fmt.Errorf(`Widget not found: text("alice@example.com") visible: ["alice@example.com"]`))
	if strings.Contains(err.Error(), "alice@example.com") {
		t.Fatalf("value must be scrubbed: %v", err)
	}
	// the step line shows the template, not the value
	desc := e.stepDescription(parser.ActionStep{Verb: parser.VerbType, Text: "${PROBE_TEST_EMAIL}"})
	if !strings.Contains(desc, "${PROBE_TEST_EMAIL}") || strings.Contains(desc, "alice@example.com") {
		t.Fatalf("description leaked the value: %s", desc)
	}
}
