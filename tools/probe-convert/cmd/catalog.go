package cmd

import (
	"fmt"
	"strings"

	"github.com/flutterprobe/probe-convert/catalog"
	"github.com/flutterprobe/probe-convert/ui"
	"github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog [language]",
	Short: "Show the grammar construct catalog for supported languages",
	Long: `Display the formal EBNF grammar and construct mappings for each source language.
This catalog is the single source of truth for what each converter recognizes.

Without arguments, prints a summary table. With a language name, prints the full
construct catalog including EBNF rules and ProbeScript mappings.

Languages: maestro, gherkin, robot, detox, appium_python, appium_java, appium_js`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		markdown, _ := cmd.Flags().GetBool("markdown")

		if len(args) == 0 {
			if markdown {
				fmt.Print(catalog.GenerateAllMarkdown())
				return nil
			}
			printCatalogSummary()
			return nil
		}

		name := args[0]
		// Allow short aliases.
		aliases := map[string]string{
			"python": "appium_python", "py": "appium_python",
			"java": "appium_java", "kotlin": "appium_java", "kt": "appium_java",
			"js": "appium_js", "wdio": "appium_js", "webdriverio": "appium_js",
		}
		if alias, ok := aliases[name]; ok {
			name = alias
		}

		lang, ok := catalog.ByName(name)
		if !ok {
			return fmt.Errorf("unknown language %q — available: maestro, gherkin, robot, detox, appium_python, appium_java, appium_js", name)
		}

		if markdown {
			fmt.Print(lang.GenerateMarkdown())
			return nil
		}

		printLanguageCatalog(lang)
		return nil
	},
}

func init() {
	catalogCmd.Flags().Bool("markdown", false, "output in Markdown format")
	rootCmd.AddCommand(catalogCmd)
}

func printCatalogSummary() {
	fmt.Println()
	fmt.Printf("  %s\n", ui.C(ui.Bold, "Grammar Construct Catalog"))
	fmt.Println("  " + strings.Repeat("─", 70))
	fmt.Printf("  %-27s %6s  %5s  %7s  %6s  %8s\n",
		"Language", "Total", "Full", "Partial", "Manual", "Coverage")
	fmt.Println("  " + strings.Repeat("─", 70))

	for _, lang := range catalog.All() {
		total, full, partial, manual, _ := lang.Stats()
		coverage := 0
		if total > 0 {
			coverage = (full + partial) * 100 / total
		}
		covStr := fmt.Sprintf("%d%%", coverage)
		if coverage == 100 {
			covStr = ui.C(ui.Green, covStr)
		} else if coverage >= 80 {
			covStr = ui.C(ui.Yellow, covStr)
		} else {
			covStr = ui.C(ui.Red, covStr)
		}
		fmt.Printf("  %-27s %4d    %3d    %5d    %4d    %s\n",
			lang.DisplayName, total, full, partial, manual, covStr)
	}
	fmt.Println("  " + strings.Repeat("─", 70))
	fmt.Println()
	fmt.Println("  Use 'probe-convert catalog <language>' for full construct details.")
	fmt.Println("  Use 'probe-convert catalog --markdown' to generate reference docs.")
	fmt.Println()
}

func printLanguageCatalog(lang catalog.Language) {
	fmt.Println()
	fmt.Printf("  %s\n", ui.C(ui.Bold, lang.DisplayName+" — Construct Catalog"))
	total, full, partial, manual, skip := lang.Stats()
	coverage := 0
	if total > 0 {
		coverage = (full + partial) * 100 / total
	}
	fmt.Printf("  Version: %s  |  %d constructs  |  %d%% coverage\n", lang.Version, total, coverage)
	fmt.Println()

	// Structure EBNF
	fmt.Printf("  %s\n", ui.C(ui.Bold, "File Structure (EBNF)"))
	for _, line := range strings.Split(strings.TrimSpace(lang.StructureEBNF), "\n") {
		fmt.Printf("    %s\n", ui.C(ui.Dim, line))
	}
	fmt.Println()

	// Group by category
	cats := []catalog.Category{
		catalog.CatStructure, catalog.CatLifecycle, catalog.CatAppControl,
		catalog.CatAction, catalog.CatGesture, catalog.CatAssertion,
		catalog.CatWait, catalog.CatNavigation, catalog.CatScreenshot,
		catalog.CatData, catalog.CatPermission, catalog.CatFlow, catalog.CatUnsupported,
	}
	catNames := map[catalog.Category]string{
		catalog.CatStructure: "Structure", catalog.CatLifecycle: "Lifecycle",
		catalog.CatAppControl: "App Control", catalog.CatAction: "Actions",
		catalog.CatGesture: "Gestures", catalog.CatAssertion: "Assertions",
		catalog.CatWait: "Wait/Timing", catalog.CatNavigation: "Navigation",
		catalog.CatScreenshot: "Screenshots", catalog.CatData: "Data/Selectors",
		catalog.CatPermission: "Permissions", catalog.CatFlow: "Control Flow",
		catalog.CatUnsupported: "Unsupported",
	}

	for _, cat := range cats {
		constructs := lang.ByCategory(cat)
		if len(constructs) == 0 {
			continue
		}

		fmt.Printf("  %s\n", ui.C(ui.Bold, catNames[cat]))
		for _, c := range constructs {
			levelColor := ui.Green
			switch c.Level {
			case catalog.Partial:
				levelColor = ui.Yellow
			case catalog.Manual:
				levelColor = ui.Red
			}
			lvl := ui.C(levelColor, fmt.Sprintf("[%s]", c.Level))
			fmt.Printf("    %-35s → %-30s %s\n", c.Name, c.ProbeTemplate, lvl)
		}
		fmt.Println()
	}

	_ = full
	_ = partial
	_ = manual
	_ = skip
}
