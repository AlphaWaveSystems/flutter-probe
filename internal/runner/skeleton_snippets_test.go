package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// The VS Code extension's generated skeleton snippets and where each one stands in a file (vscode/scripts/gen-snippets.mjs writes
// both). The gate lints every snippet, so a snippet that does not parse cannot ship.
const (
	skeletonSnippetsPath = "../../vscode/snippets/probescript.generated.json"
	skeletonContextsPath = "../../vscode/snippets/probescript.generated.contexts.json"
	fileTestStep         = "open the app"
)

// tabStop matches a VS Code tab stop with its default text, `${1:text}`, where a backslash escapes `\`, `$` and `}` in the text.
var (
	tabStop        = regexp.MustCompile(`\$\{\d+:((?:\\[\\$}]|[^}\\])*)\}|\\([\\$}])`)
	escapedInStop  = regexp.MustCompile(`\\([\\$}])`)
	skeletonHeader = `test "skeleton"` + "\n  "
)

// snippetDefaults returns the snippet as VS Code inserts it when the user leaves each tab stop as it is.
func snippetDefaults(body string) string {
	return tabStop.ReplaceAllStringFunc(body, func(m string) string {
		sub := tabStop.FindStringSubmatch(m)
		if sub[1] == "" && sub[2] != "" {
			return sub[2]
		}
		return escapedInStop.ReplaceAllString(sub[1], "$1")
	})
}

// skeletonSource returns the probe source a skeleton is linted as, for its context: a step inside a test, a file-level line, a
// file-level line after a test, or a line inside a composite test. The context is what the snippet is meant to stand in.
func skeletonSource(context, line string) (string, error) {
	switch context {
	case "test":
		return skeletonHeader + line + "\n", nil
	case "top":
		return line + "\n", nil
	case "file":
		return skeletonHeader + fileTestStep + "\n\n" + line + "\n", nil
	case "composite":
		return `composite test "skeleton"` + "\n  " + line + "\n", nil
	}
	return "", fmt.Errorf("unknown context %q", context)
}

// TestSkeletonSnippetsParse expands every generated skeleton snippet with its tab stops at their defaults, places it in its
// context, and lints it with the check `probe lint` runs: the parser, then the step and recipe references. Warnings are not
// failures; an error is.
func TestSkeletonSnippetsParse(t *testing.T) {
	raw, err := os.ReadFile(skeletonSnippetsPath)
	if err != nil {
		t.Fatalf("read %s: %v", skeletonSnippetsPath, err)
	}
	var snippets map[string]struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(raw, &snippets); err != nil {
		t.Fatalf("parse %s: %v", skeletonSnippetsPath, err)
	}
	rawContexts, err := os.ReadFile(skeletonContextsPath)
	if err != nil {
		t.Fatalf("read %s: %v", skeletonContextsPath, err)
	}
	var contexts map[string]string
	if err := json.Unmarshal(rawContexts, &contexts); err != nil {
		t.Fatalf("parse %s: %v", skeletonContextsPath, err)
	}
	if len(snippets) == 0 {
		t.Fatalf("%s has no snippets", skeletonSnippetsPath)
	}
	names := make([]string, 0, len(snippets))
	for name := range snippets {
		names = append(names, name)
	}
	sort.Strings(names)
	var broken []string
	for _, name := range names {
		context, ok := contexts[name]
		if !ok {
			broken = append(broken, name+": no context in "+skeletonContextsPath)
			continue
		}
		src, err := skeletonSource(context, snippetDefaults(snippets[name].Body))
		if err != nil {
			broken = append(broken, name+": "+err.Error())
			continue
		}
		path := filepath.Join(t.TempDir(), "skeleton.probe")
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		findings, err := LintReferences(&config.Config{}, path)
		if err != nil {
			broken = append(broken, fmt.Sprintf("%s [%s]: %q -> %v", name, context, src, err))
			continue
		}
		for _, f := range findings {
			if !f.Warning {
				broken = append(broken, fmt.Sprintf("%s [%s]: %q -> %s", name, context, src, f.Message))
			}
		}
	}
	if len(broken) > 0 {
		t.Fatalf("%d skeleton snippets do not parse in their context:\n%s", len(broken), strings.Join(broken, "\n"))
	}
}
