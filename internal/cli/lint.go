package cli

import (
	"fmt"
	"os"

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
				fmt.Printf("  \033[32m✓\033[0m  %s  (%d test(s))\n", f, len(prog.Tests))
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

	return warns
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
