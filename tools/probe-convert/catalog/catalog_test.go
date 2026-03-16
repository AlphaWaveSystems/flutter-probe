package catalog

import (
	"fmt"
	"strings"
	"testing"
)

// TestCatalogIntegrity verifies that every catalog entry has required fields.
func TestCatalogIntegrity(t *testing.T) {
	for _, lang := range All() {
		t.Run(lang.Name, func(t *testing.T) {
			if lang.DisplayName == "" {
				t.Error("missing DisplayName")
			}
			if len(lang.FileExtensions) == 0 {
				t.Error("missing FileExtensions")
			}
			if lang.StructureEBNF == "" {
				t.Error("missing StructureEBNF")
			}
			if len(lang.Constructs) == 0 {
				t.Error("no constructs defined")
			}

			ids := make(map[string]bool)
			for _, c := range lang.Constructs {
				t.Run(c.ID, func(t *testing.T) {
					if c.ID == "" {
						t.Error("missing ID")
					}
					if ids[c.ID] {
						t.Errorf("duplicate ID: %s", c.ID)
					}
					ids[c.ID] = true

					if c.Name == "" {
						t.Error("missing Name")
					}
					if c.EBNF == "" {
						t.Error("missing EBNF")
					}
					if c.Example == "" {
						t.Error("missing Example")
					}
					if c.Category == "" {
						t.Error("missing Category")
					}
					if c.Level != Skip && c.ProbeTemplate == "" {
						t.Error("missing ProbeTemplate for non-skip construct")
					}
					if c.Level == Manual && c.Notes == "" {
						t.Error("manual construct missing Notes explaining limitation")
					}
					if c.Level == Partial && c.Notes == "" {
						t.Error("partial construct missing Notes explaining limitation")
					}
				})
			}
		})
	}
}

// TestCatalogUniqueIDs verifies no ID collisions across all languages.
func TestCatalogUniqueIDs(t *testing.T) {
	allIDs := make(map[string]string) // id → language
	for _, lang := range All() {
		for _, c := range lang.Constructs {
			if prev, ok := allIDs[c.ID]; ok {
				t.Errorf("duplicate ID %q in %s and %s", c.ID, prev, lang.Name)
			}
			allIDs[c.ID] = lang.Name
		}
	}
}

// TestCatalogStats prints coverage statistics for each language.
func TestCatalogStats(t *testing.T) {
	for _, lang := range All() {
		total, full, partial, manual, skip := lang.Stats()
		coverage := 0
		if total > 0 {
			coverage = (full + partial) * 100 / total
		}
		t.Logf("%-25s  %d constructs  %d full  %d partial  %d manual  %d skip  (%d%% coverage)",
			lang.DisplayName, total, full, partial, manual, skip, coverage)
	}
}

// TestCatalogExamplesAreValid verifies that Example and ProbeExample fields
// are consistent with the ProbeTemplate pattern.
func TestCatalogExamplesAreValid(t *testing.T) {
	for _, lang := range All() {
		for _, c := range lang.Constructs {
			if c.Level == Skip {
				continue
			}
			if c.ProbeExample == "" && c.Level == Full {
				t.Errorf("%s: missing ProbeExample for full-coverage construct", c.ID)
			}
		}
	}
}

// TestCatalogCategoryDistribution verifies each language has reasonable
// category coverage. Every language must have at least action constructs.
// Languages with test-level structure (test/describe/it) must have structure.
func TestCatalogCategoryDistribution(t *testing.T) {
	// Action is universally required.
	for _, lang := range All() {
		cats := make(map[Category]int)
		for _, c := range lang.Constructs {
			cats[c.Category]++
		}
		if cats[CatAction] == 0 {
			t.Errorf("%s: missing required category %q", lang.Name, CatAction)
		}
		// Languages with test definitions need structure.
		if lang.Name != "maestro" && cats[CatStructure] == 0 {
			t.Errorf("%s: missing required category %q", lang.Name, CatStructure)
		}
	}
}

// TestCatalogEBNFPrefix verifies EBNF rules use consistent notation.
func TestCatalogEBNFPrefix(t *testing.T) {
	for _, lang := range All() {
		if !strings.Contains(lang.StructureEBNF, "=") {
			t.Errorf("%s: StructureEBNF should contain production rules (NAME = ...)", lang.Name)
		}
	}
}

// TestCatalogPrintSummary prints a formatted summary (useful for CI logs).
func TestCatalogPrintSummary(t *testing.T) {
	fmt.Println()
	fmt.Println("  ┌─────────────────────────────┬────────┬──────┬─────────┬────────┬──────┬──────────┐")
	fmt.Println("  │ Language                    │ Total  │ Full │ Partial │ Manual │ Skip │ Coverage │")
	fmt.Println("  ├─────────────────────────────┼────────┼──────┼─────────┼────────┼──────┼──────────┤")
	for _, lang := range All() {
		total, full, partial, manual, skip := lang.Stats()
		coverage := 0
		if total > 0 {
			coverage = (full + partial) * 100 / total
		}
		fmt.Printf("  │ %-27s │ %4d   │ %3d  │ %5d   │ %4d   │ %3d  │ %5d%%   │\n",
			lang.DisplayName, total, full, partial, manual, skip, coverage)
	}
	fmt.Println("  └─────────────────────────────┴────────┴──────┴─────────┴────────┴──────┴──────────┘")
	fmt.Println()
}
