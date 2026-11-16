package runner

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// lintWaitLimit warns for a literal `wait N seconds` at or above the configured test_timeout. A test whose attempt reaches such a wait fails
// at once when it runs (the executor refuses it), so the warning points at the wait before a run does. It reads the same config as the rest of
// the lint: without a test_timeout there is no limit and no warning. Composite tests are not checked, since their steps run without the limit.
// A recipe defined in another file is not checked here; its own file is.
func lintWaitLimit(cfg *config.Config, prog *parser.Program) []LintFinding {
	if cfg == nil || cfg.Defaults.TestTimeout <= 0 {
		return nil
	}
	limit := cfg.Defaults.TestTimeout
	var out []LintFinding
	check := func(body []parser.Step) {
		walkSteps(body, func(s parser.Step) {
			w, ok := s.(parser.WaitStep)
			if !ok || w.Kind != parser.WaitDuration || time.Duration(w.Duration*float64(time.Second)) < limit {
				return
			}
			out = append(out, LintFinding{Line: w.Line, Warning: true, Message: fmt.Sprintf(
				"wait %s seconds is at or above test_timeout (%s): the test fails at once when it runs; wait until the screen shows what the test needs, or raise test_timeout",
				strconv.FormatFloat(w.Duration, 'f', -1, 64), formatLimit(limit))})
		})
	}
	for _, t := range prog.Tests {
		check(t.Body)
	}
	for _, h := range prog.Hooks {
		check(h.Body)
	}
	for _, rec := range prog.Recipes {
		check(rec.Body)
	}
	return out
}
