package runner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The structural tests of the taint design: a new verb or step must be classified, the sinks must take their strings through the guard, the literal-only
// handlers must stay literal, and every write to a variable must go through setVar.

func parsePkgFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[n] = f
	}
	return files
}

// verbsAndStepsOfTheParser lists the ActionVerb constants and the Step types declared in internal/parser/ast.go.
func verbsAndStepsOfTheParser(t *testing.T) (verbs, steps []string) {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "parser", "ast.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		switch g := d.(type) {
		case *ast.GenDecl:
			for _, spec := range g.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if id, ok := vs.Type.(*ast.Ident); ok && id.Name == "ActionVerb" {
					for _, n := range vs.Names {
						verbs = append(verbs, n.Name)
					}
				}
			}
		case *ast.FuncDecl:
			if g.Recv != nil && g.Name.Name == "stepType" && len(g.Recv.List) == 1 {
				switch r := g.Recv.List[0].Type.(type) {
				case *ast.Ident:
					steps = append(steps, r.Name)
				case *ast.StarExpr:
					if id, ok := r.X.(*ast.Ident); ok {
						steps = append(steps, id.Name)
					}
				}
			}
		}
	}
	sort.Strings(verbs)
	sort.Strings(steps)
	return
}

// 1. The classification is complete: nothing is missing, nothing is stale, and (the map having one value per name) nothing is in two classes.
func TestEveryStepIsClassifiedForTaint(t *testing.T) {
	verbs, steps := verbsAndStepsOfTheParser(t)
	if len(verbs) < 40 || len(steps) < 15 {
		t.Fatalf("the scan of ast.go found %d verbs and %d step types; it is broken", len(verbs), len(steps))
	}
	want := map[string]bool{}
	for _, v := range verbs {
		if v != "Verb" { // a constant named only Verb is not in this tree; kept as a guard against a changed pattern
			want[v] = true
		}
	}
	for _, s := range steps {
		if s != "ActionStep" { // an ActionStep is classified through its verb
			want[s] = true
		}
	}
	for name := range want {
		if _, ok := taintClasses[name]; !ok {
			t.Errorf("%s is not classified in taint_classes.go: decide whether it is a sink, literal only, UI only, propagating or takes no string", name)
		}
	}
	for name := range taintClasses {
		if !want[name] {
			t.Errorf("%s is classified in taint_classes.go but is not a verb or step type of the parser", name)
		}
	}
}

func callsIn(n ast.Node) map[string]int {
	calls := map[string]int{}
	ast.Inspect(n, func(x ast.Node) bool {
		if c, ok := x.(*ast.CallExpr); ok {
			switch f := c.Fun.(type) {
			case *ast.SelectorExpr:
				calls[f.Sel.Name]++
			case *ast.Ident:
				calls[f.Name]++
			}
		}
		return true
	})
	return calls
}

// bodyOf finds the code of a handler: the whole function, or the case of runAction for a verb.
func bodyOf(t *testing.T, files map[string]*ast.File, name, fn string) ast.Node {
	t.Helper()
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name != fn {
				continue
			}
			if fn != "runAction" {
				return fd
			}
			var found ast.Node
			ast.Inspect(fd, func(x ast.Node) bool {
				cc, ok := x.(*ast.CaseClause)
				if !ok || found != nil {
					return true
				}
				for _, ex := range cc.List {
					if sel, ok := ex.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
						found = cc
					}
				}
				return true
			})
			return found
		}
	}
	return nil
}

var plainResolvers = []string{"resolve", "resolveVars", "resolveSelector", "expandEnv", "expandVars", "expandT"}

// allowedInSink lists the plain resolvers a sink handler may also call, with the reason: `compare screenshot ... of <selector>` resolves the selector it
// crops to, which is matched on the screen (the name of the screenshot, the sink part, goes through shotName and forSink).
var allowedInSink = map[string]string{"VerbCompareShot": "resolveSelector"}

// 2. The sinks take their strings through the guard and never call a plain resolver.
func TestSinkHandlersTakeTheirStringsThroughTheGuard(t *testing.T) {
	files := parsePkgFiles(t)
	for name, c := range taintClasses {
		if c != classSink {
			continue
		}
		h, ok := sinkHandlers[name]
		if !ok {
			t.Errorf("%s is a sink but has no entry in sinkHandlers", name)
			continue
		}
		body := bodyOf(t, files, name, h.fn)
		if body == nil {
			t.Errorf("%s: the handler %s was not found", name, h.fn)
			continue
		}
		calls := callsIn(body)
		if calls["forSink"] == 0 && calls["shotName"] == 0 {
			t.Errorf("%s: its handler does not take strings through forSink", name)
		}
		for _, r := range plainResolvers {
			if calls[r] > 0 && allowedInSink[name] != r {
				t.Errorf("%s: its handler calls %s directly; a sink must use forSink", name, r)
			}
		}
	}
	// shotName is the shared helper of the two screenshot sinks: it must use the guard itself
	sn := bodyOf(t, files, "", "shotName")
	if sn == nil || callsIn(sn)["forSink"] == 0 {
		t.Errorf("shotName does not use forSink")
	}
	for name := range sinkHandlers {
		if taintClasses[name] != classSink {
			t.Errorf("%s has a sinkHandlers entry but is not classified as a sink", name)
		}
	}
}

// 3. The literal-only handlers stay literal.
func TestLiteralOnlyHandlersExpandNothing(t *testing.T) {
	files := parsePkgFiles(t)
	for name, c := range taintClasses {
		if c != classLiteral {
			continue
		}
		h, ok := literalHandlers[name]
		if !ok {
			t.Errorf("%s is literal only but has no entry in literalHandlers", name)
			continue
		}
		body := bodyOf(t, files, name, h.fn)
		if body == nil {
			t.Errorf("%s: the handler %s was not found", name, h.fn)
			continue
		}
		calls := callsIn(body)
		for _, r := range append([]string{"forSink", "resolveT"}, plainResolvers...) {
			if calls[r] > 0 {
				t.Errorf("%s: a literal-only handler calls %s; if it now expands variables it must become a sink (taint_classes.go) and use forSink", name, r)
			}
		}
	}
	for name := range literalHandlers {
		if taintClasses[name] != classLiteral {
			t.Errorf("%s has a literalHandlers entry but is not classified as literal only", name)
		}
	}
}

// 4. Every write to the variable map goes through setVar, which keeps the taint flag right.
func TestEveryVariableWriteGoesThroughSetVar(t *testing.T) {
	for file, f := range parsePkgFiles(t) {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Name.Name == "setVar" {
				continue
			}
			ast.Inspect(fd, func(x ast.Node) bool {
				as, ok := x.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, l := range as.Lhs {
					if ix, ok := l.(*ast.IndexExpr); ok {
						if sel, ok := ix.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "vars" {
							t.Errorf("%s: %s writes e.vars[...] directly; use setVar so the taint flag is kept right", file, fd.Name.Name)
						}
					}
				}
				return true
			})
		}
	}
}

// the number of steps in each class, for the pull request text
func TestTaintClassCounts(t *testing.T) {
	counts := map[taintClass]int{}
	for _, c := range taintClasses {
		counts[c]++
	}
	t.Logf("sink %d, literal only %d, UI only %d, propagates %d, no string %d (total %d)", counts[classSink], counts[classLiteral], counts[classUI], counts[classPropagates], counts[classNoString], len(taintClasses))
}
