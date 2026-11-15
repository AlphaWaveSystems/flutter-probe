package runner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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

// plainResolvers are the names a sink or literal-only handler must not reach, by any reference: a call, a method value (`f := e.resolve`), or a bare name.
var plainResolvers = []string{"resolve", "resolveT", "resolveVars", "resolveSelector", "expandEnv", "expandVars", "expandT", "finishResolve"}

// allowedInSink lists the plain resolvers a sink handler may also call, with the reason: `compare screenshot ... of <selector>` resolves the selector it
// crops to, which is matched on the screen (the name of the screenshot, the sink part, goes through shotName and forSink).
var allowedInSink = map[string]string{"VerbCompareShot": "resolveSelector"}

// declsByName lists every function and method declared in the package files, by name (a name can belong to several types: all of them are followed).
func declsByName(files map[string]*ast.File) map[string][]*ast.FuncDecl {
	out := map[string][]*ast.FuncDecl{}
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				out[fd.Name.Name] = append(out[fd.Name.Name], fd)
			}
		}
	}
	return out
}

// qualified is "Type.name" for a method and "name" for a function.
func qualified(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	switch r := fd.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return r.Name + "." + fd.Name.Name
	case *ast.StarExpr:
		if id, ok := r.X.(*ast.Ident); ok {
			return id.Name + "." + fd.Name.Name
		}
	}
	return fd.Name.Name
}

// varsReadAllowed are the helpers that may read the variable map: shotName only asks whether the script defined a variable with the name of a built-in.
var varsReadAllowed = map[string]bool{"Executor.shotName": true}

// scanHandler returns what a handler reaches that it must not: a reference to a plain resolver (a call, a method value or a bare name) and any read of the
// variable map, in the handler or in any function of the package it calls (followed by name; a name shared by two types is followed for both, and a function
// is stopped at only by its qualified name in stopAt). usesGuard says whether the handler calls e.forSink or e.shotName.
func scanHandler(files map[string]*ast.File, root ast.Node, stopAt map[string]bool) (violations []string, usesGuard bool) {
	decls := declsByName(files)
	plain := map[string]bool{}
	for _, r := range plainResolvers {
		plain[r] = true
	}
	// a package-level variable initialised from a resolver (`var expand = resolveSelector`) is a resolver by another name
	for changed := true; changed; {
		changed = false
		for _, f := range files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					refs := false
					for _, v := range vs.Values {
						ast.Inspect(v, func(x ast.Node) bool {
							switch n := x.(type) {
							case *ast.Ident:
								refs = refs || plain[n.Name]
							case *ast.SelectorExpr:
								refs = refs || plain[n.Sel.Name]
							}
							return true
						})
					}
					if refs {
						for _, n := range vs.Names {
							if !plain[n.Name] {
								plain[n.Name] = true
								changed = true
							}
						}
					}
				}
			}
		}
	}
	seen := map[*ast.FuncDecl]bool{}
	var visit func(n ast.Node, path string, owner string)
	visit = func(n ast.Node, path string, owner string) {
		var callees []string
		ast.Inspect(n, func(x ast.Node) bool {
			switch v := x.(type) {
			case *ast.CallExpr:
				switch f := v.Fun.(type) {
				case *ast.SelectorExpr:
					if id, ok := f.X.(*ast.Ident); ok && id.Name == "e" && (f.Sel.Name == "forSink" || f.Sel.Name == "shotName") {
						usesGuard = true
					}
					callees = append(callees, f.Sel.Name)
				case *ast.Ident:
					callees = append(callees, f.Name)
				}
			case *ast.SelectorExpr:
				if plain[v.Sel.Name] {
					violations = append(violations, path+v.Sel.Name)
				}
				if v.Sel.Name == "vars" && !varsReadAllowed[owner] {
					violations = append(violations, path+"vars (a read of the variable map)")
				}
			case *ast.Ident:
				if plain[v.Name] {
					if _, isDecl := decls[v.Name]; isDecl || true {
						violations = append(violations, path+v.Name)
					}
				}
			}
			return true
		})
		for _, name := range callees {
			for _, fd := range decls[name] {
				if seen[fd] || stopAt[qualified(fd)] || plain[name] || name == "forSink" || name == "shotName" {
					continue
				}
				seen[fd] = true
				visit(fd, path+qualified(fd)+" -> ", qualified(fd))
			}
		}
	}
	visit(root, "", "")
	sort.Strings(violations)
	// a method call `e.resolve(x)` is seen both as a selector and as a callee; report each path once
	out := violations[:0]
	for i, v := range violations {
		if i == 0 || v != violations[i-1] {
			out = append(out, v)
		}
	}
	return out, usesGuard
}

// stopAtFuncs are the functions the scan does not descend into, by qualified name. The scan follows calls by name through this package, so it must not follow
// a nested step back into the dispatcher (every handler would reach every other): the nested step has its own handler, classified on its own. The line
// shown for a step is display only. None of the resolvers may be here (a test checks), or a sink could hide behind one.
var stopAtFuncs = map[string]bool{
	"Executor.setVar": true, "Executor.forSink": true,
	"Executor.runStep": true, "Executor.RunBody": true, "Executor.RunStep": true, "Executor.dispatchStep": true,
	"Executor.runAction": true, "Executor.runAssert": true, "Executor.runWait": true,
	"Executor.stepDescription": true, "Executor.emitStep": true,
	"Runner.Run": true, "DeviceContext.Reconnect": true,
}

// sinkProblems lists what is wrong with the handler of a sink: no guard, or a plain resolver reached directly or through a helper.
func sinkProblems(files map[string]*ast.File, name, fn string, allowed string) []string {
	body := bodyOf(files, name, fn)
	if body == nil {
		return []string{name + ": the handler " + fn + " was not found"}
	}
	var out []string
	res, guard := scanHandler(files, body, stopAtFuncs)
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
	body := bodyOf(files, name, fn)
	if body == nil {
		return []string{name + ": the handler " + fn + " was not found"}
	}
	var out []string
	res, guard := scanHandler(files, body, stopAtFuncs)
	if guard {
		out = append(out, name+": a literal-only handler calls the guard")
	}
	for _, r := range res {
		out = append(out, name+": reaches "+r+"; if it now expands variables it must become a sink (taint_classes.go) and use forSink")
	}
	return out
}

// bodyOf finds the code of a handler: the whole function, or the case of runAction for a verb.
func bodyOf(files map[string]*ast.File, name, fn string) ast.Node {
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
	if sn := bodyOf(files, "", "shotName"); sn == nil || callsIn(sn)["forSink"] == 0 {
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
	if p := sinkProblems(files, "X", "runX", ""); len(p) == 0 || !strings.Contains(strings.Join(p, " "), "Executor.sendSecond -> Executor.viaDeeper -> resolve") {
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
	if p := literalProblems(files, "L", "runLit"); len(p) == 0 || !strings.Contains(strings.Join(p, " "), "Executor.helperLit -> resolveVars") {
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

// varsMisuses reports every use of the map field `field` (vars, tainted) outside the functions named in allowed, other than a plain read: an element read (not as the target of an
// assignment, ++ or &), len(...), or ranging over it. An assignment, an alias (`v := e.vars`), delete, clear, maps.Copy, passing it to any function, returning
// it and `e.vars = ...` are all reported, so the only way to change a variable is setVar.
func varsMisuses(fset *token.FileSet, f *ast.File, field string, allowed map[string]bool) []string {
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
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == field && len(stack) > 0 {
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
		for _, m := range varsMisuses(fset, f, "vars", map[string]bool{"setVar": true}) {
			t.Errorf("%s uses the vars field in a way other than reading it; use setVar so the taint flag is kept right", m)
		}
		// the set of tainted names is changed by setVar alone, and the clipboard flag by its own two methods
		for _, m := range varsMisuses(fset, f, "tainted", map[string]bool{"setVar": true}) {
			t.Errorf("%s changes the tainted set outside setVar", m)
		}
		for _, m := range flagWrites(fset, f, "holdsTaint", map[string]bool{"set": true}) {
			t.Errorf("%s writes the clipboard flag outside clipboardTaint.set", m)
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
		if len(varsMisuses(fset, f, "vars", nil)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	for _, ok := range []string{`_ = e.vars[k]`, `v, found := e.vars[k]; _, _ = v, found`, `_ = len(e.vars)`, `for key, val := range e.vars { _, _ = key, val }`, `if e.vars[k] != "" { }`, `other[k] = e.vars[k]`} {
		fset, f := parseSnippet(t, ok)
		if m := varsMisuses(fset, f, "vars", nil); len(m) != 0 {
			t.Errorf("reported although it only reads: %s: %v", ok, m)
		}
	}
	// and the function that is allowed to write is the only one
	fset, f := parseSnippet(t, `e.vars[k] = "x"`)
	if len(varsMisuses(fset, f, "vars", map[string]bool{"f": true})) != 0 {
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

// More ways a handler could reach an expansion without a plain call: all reported. A same-named helper on two types is followed for both.
func TestTheHandlerScansReportMethodValuesVariableReadsAndSharedNames(t *testing.T) {
	files := parseFiles(t, map[string]string{"y.go": `package runner
func (e *Executor) runValue(a A) error { f := e.resolve; _ = f(a.Name); s, _ := e.forSink("v", a.Text); _ = s; return nil }
var expander = resolveSelector
func (e *Executor) runBare(a A) error { _ = expander; s, _ := e.forSink("v", a.Text); _ = s; return nil }
func (e *Executor) runReads(a A) error { _ = e.vars[a.Name]; s, _ := e.forSink("v", a.Text); _ = s; return nil }
func (e *Executor) runTwoTypes(a A) error { s, _ := e.forSink("v", a.Text); _ = s; e.helper(a); return nil }
func (e *Executor) helper(a A) {}
func (o *Other) helper(a A) { _ = o.e.resolve(a.Name) }
func (e *Executor) runFinish(a A) error { s, _ := e.forSink("v", a.Text); _ = e.finishResolve(s); return nil }
func (e *Executor) runViaStopped(a A) error { s, _ := e.forSink("v", a.Text); _ = s; e.runStop(a); return nil }
func (e *Executor) runStop(a A) { _ = e.resolve(a.Name) }
func (e *Executor) runStep(a A) { _ = e.resolve(a.Name) }
func (o *Other) runStep(a A) { _ = o.e.resolve(a.Name) }
func (e *Executor) runOtherTypeStop(a A) error { s, _ := e.forSink("v", a.Text); _ = s; o.runStep(a); return nil }
`})
	for fn, want := range map[string]string{
		"runValue": "resolve", "runBare": "expander", "runReads": "vars (a read of the variable map)", "runTwoTypes": "Other.helper -> resolve", "runFinish": "finishResolve",
	} {
		if p := sinkProblems(files, "X", fn, ""); len(p) == 0 || !strings.Contains(strings.Join(p, " "), want) {
			t.Errorf("%s: not reported (want %q): %v", fn, want, p)
		}
	}
	// a stop-listed name on the Executor stops the scan; the same name on another type does not
	stop := map[string]bool{"Executor.runStep": true, "Executor.runStop": true}
	if v, _ := scanHandler(files, bodyOf(files, "X", "runViaStopped"), stop); len(v) != 0 {
		t.Errorf("a stop-listed method was followed: %v", v)
	}
	if v, _ := scanHandler(files, bodyOf(files, "X", "runOtherTypeStop"), stop); len(v) == 0 {
		t.Errorf("a same-named method on another type was skipped")
	}
}

// The stop list cannot hide a resolver or a guard bypass behind a name.
func TestTheStopListNamesNoResolver(t *testing.T) {
	for q := range stopAtFuncs {
		name := q[strings.LastIndex(q, ".")+1:]
		for _, r := range plainResolvers {
			if name == r {
				t.Errorf("%s is in the stop list and is a resolver", q)
			}
		}
	}
	// and every entry is a function or method that exists, so a rename cannot leave a stale stop
	files := parsePkgFiles(t)
	decls := declsByName(files)
	for q := range stopAtFuncs {
		name := q[strings.LastIndex(q, ".")+1:]
		found := false
		for _, fd := range decls[name] {
			if qualified(fd) == q {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is in the stop list but is not declared", q)
		}
	}
}

// flagWrites reports every assignment to the field (a plain bool) outside the functions in allowed.
func flagWrites(fset *token.FileSet, f *ast.File, field string, allowed map[string]bool) []string {
	var out []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || allowed[fd.Name.Name] {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				for _, l := range v.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
						out = append(out, fset.Position(sel.Pos()).String()+" in "+fd.Name.Name)
					}
				}
			case *ast.IncDecStmt:
				if sel, ok := v.X.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
					out = append(out, fset.Position(sel.Pos()).String()+" in "+fd.Name.Name)
				}
			}
			return true
		})
	}
	return out
}

// The taint fields have their own writers: the other ways to change them are reported.
func TestTheTaintFieldChecksReportTheirNegativeCases(t *testing.T) {
	for _, bad := range []string{`e.tainted = nil`, `delete(e.tainted, k)`, `e.tainted[k] = true`, `clear(e.tainted)`, `t := e.tainted; t[k] = true`, `maps.Copy(e.tainted, other)`} {
		fset, f := parseSnippet(t, bad)
		if len(varsMisuses(fset, f, "tainted", nil)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	for _, ok := range []string{`_ = e.tainted[k]`, `_ = len(e.tainted)`, `for n := range e.tainted { _ = n }`} {
		fset, f := parseSnippet(t, ok)
		if m := varsMisuses(fset, f, "tainted", nil); len(m) != 0 {
			t.Errorf("reported although it only reads: %s: %v", ok, m)
		}
	}
	for _, bad := range []string{`c.holdsTaint = false`, `c.holdsTaint = !c.holdsTaint`, `c.holdsTaint, k = true, "x"`} {
		fset, f := parseSnippet(t, bad)
		if len(flagWrites(fset, f, "holdsTaint", nil)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	fset, f := parseSnippet(t, `_ = c.holdsTaint`)
	if len(flagWrites(fset, f, "holdsTaint", nil)) != 0 {
		t.Errorf("a read of the flag was reported")
	}
}

// qualifiedFuncs walks the functions of a file with their qualified names.
func eachFunc(f *ast.File, fn func(q string, fd *ast.FuncDecl)) {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			fn(qualified(fd), fd)
		}
	}
}

// clipboardMisuses reports the ways to change the clipboard flag other than the copy handler: a call of the setter outside afterCopy, a call of afterCopy outside
// runAction, an assignment to a field named clip (a struct holding the flag) outside the places that hand it to an executor, and a new flag made anywhere else.
func clipboardMisuses(fset *token.FileSet, f *ast.File, allowedAssign, allowedLiteral map[string]bool) []string {
	var out []string
	eachFunc(f, func(q string, fd *ast.FuncDecl) {
		ast.Inspect(fd, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
					recv := types.ExprString(sel.X)
					switch {
					case sel.Sel.Name == "set" && strings.Contains(recv, "clip") && q != "clipboardTaint.afterCopy":
						out = append(out, fset.Position(sel.Pos()).String()+" in "+q+": the clipboard flag is set outside afterCopy")
					case sel.Sel.Name == "afterCopy" && q != "Executor.runAction":
						out = append(out, fset.Position(sel.Pos()).String()+" in "+q+": afterCopy is called outside the copy handler")
					}
				}
			case *ast.AssignStmt:
				for _, l := range v.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "clip" && !allowedAssign[q] {
						out = append(out, fset.Position(sel.Pos()).String()+" in "+q+": the clipboard flag holder is assigned")
					}
				}
			case *ast.CompositeLit:
				if id, ok := v.Type.(*ast.Ident); ok && id.Name == "clipboardTaint" && !allowedLiteral[q] {
					out = append(out, fset.Position(v.Pos()).String()+" in "+q+": a new clipboard flag is made")
				}
			}
			return true
		})
	})
	return out
}

// resolverValues reports every use of a resolver as a value (stored in a field, a table, a variable, a composite literal, passed on) anywhere in the package:
// only a call, or the declaration itself, is fine. So a sink cannot reach an expansion through a field or a table filled somewhere the scans do not look.
func resolverValues(fset *token.FileSet, f *ast.File) []string {
	plain := map[string]bool{}
	for _, r := range plainResolvers {
		plain[r] = true
	}
	var out []string
	var stack []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		var name string
		var pos token.Pos
		switch v := n.(type) {
		case *ast.SelectorExpr:
			name, pos = v.Sel.Name, v.Sel.Pos()
		case *ast.Ident:
			name, pos = v.Name, v.Pos()
		}
		if plain[name] && len(stack) > 0 {
			parent := stack[len(stack)-1]
			isCall := false
			isDecl := false
			switch p := parent.(type) {
			case *ast.CallExpr:
				switch fun := p.Fun.(type) {
				case *ast.SelectorExpr:
					isCall = fun == n || fun.Sel == n
				case *ast.Ident:
					isCall = fun == n
				}
			case *ast.SelectorExpr: // the name inside `x.resolve`: judged at the selector
				isCall = p.Sel == n
				if isCall && len(stack) > 1 {
					if call, ok := stack[len(stack)-2].(*ast.CallExpr); ok {
						isCall = call.Fun == ast.Expr(p)
					} else {
						isCall = false
					}
				}
			case *ast.FuncDecl:
				isDecl = p.Name == n
			}
			if _, isSel := n.(*ast.SelectorExpr); isSel {
				isCall = false // judged through its Sel ident, above
				isDecl = true
			}
			if !isCall && !isDecl {
				out = append(out, fset.Position(pos).String()+": "+name+" is used as a value")
			}
		}
		stack = append(stack, n)
		return true
	})
	return out
}

func TestClipboardFlagAndResolversHaveNoSideDoors(t *testing.T) {
	fset := token.NewFileSet()
	entries, _ := os.ReadDir(".")
	assign := map[string]bool{"Runner.newExecutor": true, "Executor.clipboard": true, "CompositeRunner.RunCompositeTest": true}
	literal := map[string]bool{"Executor.clipboard": true, "CompositeRunner.clipFor": true}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range clipboardMisuses(fset, f, assign, literal) {
			t.Error(m)
		}
		for _, m := range resolverValues(fset, f) {
			t.Error(m)
		}
	}
}

func TestTheSideDoorChecksReportTheirNegativeCases(t *testing.T) {
	for _, bad := range []string{
		`e.clipboard().set(false)`, `e.deviceCtx.clip.set(true)`, `c.clip.set(true)`, `e.deviceCtx.clip = clipboardTaint{}`, `e.clip = &clipboardTaint{}`, `e.clip = nil`,
		`x := clipboardTaint{}; _ = x`, `e.clipboard().afterCopy(false, nil)`,
	} {
		fset, f := parseSnippet(t, bad)
		if len(clipboardMisuses(fset, f, nil, nil)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	if fset, f := parseSnippet(t, `_ = e.clipboard().get()`); len(clipboardMisuses(fset, f, nil, nil)) != 0 {
		t.Errorf("a read of the flag was reported")
	}
	for _, bad := range []string{
		`exec.zzf = exec.resolve`, `table["x"] = e.resolveT`, `_ = T{f: e.resolve}`, `g := resolveSelector; _ = g`, `consume(e.expandEnv)`, `_ = []func(string) string{e.resolveVars}`,
		`exec.zzf = e.finishResolve`, `x := expandVars; _ = x`,
	} {
		fset, f := parseSnippet(t, bad)
		if len(resolverValues(fset, f)) == 0 {
			t.Errorf("not reported: %s", bad)
		}
	}
	for _, ok := range []string{`_ = e.resolve("x")`, `_ = resolveSelector(nil)`, `s := e.expandEnv(e.resolveVars("x")); _ = s`, `go e.resolve("x")`} {
		fset, f := parseSnippet(t, ok)
		if m := resolverValues(fset, f); len(m) != 0 {
			t.Errorf("reported although it is a call: %s: %v", ok, m)
		}
	}
}

// The two places that build executors for tests hand them the shared clipboard flag: a missing assignment would give every test a fresh, clean flag.
func TestEveryExecutorBuilderSharesTheClipboardFlag(t *testing.T) {
	files := parsePkgFiles(t)
	for _, q := range []string{"Runner.newExecutor", "CompositeRunner.RunCompositeTest"} {
		found, assigns := false, false
		for _, f := range files {
			eachFunc(f, func(name string, fd *ast.FuncDecl) {
				if name != q {
					return
				}
				found = true
				ast.Inspect(fd, func(n ast.Node) bool {
					if as, ok := n.(*ast.AssignStmt); ok {
						for _, l := range as.Lhs {
							if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "clip" {
								assigns = true
							}
						}
					}
					return true
				})
			})
		}
		if !found || !assigns {
			t.Errorf("%s: found = %v, assigns the clipboard flag = %v", q, found, assigns)
		}
	}
	// and nothing else builds an Executor for a run
	for name, f := range files {
		eachFunc(f, func(q string, fd *ast.FuncDecl) {
			ast.Inspect(fd, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "NewExecutor" && q != "Runner.newExecutor" && q != "CompositeRunner.RunCompositeTest" {
						t.Errorf("%s: %s builds an Executor itself; it must share the clipboard flag too", name, q)
					}
				}
				return true
			})
		})
	}
}
