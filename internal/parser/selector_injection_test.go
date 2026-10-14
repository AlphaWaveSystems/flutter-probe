package parser_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

// The lines in testdata/selector-injection.json are what the device view
// inserts for a selected element (VS Code extension and Studio use the same
// rule). Widget text comes from the device screen, so each generated line must
// parse as exactly one action step, whatever the text contains. The TS tests
// assert the same table against the TypeScript implementations.
func TestSelectorInjectionFixtureParsesAsOneStep(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/selector-injection.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string  `json:"name"`
		Line *string `json:"line"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 10 {
		t.Fatalf("fixture too small: %d cases", len(cases))
	}
	checked := 0
	for _, c := range cases {
		if c.Line == nil {
			continue // refused by the extension, nothing is inserted
		}
		checked++
		t.Run(c.Name, func(t *testing.T) {
			if strings.ContainsAny(*c.Line, "\n\r  ") {
				t.Fatalf("line contains a line break: %q", *c.Line)
			}
			prog, err := parser.ParseFile("test \"t\"\n  " + *c.Line + "\n")
			if err != nil {
				t.Fatalf("parse error for %q: %v", *c.Line, err)
			}
			if len(prog.Tests) != 1 || len(prog.Tests[0].Body) != 1 {
				t.Fatalf("want one test with one step for %q, got %+v", *c.Line, prog.Tests)
			}
			a, ok := prog.Tests[0].Body[0].(parser.ActionStep)
			if !ok {
				t.Fatalf("step is %T, want ActionStep, for %q", prog.Tests[0].Body[0], *c.Line)
			}
			if a.Verb != parser.VerbTap {
				t.Fatalf("verb %v, want tap, for %q", a.Verb, *c.Line)
			}
		})
	}
	if checked < 10 {
		t.Fatalf("only %d lines checked", checked)
	}
}
