package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flutterprobe/probe/internal/report"
	"github.com/flutterprobe/probe/internal/runner"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate an HTML report from a JSON results file",
	Example: `  probe report                                    # reads reports/results.json → reports/report.html
  probe report --input results.json --open        # open in browser after generation
  probe report --input ci/results.json -o ci/report.html`,
	RunE: runReport,
}

func init() {
	reportCmd.Flags().StringP("input", "i", "reports/results.json", "path to JSON results file (from probe test --format json)")
	reportCmd.Flags().String("format", "html", "output format: html (only html is supported)")
	reportCmd.Flags().StringP("output", "o", "reports/report.html", "output file path")
	reportCmd.Flags().Bool("open", false, "open the report in the default browser after generation")
}

// jsonResultFile mirrors the JSON schema produced by runner.Reporter (writeJSON).
type jsonResultFile struct {
	GeneratedAt string `json:"generated_at"`
	TotalTests  int    `json:"total_tests"`
	Passed      int    `json:"passed"`
	Failed      int    `json:"failed"`
	Skipped     int    `json:"skipped"`
	Results     []struct {
		Name      string   `json:"name"`
		File      string   `json:"file"`
		Passed    bool     `json:"passed"`
		Skipped   bool     `json:"skipped"`
		Duration  float64  `json:"duration_ms"`
		Error     string   `json:"error,omitempty"`
		Row       int      `json:"row,omitempty"`
		Artifacts []string `json:"artifacts,omitempty"`
	} `json:"results"`
}

func runReport(cmd *cobra.Command, args []string) error {
	inputPath, _ := cmd.Flags().GetString("input")
	outputPath, _ := cmd.Flags().GetString("output")
	openBrowser, _ := cmd.Flags().GetBool("open")

	// Load project name from config (best-effort, respects --config flag)
	projectName := "FlutterProbe"
	if cfg, err := loadConfig(cmd); err == nil && cfg.Project.Name != "" {
		projectName = cfg.Project.Name
	}

	// Read JSON results file
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading results file %q: %w\n  Run: probe test --format json -o %s", inputPath, err, inputPath)
	}

	var jrf jsonResultFile
	if err := json.Unmarshal(raw, &jrf); err != nil {
		return fmt.Errorf("parsing results file: %w", err)
	}

	if len(jrf.Results) == 0 {
		return errors.New("results file contains no test results")
	}

	// Convert to runner.TestResult slice and build artifacts map
	results := make([]runner.TestResult, 0, len(jrf.Results))
	artifacts := make(map[string][]string)
	for _, r := range jrf.Results {
		tr := runner.TestResult{
			TestName:  r.Name,
			File:      r.File,
			Passed:    r.Passed,
			Skipped:   r.Skipped,
			Duration:  time.Duration(r.Duration) * time.Millisecond,
			Row:       r.Row,
			Artifacts: r.Artifacts,
		}
		if r.Error != "" {
			tr.Error = errors.New(r.Error)
		}
		if len(r.Artifacts) > 0 {
			artifacts[r.Name] = r.Artifacts
		}
		results = append(results, tr)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	// Generate HTML report
	rep := report.NewHTMLReport(outputPath, projectName)
	if err := rep.Write(results, artifacts); err != nil {
		return fmt.Errorf("generating report: %w", err)
	}

	fmt.Printf("  \033[32m✓\033[0m  Report generated → %s\n", outputPath)
	fmt.Printf("       %d passed, %d failed, %d skipped  (%d total)\n",
		jrf.Passed, jrf.Failed, jrf.Skipped, jrf.TotalTests)

	if openBrowser {
		if err := rep.Open(); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not open browser: %v\n", err)
		}
	}
	return nil
}
