package cli

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// What `probe init` scaffolds is the first thing a new user runs: it must parse, or the first `probe test` fails.
func TestInitScaffoldParses(t *testing.T) {
	for name, src := range map[string]string{"tests/example.probe": sampleTestFile, "tests/recipes/login.probe": sampleRecipeFile} {
		if _, err := parser.ParseFile(src); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
		}
	}
}
