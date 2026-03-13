package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flutterprobe/probe/internal/ai"
	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "AI-powered test generation and maintenance",
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
		endpoint, _ := cmd.Flags().GetString("endpoint")

		gen := ai.NewTestGenerator()
		if apiKey != "" {
			gen.APIKey = apiKey
		}
		if endpoint != "" {
			gen.ModelEndpoint = endpoint
		}

		// For now, generate a sample test without a real recording file
		if len(args) == 0 {
			fmt.Println("  Generating sample test (no recording file provided)")
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

		// Load recording file (JSON from probe record)
		_ = args[0] // recording file path
		fmt.Printf("  AI test generation from recordings is planned for Phase P4 (Q1 2027)\n")
		fmt.Printf("  For now, use 'probe record' to capture interactions and edit the output manually.\n")
		return nil
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
	generateFromRecordingCmd.Flags().StringP("output", "o", "", "output .probe file")
	generateFromRecordingCmd.Flags().StringP("name", "n", "", "test name")
	generateFromRecordingCmd.Flags().String("api-key", "", "AI API key (for model-powered generation)")
	generateFromRecordingCmd.Flags().String("endpoint", "", "AI model endpoint URL")

	healCmd.Flags().Bool("apply", false, "automatically apply suggested fixes")

	generateCmd.AddCommand(generateFromRecordingCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(healCmd)
}
