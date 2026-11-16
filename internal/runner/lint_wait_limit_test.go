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

// Hooks and recipes of the file are checked, as the attempt that calls them runs them under the limit.
func TestLintWaitLimitReadsHooksAndRecipes(t *testing.T) {
	src := "before each\n  wait 20 seconds\n\nrecipe \"settle\"\n  wait 20 seconds\n\ntest \"t\"\n  open the app\n"
	rep, err := LintSource(limitedConfig(15*time.Second), "t.probe", src)
	if err != nil {
		t.Fatal(err)
	}
	got := waitLimitFindings(rep)
	if len(got) != 2 {
		t.Fatalf("want a warning in the hook and one in the recipe, got %+v", got)
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
