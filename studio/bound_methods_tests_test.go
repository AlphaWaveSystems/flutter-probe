package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Every bound method is a decision about what a page script may do (bound_methods_test.go), and a decision that nothing tests is a claim nobody checks.
// So each entry of bound_methods.json names the Go tests that call the method, and this test checks the names: each test exists and calls a method of
// that name. A method with no test must say why (noTests) and only a pure read may: a method that writes, runs, asks for a key or reaches a device
// always has one. A test that calls another type's method of the same name would pass this check; the check proves a test mentions the call, not
// that the test is good, which is what the mutant of each test is for.

// testCalls maps each Test function of the package to the names of the methods it calls through a selector (x.Name(...)).
func testCalls(t *testing.T) map[string]map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files: %v", err)
	}
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, decl := range af.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil {
				continue
			}
			calls := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if ce, ok := n.(*ast.CallExpr); ok {
					if se, ok := ce.Fun.(*ast.SelectorExpr); ok {
						calls[se.Sel.Name] = true
					}
				}
				return true
			})
			out[fn.Name.Name] = calls
		}
	}
	return out
}

// pureCategories are the categories whose methods may have no test, with a reason: they read, or hold, and change nothing.
var pureCategories = map[string]bool{catUI: true, catReadsWS: true}

// boundTestsProblem says what is wrong with an entry's tests ("" when nothing is).
func boundTestsProblem(d boundDecision, calls map[string]map[string]bool) string {
	switch {
	case len(d.Tests) == 0 && d.NoTests == "":
		return "no test is named and no reason is given"
	case len(d.Tests) > 0 && d.NoTests != "":
		return "tests are named and so is a reason for having none"
	case len(d.Tests) == 0 && !pureCategories[d.Category]:
		return fmt.Sprintf("a %s method always has a test: only a pure read may go without", d.Category)
	}
	for _, name := range d.Tests {
		c, ok := calls[name]
		switch {
		case !ok:
			return fmt.Sprintf("%s is not a test of this package", name)
		case !c[d.Method]:
			return fmt.Sprintf("%s does not call %s", name, d.Method)
		}
	}
	return ""
}

func TestEveryBoundMethodNamesATestThatCallsIt(t *testing.T) {
	calls := testCalls(t)
	var bad []string
	for _, d := range boundDecisions {
		if why := boundTestsProblem(d, calls); why != "" {
			bad = append(bad, d.Method+": "+why)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("%d of %d bound methods have a problem with their tests:\n  %s", len(bad), len(boundDecisions), strings.Join(bad, "\n  "))
	}
	if len(boundDecisions) < 60 {
		t.Errorf("only %d bound methods were read: the table did not load", len(boundDecisions))
	}
}

func TestBoundTestsProblemRules(t *testing.T) {
	calls := map[string]map[string]bool{"TestA": {"Save": true}, "TestB": {"Other": true}}
	for name, c := range map[string]struct {
		d    boundDecision
		want string
	}{
		"a test that calls it":             {boundDecision{Method: "Save", Category: catWritesWS, Tests: []string{"TestA"}}, ""},
		"nothing at all":                   {boundDecision{Method: "Save", Category: catWritesWS}, "no test is named"},
		"a test that is not there":         {boundDecision{Method: "Save", Category: catWritesWS, Tests: []string{"TestGone"}}, "not a test of this package"},
		"a test that calls something else": {boundDecision{Method: "Save", Category: catWritesWS, Tests: []string{"TestB"}}, "does not call Save"},
		"one good and one that does not":   {boundDecision{Method: "Save", Category: catWritesWS, Tests: []string{"TestA", "TestB"}}, "does not call Save"},
		"a reason for a pure read":         {boundDecision{Method: "Get", Category: catUI, NoTests: "returns a constant"}, ""},
		"a reason for a workspace read":    {boundDecision{Method: "Get", Category: catReadsWS, NoTests: "one line"}, ""},
		"a reason for a write":             {boundDecision{Method: "Save", Category: catWritesWS, NoTests: "trust me"}, "always has a test"},
		"a reason for a run":               {boundDecision{Method: "Run", Category: catRunControl, NoTests: "trust me"}, "always has a test"},
		"a reason for a secret":            {boundDecision{Method: "Key", Category: catSecrets, NoTests: "trust me"}, "always has a test"},
		"a reason for a device call":       {boundDecision{Method: "Shot", Category: catDevice, NoTests: "trust me"}, "always has a test"},
		"tests and a reason":               {boundDecision{Method: "Save", Category: catUI, Tests: []string{"TestA"}, NoTests: "x"}, "so is a reason"},
	} {
		got := boundTestsProblem(c.d, calls)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}
