package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold probe.yaml and tests/ directory in the current Flutter project",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _ := os.Getwd()

		// Write probe.yaml
		cfgPath := filepath.Join(dir, "probe.yaml")
		if _, err := os.Stat(cfgPath); err == nil {
			fmt.Println("probe.yaml already exists — skipping")
		} else {
			if err := os.WriteFile(cfgPath, []byte(config.DefaultYAML), 0644); err != nil {
				return fmt.Errorf("writing probe.yaml: %w", err)
			}
			fmt.Println("  \033[32m✓\033[0m  Created probe.yaml")
		}

		// Create directory structure
		dirs := []string{
			filepath.Join(dir, "tests"),
			filepath.Join(dir, "tests", "recipes"),
			filepath.Join(dir, "reports"),
		}
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0755); err != nil {
				return fmt.Errorf("creating %s: %w", d, err)
			}
		}

		// Write a sample .probe file
		sample := filepath.Join(dir, "tests", "example.probe")
		if _, err := os.Stat(sample); err != nil {
			content := `test "the app launches successfully"
  open the app
  wait until "Welcome" appears
  see "Welcome"
`
			if err := os.WriteFile(sample, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing sample test: %w", err)
			}
			fmt.Println("  \033[32m✓\033[0m  Created tests/example.probe")
		}

		// Write a sample recipe
		recipe := filepath.Join(dir, "tests", "recipes", "login.probe")
		if _, err := os.Stat(recipe); err != nil {
			content := `recipe "log in as" (email, password)
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type <email> into the "Email" field
  type <password> into the "Password" field
  tap "Continue"
  see "Dashboard"
`
			if err := os.WriteFile(recipe, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing sample recipe: %w", err)
			}
			fmt.Println("  \033[32m✓\033[0m  Created tests/recipes/login.probe")
		}

		fmt.Println()
		fmt.Println("  FlutterProbe is ready. Run \033[1mprobe test\033[0m to start.")
		return nil
	},
}
