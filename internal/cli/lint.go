package cli

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/cobra"
)

var lintCmd = &cobra.Command{
	Use:   "lint [file|dir]...",
	Short: "Validate .probe files without running them",
	Example: `  probe lint tests/
  probe lint tests/login.probe`,
	RunE: func(cmd *cobra.Command, args []string) error {
		searchPaths := args
		if len(searchPaths) == 0 {
			searchPaths = []string{"tests"}
		}

		files, err := runner.CollectFiles(searchPaths)
		if err != nil {
			return fmt.Errorf("collecting files: %w", err)
		}

		errs := 0
		for _, f := range files {
			src, err := os.ReadFile(f)
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — %s\n", f, err)
				errs++
				continue
			}
			prog, err := parser.ParseFile(string(src))
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — parse error: %s\n", f, err)
				errs++
				continue
			}

			// Lint checks
			warnings := lintProgram(prog)
			if len(warnings) > 0 {
				fmt.Printf("  \033[33m⚠\033[0m  %s\n", f)
				for _, w := range warnings {
					fmt.Printf("     %s\n", w)
				}
			} else {
				statusOK(os.Stdout, "%s  (%d test(s))", f, len(prog.Tests))
			}
		}

		fmt.Println()
		if errs > 0 {
			return fmt.Errorf("%d file(s) have errors", errs)
		}
		fmt.Println("  All files valid.")
		return nil
	},
}

// lintProgram runs static checks on a parsed program and returns warnings.
func lintProgram(prog *parser.Program) []string {
	var warns []string

	// Check: each test has at least one step
	for _, t := range prog.Tests {
		if len(t.Body) == 0 {
			warns = append(warns, fmt.Sprintf("test %q has no steps", t.Name))
		}
		// Check: dart usage > 20% of steps
		dartCount := countDartBlocks(t.Body)
		total := len(t.Body)
		if total > 0 && dartCount*100/total > 20 {
			warns = append(warns, fmt.Sprintf(
				"test %q: %.0f%% dart blocks (consider ProbeScript instead)",
				t.Name, float64(dartCount*100)/float64(total),
			))
		}
	}

	// Check: recipes referenced by `use` that don't exist
	// (static check — file existence not verified here)

	// Check: a count or zero-state text check that a substring match passes by accident
	for _, t := range prog.Tests {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("test %q", t.Name), t.Body)...)
	}
	for _, h := range prog.Hooks {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("%s hook", h.Kind), h.Body)...)
	}
	for _, r := range prog.Recipes {
		warns = append(warns, substringCountWarnings(fmt.Sprintf("recipe %q", r.Name), r.Body)...)
	}

	return warns
}

// countOrZeroText matches a bare count or a count with one word ("0", "3 items"), and zeroStateText a zero state
// ("None", "No items", "Nothing here"). Longer sentences and decimals are not matched, to keep the warning conservative.
var (
	countOrZeroText = regexp.MustCompile(`(?i)^[0-9]+( [a-z]+)?$`)
	zeroStateText   = regexp.MustCompile(`(?i)^(no|none|nothing|empty|zero)( [a-z]+){0,2}$`)
)

// substringCountWarnings flags a `see` or `wait until ... appears` on a count or zero-state text with no matching
// pattern. A substring match passes "0 items" on "10 items", so the check should be anchored. A negated check is not
// flagged, because its substring match fails loudly rather than passing by accident, and a step with any matching
// pattern has already chosen how its text is compared.
func substringCountWarnings(owner string, steps []parser.Step) []string {
	var warns []string
	for _, s := range steps {
		switch st := s.(type) {
		case parser.AssertStep:
			if st.Negated || st.Native || st.WithAI || st.Pattern != "" || len(st.Any) > 0 || st.Sel.Kind != parser.SelectorText {
				continue
			}
			warns = appendSubstringCountWarning(warns, owner, st.Line, st.Sel.Text)
		case parser.WaitStep:
			if st.Kind == parser.WaitAppears && st.Pattern == "" {
				warns = appendSubstringCountWarning(warns, owner, st.Line, st.Target)
			}
		case parser.ConditionalStep:
			warns = append(warns, substringCountWarnings(owner, st.Then)...)
			warns = append(warns, substringCountWarnings(owner, st.Else)...)
		case parser.LoopStep:
			warns = append(warns, substringCountWarnings(owner, st.Body)...)
		case parser.RetryStep:
			warns = append(warns, substringCountWarnings(owner, st.Body)...)
		case parser.TimedStep:
			warns = append(warns, substringCountWarnings(owner, []parser.Step{st.Inner})...)
		}
	}
	return warns
}

func appendSubstringCountWarning(warns []string, owner string, line int, text string) []string {
	text = strings.TrimSpace(text)
	if !countOrZeroText.MatchString(text) && !zeroStateText.MatchString(text) {
		return warns
	}
	return append(warns, fmt.Sprintf(
		"%s, line %d: %q matches any longer text that contains it; for an exact count or zero-state check add matching \"^%s$\"",
		owner, line, text, text))
}

func countDartBlocks(steps []parser.Step) int {
	n := 0
	for _, s := range steps {
		if _, ok := s.(parser.DartBlock); ok {
			n++
		}
	}
	return n
}
