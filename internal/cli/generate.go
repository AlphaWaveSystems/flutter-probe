package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/spf13/cobra"
)

// providerConfigured reports whether the ai: block names a provider (FP-14).
func providerConfigured(cfg *config.Config) bool {
	return cfg != nil && cfg.AI.Configured()
}

// newGenerator builds the generator for `probe generate`. With an ai.provider
// set (openai | anthropic | local) every call goes to that provider — so a
// local model works with no API key. With no provider it is exactly the
// original behavior: Claude, using --api-key / ai.api_key.
func newGenerator(cfg *config.Config, apiKey, model string) (*ai.Generator, error) {
	if providerConfigured(cfg) {
		c, err := ai.NewTextCompleter(cfg.AI.Provider, config.ResolveEnvVar(cfg.AI.APIKey), model, config.ResolveEnvVar(cfg.AI.Endpoint), cfg.AI.Timeout)
		if err != nil {
			return nil, err
		}
		return ai.NewGeneratorWithCompleter(c), nil
	}
	return ai.NewGenerator(apiKey, model), nil
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AI-powered test generation and maintenance",
}

var generatePromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Generate a .probe test from a natural language description",
	Example: `  probe generate prompt -p "test that a user can sign up with email and password"
  probe generate prompt -p "verify checkout flow" -o tests/checkout.probe
  probe generate prompt -p "login test" --api-key sk-ant-...`,
	RunE: func(cmd *cobra.Command, args []string) error {
		prompt, _ := cmd.Flags().GetString("prompt")
		output, _ := cmd.Flags().GetString("output")
		apiKey, _ := cmd.Flags().GetString("api-key")
		model, _ := cmd.Flags().GetString("model")

		if prompt == "" {
			return fmt.Errorf("--prompt / -p is required")
		}

		// Load config to pick up ai.api_key / ai.model defaults
		cfg, _ := config.Load(".")
		if apiKey == "" && cfg != nil {
			apiKey = config.ResolveEnvVar(cfg.AI.APIKey)
		}
		if model == "" && cfg != nil && cfg.AI.Model != "" {
			model = cfg.AI.Model
		}

		gen, err := newGenerator(cfg, apiKey, model)
		if err != nil {
			return err
		}

		req := ai.GenerateRequest{
			Prompt: prompt,
		}
		if cfg != nil {
			req.AppName = cfg.Project.Name
			req.Platform = cfg.Defaults.Platform
		}

		fmt.Println("  Generating test from prompt...")

		result, err := gen.Generate(cmd.Context(), req)
		if err != nil {
			return fmt.Errorf("generation failed: %w", err)
		}

		// Validate generated ProbeScript through the parser
		if _, parseErr := parser.ParseFile(result.ProbeScript); parseErr != nil {
			fmt.Fprintf(os.Stderr, "  Warning: generated ProbeScript has parse issues: %v\n", parseErr)
			fmt.Fprintln(os.Stderr, "  The output will be written but may need manual fixes.")
		}

		// Determine output path
		if output == "" {
			output = filepath.Join("tests", result.Filename)
		}

		// Write or print
		if output == "-" {
			fmt.Println(result.ProbeScript)
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(output, []byte(result.ProbeScript+"\n"), 0644); err != nil {
			return err
		}

		statusOK(os.Stdout, "Generated → %s", output)
		fmt.Printf("  Description: %s\n", result.Description)
		return nil
	},
}

var generateFromRecordingCmd = &cobra.Command{
	Use:   "from-recording [recording.json]",
	Short: "Generate a .probe file from a probe record output",
	Example: `  probe generate from-recording recordings/onboarding.json --output tests/onboarding.probe
  probe generate from-recording recordings/login.json --name "user can log in"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output, _ := cmd.Flags().GetString("output")
		name, _ := cmd.Flags().GetString("name")
		apiKey, _ := cmd.Flags().GetString("api-key")
		model, _ := cmd.Flags().GetString("model")

		cfg, _ := config.Load(".")
		if apiKey == "" && cfg != nil {
			apiKey = config.ResolveEnvVar(cfg.AI.APIKey)
		}
		if model == "" && cfg != nil && cfg.AI.Model != "" {
			model = cfg.AI.Model
		}

		// No recording file: generate sample
		if len(args) == 0 {
			fmt.Println("  Generating sample test (no recording file provided)")
			gen := ai.NewTestGenerator()
			events := []ai.RecordingEvent{
				{Action: "tap", Target: "Sign In"},
				{Action: "type", Target: "Email", Text: "user@example.com"},
				{Action: "type", Target: "Password", Text: "password"},
				{Action: "tap", Target: "Continue"},
				{Action: "see", Target: "Dashboard"},
			}
			if name == "" {
				name = "generated flow"
			}
			content := gen.GenerateFromRecording(name, events)
			if output == "" {
				output = "tests/generated.probe"
			}
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(output, []byte(content), 0644); err != nil {
				return err
			}
			statusOK(os.Stdout, "Generated → %s", output)
			return nil
		}

		// Load recording file
		recordingPath := args[0]
		data, err := os.ReadFile(recordingPath)
		if err != nil {
			return fmt.Errorf("reading recording: %w", err)
		}

		// Try LLM-powered generation if API key available
		if gen, genErr := newGenerator(cfg, apiKey, model); genErr == nil && (apiKey != "" || providerConfigured(cfg)) {

			prompt := fmt.Sprintf("Convert this recorded user interaction session into a clean, well-structured ProbeScript test.\n\nRecording data:\n%s", string(data))
			if name != "" {
				prompt += fmt.Sprintf("\n\nUse this test name: %q", name)
			}

			req := ai.GenerateRequest{Prompt: prompt}
			if cfg != nil {
				req.AppName = cfg.Project.Name
				req.Platform = cfg.Defaults.Platform
			}

			fmt.Println("  Generating test from recording with AI...")
			result, err := gen.Generate(cmd.Context(), req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  AI generation failed (%v), falling back to heuristic mode\n", err)
			} else {
				return writeGeneratedOutput(output, result.Filename, result.ProbeScript)
			}
		}

		// Heuristic fallback: parse JSON recording events
		var events []ai.RecordingEvent
		if err := json.Unmarshal(data, &events); err != nil {
			return fmt.Errorf("parsing recording JSON: %w", err)
		}

		gen := ai.NewTestGenerator()
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(recordingPath), filepath.Ext(recordingPath))
		}
		content := gen.GenerateFromRecording(name, events)
		return writeGeneratedOutput(output, name+".probe", content)
	},
}

// healNotImplemented is what `probe heal` says. The command used to print two fixed suggestions for files it never
// read and then claim "2 selectors healed" with nothing patched, so it now refuses and points at what exists.
const healNotImplemented = "probe heal is not implemented: it does not scan files and never changed one. " +
	"Use `probe test --ai-heal` for an advisory proposed selector in the JSON report, or Studio's Suggest a fix"

var healCmd = &cobra.Command{
	Use:    "heal [file]",
	Short:  "Not implemented (use probe test --ai-heal)",
	Hidden: true,
	Long: `probe heal is not implemented. An earlier version printed suggestions that did not come from your files and
changed nothing. For a proposed replacement selector after a failed step, run probe test --ai-heal (it needs an
ai: block in probe.yaml; see https://flutterprobe.dev/tools/ai/#selector-proposals-for-failed-steps), or use
Suggest a fix in Studio.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New(healNotImplemented)
	},
}

func init() {
	// generate prompt flags
	generatePromptCmd.Flags().StringP("prompt", "p", "", "natural language test description")
	generatePromptCmd.Flags().StringP("output", "o", "", "output .probe file (default: tests/<inferred>.probe)")
	generatePromptCmd.Flags().String("api-key", "", "Anthropic API key (overrides probe.yaml ai.api_key)")
	generatePromptCmd.Flags().String("model", "", "Claude model name (default: claude-sonnet-4-20250514)")

	// generate from-recording flags
	generateFromRecordingCmd.Flags().StringP("output", "o", "", "output .probe file")
	generateFromRecordingCmd.Flags().StringP("name", "n", "", "test name")
	generateFromRecordingCmd.Flags().String("api-key", "", "Anthropic API key (overrides probe.yaml ai.api_key)")
	generateFromRecordingCmd.Flags().String("model", "", "Claude model name")

	healCmd.Flags().Bool("apply", false, "not implemented")

	generateCmd.AddCommand(generatePromptCmd)
	generateCmd.AddCommand(generateFromRecordingCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(healCmd)
}

// writeGeneratedOutput writes ProbeScript content to a file or stdout.
func writeGeneratedOutput(output, defaultFilename, content string) error {
	if output == "" {
		output = filepath.Join("tests", defaultFilename)
	}
	if output == "-" {
		fmt.Println(content)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(output, []byte(content+"\n"), 0644); err != nil {
		return err
	}
	statusOK(os.Stdout, "Generated → %s", output)
	return nil
}
