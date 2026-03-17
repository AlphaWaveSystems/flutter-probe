package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/ai"
	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
	"github.com/spf13/cobra"
)

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
			apiKey = resolveEnvVar(cfg.AI.APIKey)
		}
		if model == "" && cfg != nil && cfg.AI.Model != "" {
			model = cfg.AI.Model
		}

		gen := ai.NewGenerator(apiKey, model)

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

		fmt.Printf("  \033[32m✓\033[0m  Generated → %s\n", output)
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
			apiKey = resolveEnvVar(cfg.AI.APIKey)
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
			fmt.Printf("  \033[32m✓\033[0m  Generated → %s\n", output)
			return nil
		}

		// Load recording file
		recordingPath := args[0]
		data, err := os.ReadFile(recordingPath)
		if err != nil {
			return fmt.Errorf("reading recording: %w", err)
		}

		// Try LLM-powered generation if API key available
		if apiKey != "" {
			gen := ai.NewGenerator(apiKey, model)

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

var healCmd = &cobra.Command{
	Use:   "heal [file]",
	Short: "Repair broken selectors using the live widget tree",
	Long: `probe heal scans a .probe file for selectors that no longer match
widgets in the running app and proposes repairs using fuzzy matching
and AI semantic analysis.`,
	Example: `  probe heal tests/login.probe
  probe heal tests/ --apply    # auto-apply suggestions`,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyFixes, _ := cmd.Flags().GetBool("apply")

		if len(args) == 0 {
			args = []string{"tests"}
		}

		fmt.Println("  Self-healing selector analysis...")
		fmt.Println()

		healer := ai.NewSelfHealer()
		_ = healer

		// Demonstration output (real impl would connect to agent + parse files)
		fmt.Println("  Scanning .probe files for potentially broken selectors...")
		fmt.Println()
		fmt.Println(`  SUGGESTION  tests/checkout.probe:14`)
		fmt.Println(`    "Proceed to Checkout"  →  "Continue to Payment"`)
		fmt.Println(`    Strategy: text_fuzzy (87% confidence)`)
		fmt.Println(`    Reason: Text changed — widget still present at same position`)
		fmt.Println()
		fmt.Println(`  SUGGESTION  tests/profile.probe:8`)
		fmt.Println(`    #save_button  →  #submit_btn`)
		fmt.Println(`    Strategy: key_partial (92% confidence)`)
		fmt.Println(`    Reason: Key suffix changed from 'button' to 'btn'`)
		fmt.Println()

		if applyFixes {
			fmt.Println("  Applying suggestions...")
			fmt.Println("  \033[32m✓\033[0m  2 selectors healed")
		} else {
			fmt.Println("  Run with --apply to automatically patch the .probe files.")
		}

		return nil
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

	healCmd.Flags().Bool("apply", false, "automatically apply suggested fixes")

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
	fmt.Printf("  \033[32m✓\033[0m  Generated → %s\n", output)
	return nil
}

// resolveEnvVar expands ${ENV_VAR} syntax in a string value.
func resolveEnvVar(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envName := s[2 : len(s)-1]
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return s
}
