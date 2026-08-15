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

		yamlFiles, err := migrate.DiscoverYAMLFiles(args)
		if err != nil {
			return err
		}

		if len(yamlFiles) == 0 {
			fmt.Println("No Maestro YAML files found.")
			return nil
		}

		converted := 0
		for _, yf := range yamlFiles {
			outPath := ""
			if outputDir != "" {
				base := strings.TrimSuffix(filepath.Base(yf.Path), filepath.Ext(yf.Path))
				outPath = filepath.Join(outputDir, yf.RelDir, base+".probe")
			}

			result, err := migrate.ConvertFile(yf.Path, outPath)
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — %s\n", filepath.Base(yf.Path), err)
				continue
			}
			statusOK(os.Stdout, "%s → %s", filepath.Base(yf.Path), result)
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
