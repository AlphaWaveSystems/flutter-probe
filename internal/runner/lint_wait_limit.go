package runner

import (
	"fmt"
	"strconv"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// lintWaitLimit warns for a literal `wait N seconds` at or above the configured test_timeout. A wait the runtime refuses is a plain warning:
// an attempt refuses it when it starts, in a test's body, in a before-each hook, and in a recipe that one of those calls, directly or through
// another recipe. The limit is cleared before the after-each and on-failure hooks run, and before-all and after-all run with no limit, so a
// wait directly in one of them is not warned. A composite test's steps run without the limit too, so they are not checked.
//
// A recipe of this file that no test body or before-each hook here reaches may still be called under the limit by a test in another file (a
// recipes folder is read by every file that uses it), so its waits are warned with a condition instead: it fails at once where a test calls
// it in the body or in a before-each hook. A recipe only an after-each hook calls is warned the same way, a conditional that is true of the
// calls the runtime refuses and quiet about the others.
func lintWaitLimit(cfg *config.Config, prog *parser.Program) []LintFinding {
	if cfg == nil || cfg.Defaults.TestTimeout <= 0 {
		return nil
	}
	limit := cfg.Defaults.TestTimeout

	recipes := map[string]parser.RecipeDef{}
	for _, rec := range prog.Recipes {
		recipes[rec.Name] = rec
	}

	// The bodies the attempt runs under the limit: the test bodies and the before-each hooks, then the recipes they reach.
	var queue [][]parser.Step
	for _, t := range prog.Tests {
		queue = append(queue, t.Body)
	}
	for _, h := range prog.Hooks {
		if h.Kind == parser.HookBeforeEach {
			queue = append(queue, h.Body)
		}
	}
	reached := map[string]bool{}
	var bodies [][]parser.Step
	for len(queue) > 0 {
		body := queue[0]
		queue = queue[1:]
		bodies = append(bodies, body)
		walkSteps(body, func(s parser.Step) {
			rc, ok := s.(parser.RecipeCall)
			if !ok {
				return
			}
			rec, _, _, found := resolveCall(recipes, rc)
			if found && !reached[rec.Name] {
				reached[rec.Name] = true
				queue = append(queue, rec.Body)
			}
		})
	}

	var out []LintFinding
	for _, body := range bodies {
		walkSteps(body, func(s parser.Step) {
			if w, ok := limitWait(s, limit); ok {
				out = append(out, LintFinding{Line: w.Line, Warning: true, Message: fmt.Sprintf(
					"wait %s seconds is at or above test_timeout (%s): the test fails at once when it runs; wait until the screen shows what the test needs, or raise test_timeout",
					strconv.FormatFloat(w.Duration, 'f', -1, 64), formatLimit(limit))})
			}
		})
	}
	for _, rec := range prog.Recipes {
		if reached[rec.Name] {
			continue
		}
		walkSteps(rec.Body, func(s parser.Step) {
			if w, ok := limitWait(s, limit); ok {
				out = append(out, LintFinding{Line: w.Line, Warning: true, Message: fmt.Sprintf(
					"wait %s seconds in recipe %q is at or above test_timeout (%s): a test that calls the recipe in its body or in a before each hook fails at once there; wait until the screen shows what the test needs, or raise test_timeout",
					strconv.FormatFloat(w.Duration, 'f', -1, 64), rec.Name, formatLimit(limit))})
			}
		})
	}
	return out
}

// limitWait returns the step as a literal wait when it is one at or above limit.
func limitWait(s parser.Step, limit time.Duration) (parser.WaitStep, bool) {
	w, ok := s.(parser.WaitStep)
	if !ok || w.Kind != parser.WaitDuration || time.Duration(w.Duration*float64(time.Second)) < limit {
		return w, false
	}
	return w, true
}
