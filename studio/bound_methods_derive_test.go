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
	"io/ioutil":                     "files",
	"github.com/alphawavesystems/flutter-probe/internal/userprefs":  "files",
	"github.com/alphawavesystems/flutter-probe/internal/secretfile": "files",
	"github.com/grandcat/zeroconf":                                  "network",
	// a call into these reaches a device or its agent: the agent link (probelink, which holds the WebSocket and HTTP clients of the agent), the
	// device managers (adb, simctl), the device view, the device settings, the performance reader, and any process
	// the Studio starts (it starts adb and simctl)
	"github.com/alphawavesystems/flutter-probe/internal/probelink":   "devices",
	"github.com/alphawavesystems/flutter-probe/internal/device":      "devices",
	"github.com/alphawavesystems/flutter-probe/internal/deviceview":  "devices",
	"github.com/alphawavesystems/flutter-probe/internal/devsettings": "devices",
	"github.com/alphawavesystems/flutter-probe/internal/perf":        "devices",
	"os/exec":                      "devices",
	"github.com/gorilla/websocket": "devices",
	"nhooyr.io/websocket":          "devices",
}

var sinkFuncs = map[string]map[string]string{
	"github.com/alphawavesystems/flutter-probe/internal/toolpath": {"IsExecutableFile": "files", "Check": "files", "CheckIn": "files"},
	"github.com/alphawavesystems/flutter-probe/internal/httpcall": {"Do": "network"},                            // the rest of the package is types, errors and the address classes
	"github.com/alphawavesystems/flutter-probe/internal/runner":   {"New": "devices", "NewExecutor": "devices"}, // the others (reporters, lint, validation) are pure
	"github.com/alphawavesystems/flutter-probe/internal/config":   {"Load": "files", "LoadForRepair": "files"},
	"github.com/alphawavesystems/flutter-probe/internal/ai":       {"NewTextCompleter": "network", "NewChatCompleter": "network", "Triage": "network", "Doctor": "network"},
	"os": {"ReadFile": "files", "WriteFile": "files", "Open": "files", "OpenFile": "files", "Create": "files", "ReadDir": "files", "Stat": "files", "Lstat": "files",
		"Mkdir": "files", "MkdirAll": "files", "MkdirTemp": "files", "CreateTemp": "files", "Remove": "files", "RemoveAll": "files", "Rename": "files",
		"Chmod": "files", "Truncate": "files", "Readlink": "files", "Symlink": "files"},
	"path/filepath": {"Walk": "files", "WalkDir": "files", "Glob": "files", "EvalSymlinks": "files"},
	"net":           {"Dial": "network", "DialTimeout": "network", "Listen": "network", "LookupHost": "network", "LookupIP": "network", "LookupAddr": "network", "ListenPacket": "network", "ListenUDP": "network", "DialUDP": "network", "DialTCP": "network", "ResolveTCPAddr": "network"},
}

// notFollowed are method names that are, in this package, nearly always the standard library's (a Body or a connection being closed) and are also
// the name of one of the package's own methods (the automation server's Close removes its discovery file): following them would charge every method
// that closes a response with a file it never touches. The automation server's Close is not reachable from a bound method.
// deviceNames are Studio's own entry points to a device that no import shows: the agent client of the connection (a value, called through its
// methods) and the widget tree read.
var deviceNames = map[string]bool{"clientOf": true, "GetWidgetTree": true}

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
	srcs := map[string]string{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		srcs[name] = string(raw)
	}
	return parseSources(srcs)
}

// parseSources builds the call graph of the given files (name to text).
func parseSources(srcs map[string]string) (*pkgSources, error) {
	ps := &pkgSources{funcs: map[string][]*ast.FuncDecl{}, imports: map[*ast.FuncDecl]map[string]string{}}
	fset := token.NewFileSet()
	var names []string
	for n := range srcs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, srcs[name], 0)
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
		if deviceNames[n] && touched["devices"] == "" {
			touched["devices"] = strings.Join(chain, " -> ")
		}
		for _, fn := range ps.funcs[n] {
			imps := ps.imports[fn]
			selNames := map[*ast.Ident]bool{} // the name after a dot is followed by the SelectorExpr case below, not again as a bare name
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				switch x := node.(type) {
				case *ast.SelectorExpr:
					selNames[x.Sel] = true
					// any use of a sink package's name counts, called or not: a function held in a variable (`runDoctor = ai.Doctor`), a client held in a
					// package variable (`http.DefaultClient.Do`) and a value of the package whose method is called later are all ways to reach it
					if id, ok := x.X.(*ast.Ident); ok {
						if path, isPkg := imps[id.Name]; isPkg {
							for _, t := range []string{sinkPackages[path], sinkFuncs[path][x.Sel.Name]} {
								if t != "" && touched[t] == "" {
									touched[t] = strings.Join(chain, " -> ") + " -> " + id.Name + "." + x.Sel.Name
								}
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
	if got := ps.reach("Chat"); !has(got, "keychain") || !has(got, "files") || !has(got, "network") || !has(got, "devices") {
		t.Errorf("the walk from Chat found %v, want files, keychain, network and devices (the widget tree is read from the device)", got)
	}
	if got := ps.reach("TestAIConnection"); !has(got, "network") {
		t.Errorf("TestAIConnection reaches the provider through a function variable (runDoctor = ai.Doctor) and the walk found %v", got)
	}
	if got := ps.reach("ExportRunResults"); has(got, "devices") {
		t.Errorf("a pure runner helper was taken for a device: %v", got)
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

// Every method that is recorded as "asks" is one the code is known to ask for: a person's answer in the window is required in Go, and no window
// means no. Which question each one goes through is checked here, so a label cannot be "asks" without a gate behind it, and a gate cannot lose its
// label (the run methods are checked above, through the connection's device context).
func TestEveryAsksEntryHasAGateInTheCode(t *testing.T) {
	gates := map[string]string{"ConnectWiFiSaved": "confirmSendSavedToken", "RunFile": "deviceCtx", "RunFileAsync": "deviceCtx", "RunFileScoped": "deviceCtx"}
	ps, err := loadPackageSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range boundDecisions {
		gate, known := gates[d.Method]
		switch {
		case d.Confirm == "asks" && !known:
			t.Errorf("%s is recorded as asks and no gate in the code is known for it: add it to the gates of this test with the name of its question, or label it previewed or none with a decision", d.Method)
		case known && d.Confirm != "asks":
			t.Errorf("%s has a gate (%s) and is recorded as %q: the label must say asks", d.Method, gate, d.Confirm)
		case known && gate == "deviceCtx" && !(ps.mentions(d.Method, "executeRun") || d.Method == "executeRun"):
			t.Errorf("%s no longer runs through executeRun, where the run is bound to its confirmations", d.Method)
		case known && !ps.mentions(d.Method, gate):
			t.Errorf("%s no longer reaches %s: the question that makes it asks", d.Method, gate)
		case d.Method == "ConnectWiFiSaved" && !ps.refusesUnless(d.Method, gate):
			t.Errorf("%s does not stop when %s says no: `if !%s(...) { return ... }` is the gate, and a mention of the name is not", d.Method, gate, gate)
		}
	}
}

// refusesUnless says whether fn has `if !<x>.callee(...) { ... return ... }` as a whole condition: the question's answer decides, and a no returns.
func (ps *pkgSources) refusesUnless(fn, callee string) bool {
	for _, d := range ps.funcs[fn] {
		found := false
		ast.Inspect(d.Body, func(n ast.Node) bool {
			is, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}
			u, ok := is.Cond.(*ast.UnaryExpr)
			if !ok || u.Op != token.NOT {
				return true
			}
			call, ok := u.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != callee {
				return true
			}
			ast.Inspect(is.Body, func(b ast.Node) bool {
				if _, ok := b.(*ast.ReturnStmt); ok {
					found = true
				}
				return true
			})
			return true
		})
		if found {
			return true
		}
	}
	return false
}

// The walk is tried on small sources of its own: each way of reaching a sink that the review found slipping is a case, and so are the things that
// must not count.
func TestTheWalkSeesEachWayToReachASink(t *testing.T) {
	src := func(body string) *pkgSources {
		ps, err := parseSources(map[string]string{"x.go": "package main\nimport (\n\"net/http\"\n\"os\"\n\"github.com/gorilla/websocket\"\n\"github.com/zalando/go-keyring\"\n\"github.com/alphawavesystems/flutter-probe/internal/ai\"\n\"github.com/alphawavesystems/flutter-probe/internal/httpcall\"\n)\n" + body})
		if err != nil {
			t.Fatal(err)
		}
		return ps
	}
	for name, c := range map[string]struct{ code, want string }{
		"a package client":                  {"func A() { _, _ = http.DefaultClient.Do(nil) }", "network"},
		"a client built in place":           {"func A() { _, _ = (&http.Client{}).Do(nil) }", "network"},
		"a sink held in a local variable":   {"func A() { f := os.Remove; _ = f(\"x\") }", "files"},
		"a sink held in a package variable": {"var zzD = ai.Triage\nfunc A() { _ = zzD }", "network"},
		"a sink assigned to a function var": {"var runDoctor = ai.Doctor\nfunc A() { _ = runDoctor }", "network"},
		"the websocket dialer":              {"func A() { _, _, _ = websocket.DefaultDialer.Dial(\"x\", nil) }", "devices"},
		"the keychain":                      {"func A() { _, _ = keyring.Get(\"a\", \"b\") }", "keychain"},
		"a sink through two functions":      {"func A() { b() }\nfunc b() { c() }\nfunc c() { _ = os.Remove(\"x\") }", "files"},
		"the request helper":                {"func A() { _, _ = httpcall.Do(nil, httpcall.Request{}, httpcall.Options{}) }", "network"},
	} {
		if got := src(c.code).reach("A"); !has(got, c.want) {
			t.Errorf("%s: the walk found %v, want %s", name, got, c.want)
		}
	}
	for name, code := range map[string]string{
		"a constant of the request helper": "func A() { _ = httpcall.MaxURLBytes }",
		"a type of the standard library":   "func A() { var _ os.FileMode }",
		"a function nobody calls":          "func A() {}\nfunc b() { _ = os.Remove(\"x\") }",
	} {
		if got := src(code).reach("A"); len(got) != 0 {
			t.Errorf("%s: the walk found %v, want nothing", name, got)
		}
	}
}

// A run asks for a clear step through the connection's device context, and two tests show it with a real App: a clear reaches the device only on a
// yes (nothing answering, a no, or a context of another connection is a no), and executeRun binds the confirmations to the run it starts. The guard
// does not run them again; it fails when either is deleted or renamed, or when executeRun stops binding the run, so the label "asks" on the run
// methods cannot outlive the evidence for it.
func TestTheRunsAskLabelHasItsEvidence(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "confirm_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			have[fn.Name.Name] = true
		}
	}
	for _, name := range []string{"TestStudioClearAppDataReachesTheDeviceOnlyOnAYes", "TestExecuteRunSetsTheRunItsConfirmationsBelongTo", "TestConfirmWithNobodyToAskDenies"} {
		if !have[name] {
			t.Errorf("%s is gone from confirm_test.go: it is the evidence that a run's clear step asks", name)
		}
	}
	ps, err := loadPackageSources()
	if err != nil {
		t.Fatal(err)
	}
	if !ps.mentions("executeRun", "setConfirmRun") {
		t.Error("executeRun no longer binds the confirmations to its run (setConfirmRun): a question would be asked for no run")
	}
	if !ps.mentions("executeRun", "deviceCtx") {
		t.Error("executeRun no longer runs with the connection's device context")
	}
}
