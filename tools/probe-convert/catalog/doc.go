package catalog

import (
	"fmt"
	"strings"
)

// GenerateMarkdown produces a full Markdown reference for one language.
func (l Language) GenerateMarkdown() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# %s → ProbeScript Conversion Reference\n\n", l.DisplayName))
	sb.WriteString(fmt.Sprintf("**Version:** %s  \n", l.Version))
	sb.WriteString(fmt.Sprintf("**File extensions:** %s\n\n", strings.Join(l.FileExtensions, ", ")))

	// Stats
	total, full, partial, manual, skip := l.Stats()
	coverage := 0
	if total > 0 {
		coverage = (full + partial) * 100 / total
	}
	sb.WriteString(fmt.Sprintf("**Coverage:** %d/%d constructs (%d%%) — %d full, %d partial, %d manual, %d skip\n\n",
		full+partial, total, coverage, full, partial, manual, skip))

	// Structure EBNF
	sb.WriteString("## File Structure (EBNF)\n\n")
	sb.WriteString("```ebnf\n")
	sb.WriteString(strings.TrimSpace(l.StructureEBNF))
	sb.WriteString("\n```\n\n")

	// Group by category
	cats := []Category{CatStructure, CatLifecycle, CatAppControl, CatAction, CatGesture,
		CatAssertion, CatWait, CatNavigation, CatScreenshot, CatData, CatPermission, CatFlow, CatUnsupported}
	catNames := map[Category]string{
		CatStructure: "Structure", CatLifecycle: "Lifecycle", CatAppControl: "App Control",
		CatAction: "Actions", CatGesture: "Gestures", CatAssertion: "Assertions",
		CatWait: "Wait / Timing", CatNavigation: "Navigation", CatScreenshot: "Screenshots",
		CatData: "Data / Selectors", CatPermission: "Permissions", CatFlow: "Control Flow",
		CatUnsupported: "Unsupported / Manual",
	}

	for _, cat := range cats {
		constructs := l.ByCategory(cat)
		if len(constructs) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s\n\n", catNames[cat]))
		sb.WriteString("| Source | EBNF | ProbeScript | Level |\n")
		sb.WriteString("|--------|------|-------------|-------|\n")

		for _, c := range constructs {
			ebnf := strings.ReplaceAll(c.EBNF, "|", "\\|")
			probe := c.ProbeTemplate
			if probe == "" {
				probe = "—"
			}
			level := c.Level.String()
			if c.Notes != "" {
				level += " *"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | %s |\n",
				c.Name, ebnf, probe, level))
		}
		sb.WriteString("\n")

		// Show notes for partial/manual
		for _, c := range constructs {
			if c.Notes != "" {
				sb.WriteString(fmt.Sprintf("> **%s**: %s\n\n", c.Name, c.Notes))
			}
		}
	}

	// Examples section
	sb.WriteString("## Examples\n\n")
	for _, c := range l.Constructs {
		if c.Level == Skip || c.ProbeExample == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n", c.Name))
		sb.WriteString("**Source:**\n```\n")
		sb.WriteString(c.Example)
		sb.WriteString("\n```\n\n**ProbeScript:**\n```probe\n")
		sb.WriteString(c.ProbeExample)
		sb.WriteString("\n```\n\n")
	}

	return sb.String()
}

// GenerateAllMarkdown produces a combined reference for all languages.
func GenerateAllMarkdown() string {
	var sb strings.Builder
	sb.WriteString("# probe-convert Grammar Catalog\n\n")
	sb.WriteString("This document is auto-generated from the construct catalog.\n")
	sb.WriteString("Run `go test ./catalog/ -run TestCatalogPrintSummary -v` to see coverage stats.\n\n")

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Language | Constructs | Full | Partial | Manual | Coverage |\n")
	sb.WriteString("|----------|-----------|------|---------|--------|----------|\n")
	for _, lang := range All() {
		total, full, partial, manual, _ := lang.Stats()
		coverage := 0
		if total > 0 {
			coverage = (full + partial) * 100 / total
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %d%% |\n",
			lang.DisplayName, total, full, partial, manual, coverage))
	}
	sb.WriteString("\n---\n\n")

	for _, lang := range All() {
		sb.WriteString(lang.GenerateMarkdown())
		sb.WriteString("\n---\n\n")
	}

	return sb.String()
}
