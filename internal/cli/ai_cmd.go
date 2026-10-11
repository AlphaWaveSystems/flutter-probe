package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// Everything in this file is optional and advisory (FP-14). `probe test`
// reaches it only when --ai-triage is passed, only after results are final,
// and through maybeAITriage, which cannot return an error or change the exit
// code. Running tests never requires an ai: block, a model, or a network.

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "Optional AI helpers (local or hosted models) — never required to run tests",
}

var aiDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the configured ai: provider (endpoint, model, text, vision)",
	Long: `Checks the ai: block in probe.yaml end to end: the endpoint answers, the model is
available, a plain text request works, and whether the model accepts images.

A text-only model is a supported setup: triage and generation only need text, and
with ai.vision: false, ` + "`see \"...\" with ai`" + ` is evaluated against the screen's visible texts.
The exit code reflects this check only — it has no effect on running tests.`,
	Example: `  probe ai doctor
  probe ai doctor --config probe.yaml`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig(cmd)
		if err != nil {
			return err
		}
		rep := ai.Doctor(cmd.Context(), ai.DoctorInput{
			Provider: cfg.AI.Provider,
			APIKey:   config.ResolveEnvVar(cfg.AI.APIKey),
			Model:    cfg.AI.Model,
			Endpoint: config.ResolveEnvVar(cfg.AI.Endpoint),
			Timeout:  cfg.AI.Timeout,
		})
		for _, c := range rep.Checks {
			if c.OK {
				statusOK(os.Stdout, "%s: %s", c.Name, c.Detail)
			} else {
				statusFail(os.Stdout, "%s: %s", c.Name, c.Detail)
			}
		}
		if !rep.OK() {
			return fmt.Errorf("ai: provider is not usable (see above)")
		}
		return nil
	},
}

var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Explain failures from a JSON results file with the configured AI model (advisory)",
	Example: `  probe test tests/ --format json -o reports/results.json
  probe triage --input reports/results.json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		input, _ := cmd.Flags().GetString("input")
		output, _ := cmd.Flags().GetString("output")
		cfg, err := loadConfig(cmd)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("reading %s: %w", input, err)
		}
		failures, err := failuresFromJSONReport(data)
		if err != nil {
			return err
		}
		if len(failures) == 0 {
			fmt.Println("  No failed tests in", input)
			return nil
		}
		md, err := triageToMarkdown(cmd.Context(), cfg, failures)
		if err != nil {
			return err
		}
		fmt.Print(md)
		if output != "" {
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			return os.WriteFile(output, []byte(md), 0o644)
		}
		return nil
	},
}

func init() {
	aiCmd.AddCommand(aiDoctorCmd)
	rootCmd.AddCommand(aiCmd)

	f := triageCmd.Flags()
	f.String("input", "reports/results.json", "JSON results file written by `probe test --format json`")
	f.StringP("output", "o", "", "also write the advice to this markdown file")
	rootCmd.AddCommand(triageCmd)
}

// failuresFromJSONReport extracts failed (not skipped) tests from a
// `probe test --format json` report.
func failuresFromJSONReport(data []byte) ([]ai.Failure, error) {
	var rep struct {
		Results []struct {
			Name    string `json:"name"`
			File    string `json:"file"`
			Passed  bool   `json:"passed"`
			Skipped bool   `json:"skipped"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, fmt.Errorf("not a probe JSON report: %w", err)
	}
	var out []ai.Failure
	for _, r := range rep.Results {
		if !r.Passed && !r.Skipped {
			out = append(out, ai.Failure{Test: r.Name, File: r.File, Error: r.Error})
		}
	}
	return out, nil
}

// triageToMarkdown runs triage with the ai: block's provider. An unusable or
// missing ai: block is an error here because the user explicitly asked for
// triage (probe triage); maybeAITriage downgrades it to a note.
func triageToMarkdown(ctx context.Context, cfg *config.Config, failures []ai.Failure) (string, error) {
	if !cfg.AI.Configured() {
		return "", fmt.Errorf("triage needs an ai: provider in probe.yaml (e.g. provider: local with endpoint and model) — see `probe ai doctor`")
	}
	c, err := ai.NewTextCompleter(cfg.AI.Provider, config.ResolveEnvVar(cfg.AI.APIKey), cfg.AI.Model, config.ResolveEnvVar(cfg.AI.Endpoint), cfg.AI.Timeout)
	if err != nil {
		return "", err
	}
	notes, stopped := ai.Triage(ctx, c, failures, ai.TriageOptions{PerCall: cfg.AI.Timeout})
	return ai.FormatTriageMarkdown(notes, stopped), nil
}

// maybeAITriage implements `probe test --ai-triage`. It is called after the
// report has been written and the results are final, and it cannot return an
// error: any problem becomes a one-line note on w. It never touches results,
// the report files already written, or the exit code.
func maybeAITriage(ctx context.Context, w io.Writer, cfg *config.Config, results []runner.TestResult, reportsDir string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(w, "  AI triage skipped: %v\n", r)
		}
	}()
	var failures []ai.Failure
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			msg := ""
			if r.Error != nil {
				msg = r.Error.Error()
			}
			failures = append(failures, ai.Failure{Test: r.TestName, File: r.File, Error: msg})
		}
	}
	if len(failures) == 0 {
		return
	}
	if !cfg.AI.Configured() {
		fmt.Fprintln(w, "  AI triage skipped: no ai: provider configured in probe.yaml (tests are unaffected)")
		return
	}
	// A hard overall ceiling on top of the per-call timeouts, so a wedged
	// model server can never keep the CLI from exiting.
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	md, err := triageToMarkdown(tctx, cfg, failures)
	if err != nil {
		fmt.Fprintf(w, "  AI triage skipped: %v (tests are unaffected)\n", err)
		return
	}
	fmt.Fprint(w, "\n"+md)
	if reportsDir != "" {
		path := filepath.Join(reportsDir, "triage.md")
		if err := os.MkdirAll(reportsDir, 0o755); err == nil {
			if werr := os.WriteFile(path, []byte(md), 0o644); werr == nil {
				fmt.Fprintf(w, "  AI triage written to %s\n", path)
			}
		}
	}
}
