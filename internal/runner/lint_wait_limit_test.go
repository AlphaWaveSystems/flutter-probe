package runner

import (
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// waitLimitFindings returns the findings of rep that are the wait-limit warning.
func waitLimitFindings(rep LintReport) []LintFinding {
	var out []LintFinding
	for _, f := range rep.Program {
		if strings.Contains(f.Message, "at or above test_timeout") {
			out = append(out, f)
		}
	}
	return out
}

func limitedConfig(limit time.Duration) *config.Config {
	cfg := &config.Config{}
	cfg.Defaults.TestTimeout = limit
	return cfg
}

// A literal wait at or above the configured limit is a warning at its line: the test fails there when it runs.
func TestLintWarnsAWaitAtOrAboveTheLimit(t *testing.T) {
	src := "test \"slow\"\n  open the app\n  wait 20 seconds\n  wait 3 seconds\n"
	rep, err := LintSource(limitedConfig(15*time.Second), "slow.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	got := waitLimitFindings(rep)
	if len(got) != 1 || got[0].Line != 3 || !got[0].Warning {
		t.Fatalf("want one warning at line 3 for the 20 second wait, got %+v", rep.Program)
	}
	if !strings.Contains(got[0].Message, "wait 20 seconds is at or above test_timeout (15s)") {
		t.Errorf("the message must name the wait and the limit, got %q", got[0].Message)
	}
}

// Without a configured limit there is no warning, and a wait below the limit is not warned.
func TestLintWaitLimitIsOffWithoutATestTimeout(t *testing.T) {
	src := "test \"slow\"\n  open the app\n  wait 20 seconds\n"
	rep, err := LintSource(&config.Config{}, "slow.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	if got := waitLimitFindings(rep); len(got) != 0 {
		t.Errorf("without test_timeout there is no limit to warn about, got %+v", got)
	}
	rep, err = LintSource(limitedConfig(25*time.Second), "slow.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	if got := waitLimitFindings(rep); len(got) != 0 {
		t.Errorf("a wait below the limit is not warned, got %+v", got)
	}
}

// lintWaitLimitCount returns how many wait-limit warnings src gets under a 15 second limit.
func lintWaitLimitCount(t *testing.T, src string) int {
	t.Helper()
	rep, err := LintSource(limitedConfig(15*time.Second), "t.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	return len(waitLimitFindings(rep))
}

// The steps the attempt runs under the limit warn: a test body, a before-each hook, and a recipe that one of them calls, directly or
// through another recipe. The runtime refuses the wait in each of these, so each is a warning.
func TestLintWaitLimitWarnsWhereTheRuntimeRefuses(t *testing.T) {
	const settle = "recipe \"settle\"\n  wait 20 seconds\n\n"
	cases := map[string]string{
		"test body":               "test \"t\"\n  wait 20 seconds\n",
		"before each":             "before each test\n  wait 20 seconds\n\ntest \"t\"\n  open the app\n",
		"recipe from a test":      settle + "test \"t\"\n  settle\n",
		"recipe from before each": settle + "before each test\n  settle\n\ntest \"t\"\n  open the app\n",
		"recipe through a recipe": settle + "recipe \"outer\"\n  settle\n\ntest \"t\"\n  outer\n",
	}
	for name, src := range cases {
		if got := lintWaitLimitCount(t, src); got != 1 {
			t.Errorf("%s: want one warning, got %d for:\n%s", name, got, src)
		}
	}
}

// The steps that run where the limit is cleared, or with none, do not warn when they are the wait itself: the after-each and on-failure
// hooks, and before-all and after-all, since the runtime lets their wait run to its end.
func TestLintWaitLimitDoesNotWarnWhereTheLimitIsCleared(t *testing.T) {
	const body = "test \"t\"\n  open the app\n"
	cases := map[string]string{
		"after each": body + "\nafter each test\n  wait 20 seconds\n",
		"on failure": body + "\non failure\n  wait 20 seconds\n",
		"before all": "before all tests\n  wait 20 seconds\n\n" + body,
		"after all":  body + "\nafter all tests\n  wait 20 seconds\n",
	}
	for name, src := range cases {
		if got := lintWaitLimitCount(t, src); got != 0 {
			t.Errorf("%s: a wait the runtime lets run must not warn, got %d for:\n%s", name, got, src)
		}
	}
}

// A recipe that no test body or before-each hook of this file reaches is warned with its condition, since a test in another file may call
// it under the limit: a recipe only an after-each hook calls, and a recipe nothing here calls. The message names the calls that fail.
func TestLintWaitLimitWarnsAnUnreachedRecipeWithItsCondition(t *testing.T) {
	const settle = "recipe \"settle\"\n  wait 20 seconds\n\n"
	const body = "test \"t\"\n  open the app\n"
	cases := map[string]string{
		"recipe from after each": settle + body + "\nafter each test\n  settle\n",
		"recipe nothing calls":   settle + body,
	}
	for name, src := range cases {
		rep, err := LintSource(limitedConfig(15*time.Second), "t.probe", src)
		if err != nil {
			t.Fatal(err)
		}
		got := waitLimitFindings(rep)
		if len(got) != 1 {
			t.Errorf("%s: want one conditional warning, got %d for:\n%s", name, len(got), src)
			continue
		}
		if !strings.Contains(got[0].Message, "a test that calls the recipe in its body or in a before each hook fails at once there") {
			t.Errorf("%s: the warning must name the calls that fail, got %q", name, got[0].Message)
		}
	}
}

// A recipe that a test body or a before-each hook reaches is warned unconditionally: the wait fails at once there.
func TestLintWaitLimitWarnsAReachedRecipeUnconditionally(t *testing.T) {
	src := "recipe \"settle\"\n  wait 20 seconds\n\ntest \"t\"\n  settle\n"
	rep, err := LintSource(limitedConfig(15*time.Second), "t.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	got := waitLimitFindings(rep)
	if len(got) != 1 {
		t.Fatalf("want one warning, got %+v", got)
	}
	if !strings.Contains(got[0].Message, "the test fails at once when it runs") {
		t.Errorf("a recipe a test reaches must be warned unconditionally, got %q", got[0].Message)
	}
}

// A composite test's steps run without the limit (the composite runner does not apply it), so they are not warned.
func TestLintWaitLimitSkipsCompositeTests(t *testing.T) {
	src := "composite test \"two devices\"\n  devices:\n    A: iPhone 15 Simulator\n    B: Pixel 9 Emulator\n  A:\n    wait 20 seconds\n  B:\n    tap \"Fine\"\n"
	rep, err := LintSource(limitedConfig(15*time.Second), "c.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	if got := waitLimitFindings(rep); len(got) != 0 {
		t.Errorf("a composite test is not checked for the limit, got %+v", got)
	}
}
