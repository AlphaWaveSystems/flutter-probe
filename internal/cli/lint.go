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
		cfg, err := loadConfig(cmd)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
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

			// Lint checks: a reference a run could not resolve is an error and fails the command; a placeholder that
			// no value defines is a warning. The program rules and the reference rules run in runner.LintFile.
			report, err := runner.LintFile(cfg, f)
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — %s\n", f, err)
				errs++
				continue
			}
			var problems, warnings []string
			for _, ref := range report.References {
				line := fmt.Sprintf("line %d: %s", ref.Line, ref.Message)
				if ref.Warning {
					warnings = append(warnings, line)
				} else {
					problems = append(problems, line)
				}
			}
			var program []string
			for _, p := range report.Program {
				if p.Line > 0 {
					program = append(program, fmt.Sprintf("line %d: %s", p.Line, p.Message))
				} else {
					program = append(program, p.Message)
				}
			}
			warnings = append(program, warnings...)
			if len(problems) == 0 && len(warnings) == 0 {
				statusOK(os.Stdout, "%s  (%d test(s))", f, len(prog.Tests))
				continue
			}
			if len(problems) > 0 {
				errs++
				fmt.Printf("  \033[31m✗\033[0m  %s\n", f)
			} else {
				fmt.Printf("  \033[33m⚠\033[0m  %s\n", f)
			}
			for _, p := range problems {
				fmt.Printf("     %s\n", p)
			}
			for _, w := range warnings {
				fmt.Printf("     %s\n", w)
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
