package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/migrate"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate test files from other frameworks to ProbeScript",
}

var migrateMaestroCmd = &cobra.Command{
	Use:   "maestro [dir|file]...",
	Short: "Convert Maestro YAML flows to ProbeScript .probe files",
	Example: `  probe migrate maestro tests/maestro/
  probe migrate maestro flows/login.yaml --output tests/probe/
  probe migrate maestro .maestro/ --output tests/`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outputDir, _ := cmd.Flags().GetString("output")

		if len(args) == 0 {
			args = []string{".maestro"}
		}

		// Collect YAML files
		var yamlFiles []string
		for _, path := range args {
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			if info.IsDir() {
				entries, _ := os.ReadDir(path)
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
						yamlFiles = append(yamlFiles, filepath.Join(path, e.Name()))
					}
				}
			} else {
				yamlFiles = append(yamlFiles, path)
			}
		}

		if len(yamlFiles) == 0 {
			fmt.Println("No Maestro YAML files found.")
			return nil
		}

		converted := 0
		for _, yf := range yamlFiles {
			outPath := ""
			if outputDir != "" {
				base := strings.TrimSuffix(filepath.Base(yf), filepath.Ext(yf))
				outPath = filepath.Join(outputDir, base+".probe")
			}

			result, err := migrate.ConvertFile(yf, outPath)
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — %s\n", filepath.Base(yf), err)
				continue
			}
			fmt.Printf("  \033[32m✓\033[0m  %s → %s\n", filepath.Base(yf), result)
			converted++
		}

		fmt.Printf("\n  Converted %d/%d file(s)\n", converted, len(yamlFiles))

		// Optionally lint the output
		if outputDir != "" && converted > 0 {
			fmt.Println("\n  Linting generated .probe files...")
			probeFiles, _ := runner.CollectFiles([]string{outputDir})
			for _, f := range probeFiles {
				fmt.Printf("  → %s\n", f)
			}
		}
		return nil
	},
}

func init() {
	migrateMaestroCmd.Flags().StringP("output", "o", "", "output directory for .probe files")
	migrateCmd.AddCommand(migrateMaestroCmd)
	rootCmd.AddCommand(migrateCmd)
}
