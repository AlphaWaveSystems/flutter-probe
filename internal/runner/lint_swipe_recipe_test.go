package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// TestLintSwipeWordNamingARecipeIsNotAWarning checks that a word after swipe which names a recipe draws no direction warning:
// the line is a recipe call, so calling it a mistyped direction would be wrong. Without the recipe the warning stays.
func TestLintSwipeWordNamingARecipeIsNotAWarning(t *testing.T) {
	const recipe = "recipe \"sideways\"\n  tap \"x\"\n\n"
	for _, tc := range []struct {
		name string
		src  string
		warn bool
	}{
		{"no recipe named like the word", "test \"t\"\n  swipe sideways\n", true},
		{"recipe named like the word", recipe + "test \"t\"\n  swipe sideways\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "swipe_recipe.probe")
			if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			findings, err := LintReferences(&config.Config{}, path)
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, f := range findings {
				if f.Warning && strings.HasPrefix(f.Message, "swipe: ") {
					got = true
				}
			}
			if got != tc.warn {
				t.Errorf("direction warning = %v, want %v; findings %+v", got, tc.warn, findings)
			}
		})
	}
}
