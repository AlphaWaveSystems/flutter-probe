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

// yamlFile is a discovered Maestro flow file paired with its directory
// relative to whichever search root it was found under, so migration
// output can mirror the source layout instead of flattening everything
// into one directory.
type yamlFile struct {
	path   string
	relDir string
}

// discoverYAMLFiles resolves the migrate command's [dir|file]... arguments
// into concrete YAML files. Real Maestro suites commonly organize flows
// into subdirectories (flows/auth/, flows/settings/, etc. — nect-flutter's
// own real 76-flow suite is laid out exactly this way) — a single-level
// os.ReadDir here used to silently find nothing at all for any project
// organized that way, rather than erroring or partially converting. Walks
// recursively instead, and records each file's directory relative to
// whichever search root it was found under, so migration output can mirror
// the source layout and files sharing a base name in different
// subdirectories (e.g. two "login.yaml" under different features) don't
// collide and overwrite each other in a flat output directory.
func discoverYAMLFiles(args []string) ([]yamlFile, error) {
	var yamlFiles []yamlFile
	for _, path := range args {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		if !info.IsDir() {
			yamlFiles = append(yamlFiles, yamlFile{path: path, relDir: "."})
			continue
		}
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
				return nil
			}
			relDir, relErr := filepath.Rel(path, filepath.Dir(p))
			if relErr != nil {
				relDir = "."
			}
			yamlFiles = append(yamlFiles, yamlFile{path: p, relDir: relDir})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return yamlFiles, nil
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

		yamlFiles, err := discoverYAMLFiles(args)
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
				base := strings.TrimSuffix(filepath.Base(yf.path), filepath.Ext(yf.path))
				outPath = filepath.Join(outputDir, yf.relDir, base+".probe")
			}

			result, err := migrate.ConvertFile(yf.path, outPath)
			if err != nil {
				fmt.Printf("  \033[31m✗\033[0m  %s — %s\n", filepath.Base(yf.path), err)
				continue
			}
			statusOK(os.Stdout, "%s → %s", filepath.Base(yf.path), result)
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
