package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flutterprobe/probe-convert/convert"
	"github.com/flutterprobe/probe-convert/convert/appium"
	"github.com/flutterprobe/probe-convert/convert/detox"
	"github.com/flutterprobe/probe-convert/convert/gherkin"
	"github.com/flutterprobe/probe-convert/convert/maestro"
	"github.com/flutterprobe/probe-convert/convert/robot"
	"github.com/flutterprobe/probe-convert/ui"
	"github.com/spf13/cobra"
)

// registry maps formats to their converter implementations.
var registry = map[convert.Format]convert.Converter{
	convert.FormatMaestro: maestro.New(),
	convert.FormatGherkin: gherkin.New(),
	convert.FormatRobot:   robot.New(),
	convert.FormatDetox:   detox.New(),
	convert.FormatAppium:  appium.New(),
}

// knownExtensions is the set of file extensions we can process.
var knownExtensions = map[string]bool{
	".yaml": true, ".yml": true, ".feature": true, ".robot": true,
	".js": true, ".ts": true, ".py": true, ".java": true, ".kt": true,
}

func runConvert(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no input files or directories specified\n\nUsage: probe-convert [flags] <file|dir>...")
	}

	noColorFlag, _ := cmd.Flags().GetBool("no-color")
	if noColorFlag {
		ui.SetNoColor(true)
	}

	forceFmt, _ := cmd.Flags().GetString("from")
	outputDir, _ := cmd.Flags().GetString("output")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	recursive, _ := cmd.Flags().GetBool("recursive")
	verbose, _ := cmd.Flags().GetBool("verbose")
	doLint, _ := cmd.Flags().GetBool("lint")
	doVerify, _ := cmd.Flags().GetBool("verify")
	probePath, _ := cmd.Flags().GetString("probe-path")

	// Collect input files.
	files, err := collectFiles(args, recursive)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no convertible files found")
	}

	var (
		totalConverted int
		totalWarnings  int
		totalErrors    int
		outputFiles    []string // tracks written .probe files for lint/verify
	)

	spinner := ui.NewSpinner("")
	if !dryRun && len(files) > 1 {
		spinner.Start()
		defer spinner.Stop()
	}

	for i, f := range files {
		if !dryRun && len(files) > 1 {
			spinner.Update(fmt.Sprintf("[%d/%d] Converting %s...", i+1, len(files), filepath.Base(f)))
		}

		src, err := os.ReadFile(f)
		if err != nil {
			totalErrors++
			ui.PrintFileResult(filepath.Base(f), "", 0, err)
			continue
		}

		// Determine format.
		var format convert.Format
		if forceFmt != "" {
			format = convert.Format(forceFmt)
		} else {
			format, err = convert.DetectFormat(f, src)
			if err != nil {
				totalErrors++
				ui.PrintFileResult(filepath.Base(f), "", 0, err)
				continue
			}
		}

		conv, ok := registry[format]
		if !ok {
			totalErrors++
			ui.PrintFileResult(filepath.Base(f), "", 0, fmt.Errorf("no converter for format %q", format))
			continue
		}

		result, err := conv.Convert(src, f)
		if err != nil {
			totalErrors++
			ui.PrintFileResult(filepath.Base(f), "", 0, err)
			continue
		}

		warnCount := len(result.Warnings)
		totalWarnings += warnCount

		if dryRun {
			ui.PrintDryRun(filepath.Base(f), result.ProbeCode)
			if verbose {
				for _, w := range result.Warnings {
					fmt.Printf("  %s line %d: %s\n", w.Severity, w.Line, w.Message)
				}
			}
			totalConverted++
			continue
		}

		// Determine output path.
		outPath := computeOutputPath(f, outputDir)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			totalErrors++
			ui.PrintFileResult(filepath.Base(f), "", 0, err)
			continue
		}
		if err := os.WriteFile(outPath, []byte(result.ProbeCode), 0644); err != nil {
			totalErrors++
			ui.PrintFileResult(filepath.Base(f), "", 0, err)
			continue
		}

		if len(files) > 1 {
			spinner.Stop()
		}
		ui.PrintFileResult(filepath.Base(f), outPath, warnCount, nil)
		if verbose {
			for _, w := range result.Warnings {
				fmt.Printf("       %s line %d: %s\n", w.Severity, w.Line, w.Message)
			}
		}
		if len(files) > 1 {
			spinner.Start()
		}
		outputFiles = append(outputFiles, outPath)
		totalConverted++
	}

	if !dryRun && len(files) > 1 {
		spinner.Stop()
	}

	outDirDisplay := outputDir
	if outDirDisplay == "" {
		outDirDisplay = "(same as input)"
	}
	ui.PrintSummary(totalConverted, totalWarnings, totalErrors, len(files), outDirDisplay)

	// Post-conversion verification.
	if (doLint || doVerify) && len(outputFiles) > 0 && !dryRun {
		probeBin, err := findProbe(probePath)
		if err != nil {
			fmt.Printf("  %s  %s\n", ui.C(ui.Yellow, "⚠"), err)
			return nil
		}

		if doLint {
			results, err := runLint(probeBin, outputFiles)
			if err != nil {
				return fmt.Errorf("lint: %w", err)
			}
			printLintResults("Lint Verification (probe lint)", results)
			for _, r := range results {
				if !r.Passed {
					totalErrors++
				}
			}
		}

		if doVerify {
			results, err := runVerify(probeBin, outputFiles)
			if err != nil {
				return fmt.Errorf("verify: %w", err)
			}
			printLintResults("Dry-Run Verification (probe test --dry-run)", results)
			for _, r := range results {
				if !r.Passed {
					totalErrors++
				}
			}
		}
	}

	if (doLint || doVerify) && dryRun {
		fmt.Printf("  %s  --lint/--verify require writing files (incompatible with --dry-run)\n", ui.C(ui.Yellow, "⚠"))
	}

	if totalErrors > 0 {
		return fmt.Errorf("%d error(s) encountered", totalErrors)
	}
	return nil
}

func collectFiles(args []string, recursive bool) ([]string, error) {
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("cannot access %s: %w", arg, err)
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		if recursive {
			err = filepath.Walk(arg, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !fi.IsDir() && knownExtensions[strings.ToLower(filepath.Ext(path))] {
					files = append(files, path)
				}
				return nil
			})
		} else {
			entries, err2 := os.ReadDir(arg)
			if err2 != nil {
				return nil, err2
			}
			for _, e := range entries {
				if !e.IsDir() && knownExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
					files = append(files, filepath.Join(arg, e.Name()))
				}
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func computeOutputPath(inputPath, outputDir string) string {
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	name := base + ".probe"
	if outputDir != "" {
		return filepath.Join(outputDir, name)
	}
	return filepath.Join(filepath.Dir(inputPath), name)
}
