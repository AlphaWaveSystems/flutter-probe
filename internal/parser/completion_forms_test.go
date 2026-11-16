package parser_test

import (
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// TestCompletionForms pins the forms the VS Code completion list offers beyond what dictionary.md lists (vscode/scripts/
// extra-forms.json): the parser accepts each of them.
func TestCompletionForms(t *testing.T) {
	for _, src := range []string{`tap on "Sign In"`, `toggle wifi`} {
		if _, err := parser.ParseFile(testBody(src)); err != nil {
			t.Errorf("%q: %v; the completion list offers it, so the parser must accept it", src, err)
		}
	}
}
