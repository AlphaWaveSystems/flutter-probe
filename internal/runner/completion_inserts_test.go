package runner

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// The VS Code completion list (vscode/src/providers/probescriptKeywords.generated.ts) gives each command form the text it inserts. A completion
// that inserts something that parses as another step is worse than none: `see filled` is a plain `see` of the text "filled", not a state check.
// So every insert of a `see <state>` form, with each tab stop left as its default, must parse to the assertion that form names.
const completionsPath = "../../vscode/src/providers/probescriptKeywords.generated.ts"

var completionEntry = regexp.MustCompile(`\{ label: ("(?:[^"\\]|\\.)*"), insert: ("(?:[^"\\]|\\.)*") \}`)

func TestCompletionInsertsOfStateChecksParseToTheStateCheck(t *testing.T) {
	raw, err := os.ReadFile(completionsPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]parser.StateCheck{
		"see enabled":  parser.StateEnabled,
		"see disabled": parser.StateDisabled,
		"see checked":  parser.StateChecked,
		"see focused":  parser.StateFocused,
		"see filled":   parser.StateFilled,
		"see empty":    parser.StateEmpty,
		"see contains": parser.StateContains,
	}
	seen := map[string]bool{}
	for _, m := range completionEntry.FindAllStringSubmatch(string(raw), -1) {
		var label, insert string
		if err := json.Unmarshal([]byte(m[1]), &label); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(m[2]), &insert); err != nil {
			t.Fatal(err)
		}
		state, ok := want[label]
		if !ok {
			continue
		}
		seen[label] = true
		src := skeletonHeader + snippetDefaults(insert) + "\n"
		prog, err := parser.ParseFile(src)
		if err != nil {
			t.Errorf("completion %q inserts %q, which does not parse: %v", label, insert, err)
			continue
		}
		a, ok := prog.Tests[0].Body[0].(parser.AssertStep)
		if !ok || a.Check != state {
			t.Errorf("completion %q inserts %q, which parses as %+v, not as a see with the state check %v (%s)", label, insert, prog.Tests[0].Body[0], state, strings.TrimSpace(src))
		}
	}
	for label := range want {
		if !seen[label] {
			t.Errorf("the completion list has no entry %q: the test cannot check it", label)
		}
	}
}
