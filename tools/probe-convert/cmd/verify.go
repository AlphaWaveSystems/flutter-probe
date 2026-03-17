package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/probe-convert/ui"
)

// findProbe locates the probe binary. Search order:
// 1. --probe-path flag (if set)
// 2. Adjacent to probe-convert binary (../../bin/probe or ../probe)
// 3. $GOPATH/bin/probe
// 4. $PATH
func findProbe(flagPath string) (string, error) {
	if flagPath != "" {
		if _, err := os.Stat(flagPath); err == nil {
			return flagPath, nil
		}
		return "", fmt.Errorf("probe binary not found at specified path: %s", flagPath)
	}

	// Adjacent to this binary.
	self, err := os.Executable()
	if err == nil {
		selfDir := filepath.Dir(self)
		candidates := []string{
			filepath.Join(selfDir, "probe"),
			filepath.Join(selfDir, "..", "probe"),
			filepath.Join(selfDir, "..", "..", "bin", "probe"),
		}
		for _, c := range candidates {
			if abs, err := filepath.Abs(c); err == nil {
				if _, err := os.Stat(abs); err == nil {
					return abs, nil
				}
			}
		}
	}

	// $GOPATH/bin.
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		p := filepath.Join(gopath, "bin", "probe")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// $PATH.
	if p, err := exec.LookPath("probe"); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("probe binary not found — install with 'make install' from the FlutterProbe root, or pass --probe-path")
}

// lintResult holds the outcome of linting a single .probe file.
type lintResult struct {
	File     string
	Passed   bool
	Tests    int
	Warnings []string
	Error    string
}

// runLint runs `probe lint` on the given .probe files and returns per-file results.
func runLint(probeBin string, files []string) ([]lintResult, error) {
	var results []lintResult

	for _, f := range files {
		cmd := exec.Command(probeBin, "lint", f)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		out := stdout.String() + stderr.String()

		lr := lintResult{File: f}

		if err != nil {
			// Parse error output for specifics.
			lr.Passed = false
			lr.Error = extractLintError(out, f)
		} else {
			lr.Passed = true
			lr.Tests = extractTestCount(out)
			lr.Warnings = extractLintWarnings(out)
		}
		results = append(results, lr)
	}

	return results, nil
}

// runVerify runs `probe test --dry-run` on .probe files for full validation.
func runVerify(probeBin string, files []string) ([]lintResult, error) {
	var results []lintResult

	for _, f := range files {
		cmd := exec.Command(probeBin, "test", f, "--dry-run")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		out := stdout.String() + stderr.String()

		lr := lintResult{File: f}

		if err != nil {
			lr.Passed = false
			lr.Error = extractVerifyError(out, f)
		} else {
			lr.Passed = true
			lr.Tests = countTestLines(out)
		}
		results = append(results, lr)
	}

	return results, nil
}

// printLintResults displays lint/verify results in the terminal.
func printLintResults(header string, results []lintResult) {
	fmt.Println()
	fmt.Printf("  %s\n", ui.C(ui.Bold, header))
	fmt.Println("  " + strings.Repeat("─", 50))

	passed, failed := 0, 0
	for _, r := range results {
		name := filepath.Base(r.File)
		if r.Passed {
			passed++
			suffix := ""
			if r.Tests > 0 {
				suffix = fmt.Sprintf(" (%d tests)", r.Tests)
			}
			if len(r.Warnings) > 0 {
				suffix += fmt.Sprintf(" %s", ui.C(ui.Yellow, fmt.Sprintf("[%d warnings]", len(r.Warnings))))
			}
			fmt.Printf("  %s  %s%s\n", ui.C(ui.Green, "✓"), name, suffix)
			for _, w := range r.Warnings {
				fmt.Printf("     %s\n", ui.C(ui.Yellow, w))
			}
		} else {
			failed++
			fmt.Printf("  %s  %s — %s\n", ui.C(ui.Red, "✗"), name, ui.C(ui.Red, r.Error))
		}
	}

	fmt.Println("  " + strings.Repeat("─", 50))
	summary := fmt.Sprintf("  %d passed", passed)
	if failed > 0 {
		summary += fmt.Sprintf(", %s", ui.C(ui.Red, fmt.Sprintf("%d failed", failed)))
	}
	fmt.Println(summary)
	fmt.Println()
}

// extractLintError pulls the error message from probe lint output.
func extractLintError(out, file string) string {
	base := filepath.Base(file)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// Strip ANSI codes for matching.
		plain := stripANSI(line)
		if strings.Contains(plain, base) && strings.Contains(plain, "parse error") {
			// Extract just the error part after "parse error:"
			if idx := strings.Index(plain, "parse error:"); idx >= 0 {
				return strings.TrimSpace(plain[idx:])
			}
			return plain
		}
		if strings.Contains(plain, base) && strings.Contains(plain, "✗") {
			// Strip the ✗ prefix and filename.
			if idx := strings.Index(plain, "—"); idx >= 0 {
				return strings.TrimSpace(plain[idx+len("—"):])
			}
		}
	}
	out = strings.TrimSpace(out)
	if out != "" {
		// Return first non-empty line.
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				return stripANSI(line)
			}
		}
	}
	return "lint failed"
}

// extractTestCount parses "(N test(s))" from probe lint output.
func extractTestCount(out string) int {
	// Look for pattern like "(3 test(s))"
	for _, line := range strings.Split(out, "\n") {
		plain := stripANSI(line)
		if idx := strings.Index(plain, "("); idx >= 0 {
			end := strings.Index(plain[idx:], " test")
			if end > 0 {
				numStr := strings.TrimSpace(plain[idx+1 : idx+end])
				n := 0
				fmt.Sscanf(numStr, "%d", &n)
				return n
			}
		}
	}
	return 0
}

// extractLintWarnings pulls warning lines from probe lint output.
func extractLintWarnings(out string) []string {
	var warnings []string
	for _, line := range strings.Split(out, "\n") {
		plain := stripANSI(strings.TrimSpace(line))
		// Warning lines are indented and don't start with ✓ or ✗
		if strings.HasPrefix(plain, "test ") && strings.Contains(plain, "has no steps") {
			warnings = append(warnings, plain)
		}
		if strings.Contains(plain, "dart blocks") {
			warnings = append(warnings, plain)
		}
	}
	return warnings
}

// extractVerifyError pulls error from probe test --dry-run output.
func extractVerifyError(out, file string) string {
	plain := stripANSI(strings.TrimSpace(out))
	if plain == "" {
		return "dry-run verification failed"
	}
	// Look for "error:" line
	for _, line := range strings.Split(plain, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "error:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "error:"))
		}
	}
	// Return first non-empty line.
	for _, line := range strings.Split(plain, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return "dry-run verification failed"
}

// countTestLines counts ✓ lines in probe test --dry-run output.
func countTestLines(out string) int {
	count := 0
	for _, line := range strings.Split(out, "\n") {
		plain := stripANSI(line)
		if strings.Contains(plain, "✓") || strings.Contains(plain, "passed") {
			count++
		}
	}
	// The summary line "N passed" is more reliable.
	for _, line := range strings.Split(out, "\n") {
		plain := stripANSI(strings.TrimSpace(line))
		n := 0
		if _, err := fmt.Sscanf(plain, "%d passed", &n); err == nil && n > 0 {
			return n
		}
	}
	return count
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until 'm'.
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
