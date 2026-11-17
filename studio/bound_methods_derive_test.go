package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The recorded "touches" of a bound method are checked against the code: a call graph of the package's own non-test sources is walked from each
// method, and every call into the keychain, the file system or the network that it can reach is a touch the entry must list. The walk is wide on
// purpose (a name that a function or method of the package has is followed, whatever it is called on), so it can ask for a touch that a method does
// not really make, never the other way round; an entry may list more than the walk finds, never less. What it cannot see (a call through an
// interface or a function value that is not named like a function of the package) is not found, and that is why the entries are still read by a person.

// sinkOf says what a call into a package outside Studio touches ("" for nothing that matters here). Keys are import paths, values are the touched
// thing for every function of the package, or per function name when the package mixes (os, filepath, net).
var sinkPackages = map[string]string{
	"github.com/zalando/go-keyring": "keychain",
	"net/http":                      "network",
	"github.com/alphawavesystems/flutter-probe/internal/userprefs":  "files",
	"github.com/alphawavesystems/flutter-probe/internal/secretfile": "files",
	"io/ioutil": "files",
}

var sinkFuncs = map[string]map[string]string{
	"github.com/alphawavesystems/flutter-probe/internal/config": {"Load": "files", "LoadForRepair": "files"},
	"github.com/alphawavesystems/flutter-probe/internal/ai":     {"NewTextCompleter": "network", "NewChatCompleter": "network", "Triage": "network", "Doctor": "network"},
	"os": {"ReadFile": "files", "WriteFile": "files", "Open": "files", "OpenFile": "files", "Create": "files", "ReadDir": "files", "Stat": "files", "Lstat": "files",
		"Mkdir": "files", "MkdirAll": "files", "MkdirTemp": "files", "CreateTemp": "files", "Remove": "files", "RemoveAll": "files", "Rename": "files",
		"Chmod": "files", "Truncate": "files", "Readlink": "files", "Symlink": "files"},
	"path/filepath": {"Walk": "files", "WalkDir": "files", "Glob": "files", "EvalSymlinks": "files"},
	"net":           {"Dial": "network", "DialTimeout": "network", "Listen": "network", "LookupHost": "network", "LookupIP": "network", "LookupAddr": "network", "ListenPacket": "network", "ListenUDP": "network", "DialUDP": "network", "DialTCP": "network", "ResolveTCPAddr": "network"},
}

// notFollowed are method names that are, in this package, nearly always the standard library's (a Body or a connection being closed) and are also
// the name of one of the package's own methods (the automation server's Close removes its discovery file): following them would charge every method
// that closes a response with a file it never touches. The automation server's Close is not reachable from a bound method.
var notFollowed = map[string]bool{"Close": true}

type pkgSources struct {
	funcs   map[string][]*ast.FuncDecl // by bare name: package functions and methods of any type share one namespace
	imports map[*ast.FuncDecl]map[string]string
}

func loadPackageSources() (*pkgSources, error) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		return nil, err
	}
	ps := &pkgSources{funcs: map[string][]*ast.FuncDecl{}, imports: map[*ast.FuncDecl]map[string]string{}}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			return nil, err
		}
		imps := map[string]string{}
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			local := strings.TrimPrefix(p[strings.LastIndex(p, "/")+1:], "go-") // github.com/zalando/go-keyring is package keyring
			if im.Name != nil {
				local = im.Name.Name
			}
			imps[local] = p
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					ps.funcs[d.Name.Name] = append(ps.funcs[d.Name.Name], d)
					ps.imports[d] = imps
				}
			case *ast.GenDecl:
				// a package-level variable that holds a function (apiKeyLookup = keychainGet) is a name that leads to what it is set to
				for _, sp := range d.Specs {
					if vs, ok := sp.(*ast.ValueSpec); ok {
						for i, id := range vs.Names {
							if i < len(vs.Values) {
								fn := &ast.FuncDecl{Name: id, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: vs.Values[i]}}}}
								ps.funcs[id.Name] = append(ps.funcs[id.Name], fn)
								ps.imports[fn] = imps
							}
						}
					}
				}
			}
		}
	}
	return ps, nil
}

// reach is what the method (or function) name can touch, following every name of the package that it mentions.
func (ps *pkgSources) reach(name string) []string {
	got := ps.reachVia(name)
	var out []string
	for t := range got {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// reachVia is reach with, for each touch, the first chain of names that leads to it (for the message that says why an entry is understated).
func (ps *pkgSources) reachVia(name string) map[string]string {
	touched := map[string]string{}
	seen := map[string]bool{}
	var chain []string
	var visit func(string)
	visit = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		chain = append(chain, n)
		defer func() { chain = chain[:len(chain)-1] }()
		for _, fn := range ps.funcs[n] {
			imps := ps.imports[fn]
			selNames := map[*ast.Ident]bool{} // the name after a dot is followed by the SelectorExpr case below, not again as a bare name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch x := node.(type) {
				case *ast.SelectorExpr:
					selNames[x.Sel] = true
					if id, ok := x.X.(*ast.Ident); ok {
						if path, isPkg := imps[id.Name]; isPkg {
							if t := sinkPackages[path]; t != "" && touched[t] == "" {
								touched[t] = strings.Join(chain, " -> ") + " -> " + id.Name + "." + x.Sel.Name
							}
							if t := sinkFuncs[path][x.Sel.Name]; t != "" && touched[t] == "" {
								touched[t] = strings.Join(chain, " -> ") + " -> " + id.Name + "." + x.Sel.Name
							}
						}
					}
					if _, ok := ps.funcs[x.Sel.Name]; ok && !notFollowed[x.Sel.Name] {
						visit(x.Sel.Name)
					}
				case *ast.Ident:
					if _, ok := ps.funcs[x.Name]; ok && !selNames[x] {
						visit(x.Name)
					}
				}
				return true
			})
		}
	}
	visit(name)
	return touched
}

func init() {
	if os.Getenv("DUMP_TOUCHES") == "" {
		return
	}
}

// checkTouches says, for each decision, which touches the code reaches that the entry does not list. reach is the walk (or a stand-in in the
// tests of the check itself). A keychain entry is also checked the other way: the keychain is reached through one package only, so an entry that
// lists it for a method that cannot reach it is a false claim too.
func checkTouches(decisions []boundDecision, reach func(string) map[string]string) []string {
	var bad []string
	for _, d := range decisions {
		got := reach(d.Method)
		var kinds []string
		for k := range got {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			if !has(d.Touches, k) {
				bad = append(bad, fmt.Sprintf("%s: the entry does not list %q, and the code reaches it (%s): add it to touches in studio/bound_methods.json", d.Method, k, got[k]))
			}
		}
		if has(d.Touches, "keychain") && got["keychain"] == "" {
			bad = append(bad, d.Method+": the entry lists the keychain and no call to it is reachable from the method: remove it from touches")
		}
	}
	return bad
}

func TestTheRecordedTouchesAreWhatTheCodeReaches(t *testing.T) {
	ps, err := loadPackageSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range checkTouches(append(append([]boundDecision{}, boundDecisions...), automationInternal...), ps.reachVia) {
		t.Error(msg)
	}
}

// The check of the touches fails on each kind of mistake, and the walk finds what it is meant to find in the real sources.
func TestTheTouchCheckFailsOnAnUnderstatedEntry(t *testing.T) {
	ps, err := loadPackageSources()
	if err != nil {
		t.Fatal(err)
	}
	pure := boundDecision{Method: "AIChatLabel", Category: catUI, Does: "A pure line, so the entry says.", Callers: []string{"window"}, Confirm: "none"}
	// a method that is recorded as pure but calls aiProvider reaches probe.yaml and the keychain
	bad := checkTouches([]boundDecision{pure}, ps.reachVia)
	if len(bad) != 2 || !strings.Contains(strings.Join(bad, "\n"), `"files"`) || !strings.Contains(strings.Join(bad, "\n"), `"keychain"`) {
		t.Errorf("a method recorded as pure that reaches probe.yaml and the keychain: %v", bad)
	}
	// each real finding of the review, as data: remove one touch and the check says which
	for _, c := range []struct{ method, drop string }{{"Chat", "keychain"}, {"Chat", "files"}, {"Chat", "network"}, {"ConnectWiFi", "files"}, {"Connect", "files"}, {"ListDevices", "files"}, {"SetAPIKey", "keychain"}} {
		for _, d := range boundDecisions {
			if d.Method != c.method {
				continue
			}
			var rest []string
			for _, x := range d.Touches {
				if x != c.drop {
					rest = append(rest, x)
				}
			}
			d.Touches = rest
			if bad := checkTouches([]boundDecision{d}, ps.reachVia); len(bad) == 0 {
				t.Errorf("%s without %q was not caught", c.method, c.drop)
			}
		}
	}
	// the keychain claimed for a method that cannot reach it
	c := boundDecision{Method: "Lint", Category: catSecrets, Does: "Claims the keychain falsely.", Callers: []string{"window"}, Touches: []string{"keychain"}, Confirm: "none"}
	if bad := checkTouches([]boundDecision{c}, ps.reachVia); len(bad) != 1 || !strings.Contains(bad[0], "remove it from touches") {
		t.Errorf("a keychain claim with no call behind it: %v", bad)
	}
	// the walk follows a function held in a package variable and a method through its name, and does not follow Close
	if got := ps.reach("Chat"); !has(got, "keychain") || !has(got, "files") || !has(got, "network") {
		t.Errorf("the walk from Chat found %v, want files, keychain and network", got)
	}
	if got := ps.reach("Disconnect"); has(got, "files") {
		t.Errorf("Disconnect was charged with a file through a Close: %v", got)
	}
}

// The unexported methods the automation endpoint reaches are recorded, and a run it starts is gated like a run from the window.
func TestTheAutomationEndpointsInternalMethodsAreRecordedAndAGatedRunIsAsked(t *testing.T) {
	var unexported []string
	for _, m := range automationBackendMethods {
		if m[0] < 'A' || m[0] > 'Z' {
			unexported = append(unexported, m)
		}
	}
	sort.Strings(unexported)
	var recorded []string
	for _, d := range automationInternal {
		recorded = append(recorded, d.Method)
		if !has(d.Callers, "automation") || len(d.Callers) != 1 {
			t.Errorf("%s: an internal method is reached from the automation endpoint only", d.Method)
		}
		if !haveCat(d.Category) || len(strings.TrimSpace(d.Does)) < 10 || !validConfirm[d.Confirm] {
			t.Errorf("%s: an incomplete entry", d.Method)
		}
		if d.Category == catRunControl && d.Confirm != "asks" {
			t.Errorf("%s: a run the endpoint starts can reach `clear app data`: confirm must be 'asks'", d.Method)
		}
	}
	sort.Strings(recorded)
	if a, b := diff(unexported, recorded); len(a) > 0 || len(b) > 0 {
		t.Errorf("unexported methods of the automation backend without an automationInternal entry %v; entries for methods it does not reach %v", a, b)
	}
	// "asks" is true: a run, from the window or from the endpoint, goes through the device context of the connection (executeRun reads deviceCtx),
	// and every connection makes that context with confirmClearData, the question in the window. A run with no window to ask does not run the step.
	ps, err := loadPackageSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"runFileAsync", "RunFile", "RunFileScoped", "RunFileAsync"} {
		if !ps.mentions(m, "deviceCtx") {
			t.Errorf("%s no longer runs with the connection's device context: a clear step would not reach the gate", m)
		}
	}
	// the one place a device context is made sets its Confirm to confirmClearData's answer (not a mention of the name: an assignment), and both
	// ways to connect make their context there
	if !ps.assignsConfirmFrom("newStudioDeviceContextWith", "confirmClearData") {
		t.Error("newStudioDeviceContextWith does not set the context's Confirm from confirmClearData: a clear step would not ask the window")
	}
	for _, m := range []string{"Connect", "ConnectWiFi"} {
		if !ps.mentions(m, "newStudioDeviceContextWith") && !ps.mentions(m, "newStudioDeviceContext") {
			t.Errorf("%s no longer makes its device context where Confirm is set", m)
		}
	}
	if !ps.assignsConfirmFrom("newStudioDeviceContext", "newStudioDeviceContextWith") && !ps.mentions("newStudioDeviceContext", "newStudioDeviceContextWith") {
		t.Error("newStudioDeviceContext makes a device context without going through newStudioDeviceContextWith")
	}
}

// mentions says whether target is a name (a function, a method or a field) that the walk from name mentions.
func (ps *pkgSources) mentions(name, target string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(n string) bool {
		if n == target {
			return true
		}
		if seen[n] {
			return false
		}
		seen[n] = true
		for _, fn := range ps.funcs[n] {
			found := false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if found {
					return false
				}
				var next string
				switch x := node.(type) {
				case *ast.SelectorExpr:
					next = x.Sel.Name
				case *ast.Ident:
					next = x.Name
				}
				if next == target {
					found = true
				} else if _, ok := ps.funcs[next]; ok && !notFollowed[next] && visit(next) {
					found = true
				}
				return !found
			})
			if found {
				return true
			}
		}
		return false
	}
	return visit(name)
}

// assignsConfirmFrom says whether the function fn has an assignment `<x>.Confirm = <...>.callee(...)`: the Confirm of a device context set from the
// question in the window.
func (ps *pkgSources) assignsConfirmFrom(fn, callee string) bool {
	for _, d := range ps.funcs[fn] {
		found := false
		ast.Inspect(d.Body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				return true
			}
			l, ok := as.Lhs[0].(*ast.SelectorExpr)
			if !ok || l.Sel.Name != "Confirm" {
				return true
			}
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == callee {
					found = true
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}
