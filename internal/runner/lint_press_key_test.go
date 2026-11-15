package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// TestLintSuggestsAKeyForAPressTypo checks the lint for `press <word>` that calls no recipe: a word within two edits of a key
// draws a warning that names the keys and the recipe the line would need. A word far from every key, and a recipe defined
// under that name, draw no such warning.
func TestLintSuggestsAKeyForAPressTypo(t *testing.T) {
	const definedRecipe = "recipe \"press tba\"\n  tap \"x\"\n\n"
	for _, tc := range []struct {
		name string
		src  string
		warn bool
	}{
		{"typo near tab", "test \"t\"\n  press tba\n", true},
		{"typo near backspace", "test \"t\"\n  press backspce\n", true},
		{"word far from every key", "test \"t\"\n  press home\n", false},
		{"recipe with that name", definedRecipe + "test \"t\"\n  press tba\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "press.probe")
			if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			findings, err := LintReferences(&config.Config{}, path)
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, f := range findings {
				if f.Warning && strings.Contains(f.Message, "unknown key") {
					got = true
					if !strings.Contains(f.Message, "keys: tab, escape, backspace, space, up, down, left, right") ||
						!strings.Contains(f.Message, "or define a recipe named 'press") {
						t.Errorf("warning %q does not name the keys and the recipe", f.Message)
					}
				}
			}
			if got != tc.warn {
				t.Errorf("warning = %v, want %v; findings %+v", got, tc.warn, findings)
			}
		})
	}
}
