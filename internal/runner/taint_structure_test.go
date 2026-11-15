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

// handlerCalls returns the plain resolvers a handler reaches: it follows calls to functions and methods of this package by name (a small call graph, not
// type-checked, so a name shared by two types is followed for both) and stops at the guard. A call to a resolver is reported with the path to it.
func handlerCalls(files map[string]*ast.File, root ast.Node, stopAt map[string]bool) (resolvers []string, usesGuard bool) {
	decls := map[string]*ast.FuncDecl{}
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				decls[fd.Name.Name] = fd
			}
		}
	}
	plain := map[string]bool{}
	for _, r := range plainResolvers {
		plain[r] = true
	}
	seen := map[string]bool{}
	var visit func(n ast.Node, path string)
	visit = func(n ast.Node, path string) {
		for name := range callsIn(n) {
			switch {
			case name == "forSink" || name == "shotName":
				usesGuard = true
			case plain[name]:
				resolvers = append(resolvers, path+name)
			case stopAt[name] || seen[name]:
			case decls[name] != nil:
				seen[name] = true
				visit(decls[name], path+name+" -> ")
			}
		}
	}
	visit(root, "")
	sort.Strings(resolvers)
	return
}

// stopAtFuncs are functions the scan does not descend into. The scan follows calls by name through this package, so it must not follow a nested step back
// into the dispatcher (every handler would reach every other); the first part of the list is the guard and the expansion itself.
var stopAtFuncs = map[string]bool{"resolveT": true, "expandT": true, "setVar": true, "forSink": true, "resolveVars": true,
	// re-entry into the executor: running a nested step is that step's own handler, classified on its own; and the line shown for a step is display only
	"runStep": true, "RunBody": true, "RunStep": true, "dispatchStep": true, "Run": true, "runAction": true, "runAssert": true, "runWait": true,
	"stepDescription": true, "emitStep": true, "Reconnect": true}

// sinkProblems lists what is wrong with the handler of a sink: no guard, or a plain resolver reached directly or through a helper.
func sinkProblems(files map[string]*ast.File, name, fn string, allowed string) []string {
	body := bodyOf2(files, name, fn)
	if body == nil {
		return []string{name + ": the handler " + fn + " was not found"}
	}
	var out []string
	res, guard := handlerCalls(files, body, stopAtFuncs)
	if !guard {
		out = append(out, name+": its handler does not take strings through forSink")
	}
	for _, r := range res {
		if allowed != "" && strings.HasSuffix(r, allowed) {
			continue
		}
		out = append(out, name+": reaches "+r+"; a sink must use forSink")
	}
	return out
}

func literalProblems(files map[string]*ast.File, name, fn string) []string {
	body := bodyOf2(files, name, fn)
	if body == nil {
		return []string{name + ": the handler " + fn + " was not found"}
	}
	var out []string
	res, guard := handlerCalls(files, body, stopAtFuncs)
	if guard {
		out = append(out, name+": a literal-only handler calls the guard")
	}
	for _, r := range res {
		out = append(out, name+": reaches "+r+"; if it now expands variables it must become a sink (taint_classes.go) and use forSink")
	}
	return out
}

// bodyOf2 is bodyOf for a files map.
func bodyOf2(files map[string]*ast.File, name, fn string) ast.Node {
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

// 2. The sinks take their strings through the guard and never reach a plain resolver, directly or through a helper of this package.
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
		for _, p := range sinkProblems(files, name, h.fn, allowedInSink[name]) {
			t.Error(p)
		}
	}
	for name := range sinkHandlers {
		if taintClasses[name] != classSink {
			t.Errorf("%s has a sinkHandlers entry but is not classified as a sink", name)
		}
	}
	// shotName is the shared helper of the two screenshot sinks: it must use the guard itself
	if sn := bodyOf2(files, "", "shotName"); sn == nil || callsIn(sn)["forSink"] == 0 {
		t.Errorf("shotName does not use forSink")
	}
}

// 3. The literal-only handlers stay literal, also through helpers.
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
		for _, p := range literalProblems(files, name, h.fn) {
			t.Error(p)
		}
	}
	for name := range literalHandlers {
		if taintClasses[name] != classLiteral {
			t.Errorf("%s has a literalHandlers entry but is not classified as literal only", name)
		}
	}
}

func parseFiles(t *testing.T, srcs map[string]string) map[string]*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for n, src := range srcs {
		f, err := parser.ParseFile(fset, n, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		out[n] = f
	}
	return out
}

// The scan reports what it is there to report: a second string that goes through a helper that resolves it, a missing guard, a literal handler that starts to
// expand, also through two helpers; and it accepts the real shape.
func TestTheHandlerScansReportTheirNegativeCases(t *testing.T) {
	files := parseFiles(t, map[string]string{"x.go": `package runner
func (e *Executor) runX(a A) error {
	first, err := e.forSink("x", a.Name)
	_ = first
	return e.sendSecond(a.Text, err)
}
func (e *Executor) sendSecond(raw string, err error) error { v := e.viaDeeper(raw); _ = v; return err }
func (e *Executor) viaDeeper(raw string) string { return e.resolve(raw) }
func (e *Executor) runNoGuard(a A) error { return nil }
func (e *Executor) runGood(a A) error { s, _ := e.forSink("g", a.Name); _ = s; return nil }
func (e *Executor) runLit(a A) error { return e.helperLit(a) }
func (e *Executor) helperLit(a A) error { _ = e.resolveVars(a.Name); return nil }
func (e *Executor) runLitClean(a A) error { return nil }
func (e *Executor) runSelf(a A) error { _ = e.runSelf; _ = e.forSink; return e.runSelf(a) }
`})
	if p := sinkProblems(files, "X", "runX", ""); len(p) == 0 || !strings.Contains(strings.Join(p, " "), "sendSecond -> viaDeeper -> resolve") {
		t.Errorf("a second string through two helpers was not reported: %v", p)
	}
	if p := sinkProblems(files, "X", "runNoGuard", ""); len(p) == 0 {
		t.Errorf("a sink with no guard was not reported")
	}
	if p := sinkProblems(files, "X", "runGood", ""); len(p) != 0 {
		t.Errorf("a good sink was reported: %v", p)
	}
	if p := sinkProblems(files, "X", "runMissing", ""); len(p) == 0 {
		t.Errorf("a missing handler was not reported")
	}
	if p := literalProblems(files, "L", "runLit"); len(p) == 0 || !strings.Contains(strings.Join(p, " "), "helperLit -> resolveVars") {
		t.Errorf("a literal handler that expands through a helper was not reported: %v", p)
	}
	if p := literalProblems(files, "L", "runLitClean"); len(p) != 0 {
		t.Errorf("a clean literal handler was reported: %v", p)
	}
	if p := literalProblems(files, "L", "runGood"); len(p) == 0 {
		t.Errorf("a literal handler that calls the guard was not reported")
	}
	// recursion does not loop
	_ = sinkProblems(files, "X", "runSelf", "")
}

// varsMisuses reports every use of the `vars` field outside the functions named in allowed, other than a plain read: an element read (not as the target of an
// assignment, ++ or &), len(...), or ranging over it. An assignment, an alias (`v := e.vars`), delete, clear, maps.Copy, passing it to any function, returning
// it and `e.vars = ...` are all reported, so the only way to change a variable is setVar.
func varsMisuses(fset *token.FileSet, f *ast.File, allowed map[string]bool) []string {
	var out []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || allowed[fd.Name.Name] {
			continue
		}
		var stack []ast.Node
		ast.Inspect(fd, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "vars" && len(stack) > 0 {
				parent := stack[len(stack)-1]
				var grand ast.Node
				if len(stack) > 1 {
					grand = stack[len(stack)-2]
				}
				fine := false
				switch p := parent.(type) {
				case *ast.IndexExpr: // e.vars[k]
					fine = p.X == ast.Expr(sel)
					switch g := grand.(type) {
					case *ast.AssignStmt:
						for _, l := range g.Lhs {
							if l == ast.Expr(p) {
								fine = false
							}
						}
					case *ast.IncDecStmt:
						fine = false
					case *ast.UnaryExpr:
						if g.Op == token.AND {
							fine = false
						}
					}
				case *ast.CallExpr: // len(e.vars)
					if id, ok := p.Fun.(*ast.Ident); ok && id.Name == "len" {
						fine = true
					}
				case *ast.RangeStmt:
					fine = p.X == ast.Expr(sel)
				}
				if !fine {
					out = append(out, fset.Position(sel.Pos()).String()+" in "+fd.Name.Name)
				}
			}
			stack = append(stack, n)
			return true
		})
	}
	return out
}

// 4. Every change to the variable map goes through setVar, which keeps the taint flag right.
func TestEveryVariableWriteGoesThroughSetVar(t *testing.T) {
	fset := token.NewFileSet()
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range varsMisuses(fset, f, map[string]bool{"setVar": true}) {
			t.Errorf("%s uses the vars field in a way other than reading it; use setVar so the taint flag is kept right", m)
		}
	}
}

func parseSnippet(t *testing.T, body string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "snippet.go", "package runner\nimport \"maps\"\nvar _ = maps.Copy[map[string]string, map[string]string]\nfunc (e *Executor) f(k string, other map[string]string) map[string]string {\n"+body+"\n return nil\n}\n", 0)
	if err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	return fset, f
}

// The ways round setVar the first version of this test missed are reported; plain reads are not.
func TestTheVariableWriteCheckReportsEveryWayRoundSetVar(t *testing.T) {
	for _, bad := range []string{
		`e.vars[k] = "x"`, `e.vars[k] += "x"`, `e.vars[k], _ = "a", 1`, `v := e.vars; v[k] = "x"`, `var v = e.vars; _ = v`, `delete(e.vars, k)`, `clear(e.vars)`,
		`e.vars = other`, `e.vars = map[string]string{}`, `maps.Copy(e.vars, other)`, `maps.Copy(other, e.vars)`, `consume(e.vars)`, `return e.vars`, `p := &e.vars[k]; _ = p`,
		`for k := range other { e.vars[k] = "x" }`, `other = e.vars`,
	} {
		fset, f := parseSnippet(t, bad)
		if len(varsMisuses(fset, f, nil)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	for _, ok := range []string{`_ = e.vars[k]`, `v, found := e.vars[k]; _, _ = v, found`, `_ = len(e.vars)`, `for key, val := range e.vars { _, _ = key, val }`, `if e.vars[k] != "" { }`, `other[k] = e.vars[k]`} {
		fset, f := parseSnippet(t, ok)
		if m := varsMisuses(fset, f, nil); len(m) != 0 {
			t.Errorf("reported although it only reads: %s: %v", ok, m)
		}
	}
	// and the function that is allowed to write is the only one
	fset, f := parseSnippet(t, `e.vars[k] = "x"`)
	if len(varsMisuses(fset, f, map[string]bool{"f": true})) != 0 {
		t.Errorf("the allowed function was reported")
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
