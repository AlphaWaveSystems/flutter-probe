package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every exported method of App is callable from the window (Wails binds them all), and in the headless build from any page script that
// has the bridge token (headless.go calls a method by its name through reflection). So adding a method to App is a decision about what a
// page script may do, and it is made here, in a table, with a category: a method that is not in the table fails this test, and so does a
// table entry for a method that no longer exists. The automation endpoint is a smaller, separate surface (an interface of a few
// methods plus a switch of method names): each is an explicit set too, and the categories that may never reach it are named.

const (
	catUI            = "ui"             // pure: reads and changes nothing outside memory
	catReadsWS       = "readsWorkspace" // reads a file or folder inside the workspace
	catWritesWS      = "writesWorkspace"
	catWorkspaceRoot = "workspaceRoot" // chooses which folder reads and writes are confined to
	catSecrets       = "secrets"       // the keychain: API keys and WiFi tokens
	catNetwork       = "network"       // a lookup or a request to a host a person typed or the network announced
	catAIProvider    = "aiProvider"    // text sent to, or read from, an AI provider
	catDevice        = "device"        // talks to a connected device or its agent
	catRunControl    = "runControl"    // starts or stops a run or a recording
	catUserPrefs     = "userPrefs"     // the person's own Studio preferences on this computer (not the workspace): only the window may change them
	catWritesOutside = "writesOutside" // writes a file outside the workspace, to a path a person picked in a native dialog
	catConfirm       = "confirm"       // answers a question the window asked about a destructive step: only a person in the window may
)

// boundDecision is one entry of bound_methods.json: what the method does, who may call it and what it touches.
type boundDecision struct {
	Method   string   `json:"method"`
	Category string   `json:"category"`
	Does     string   `json:"does"`
	Callers  []string `json:"callers"`
	Touches  []string `json:"touches"`
	Confirm  string   `json:"confirm"`
	// Decision says why an aiProvider method with confirm "none" needs no confirmation, or what the window does instead.
	Decision string `json:"decision"`
}

// boundDecisions are the recorded decisions, read from bound_methods.json (the one place they are written), and boundMethods is their
// method-to-category view that the tests below use.
var boundDecisions, boundMethods = loadBoundDecisions()

// automationInternal are the unexported methods the automation endpoint reaches: not bound, but what a script that holds its token can do.
var automationInternal = loadAutomationInternal()

func loadAutomationInternal() []boundDecision {
	raw, err := os.ReadFile("bound_methods.json")
	if err != nil {
		panic("bound_methods.json: " + err.Error())
	}
	var f struct {
		Internal []boundDecision `json:"automationInternal"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		panic("bound_methods.json is not valid: " + err.Error())
	}
	return f.Internal
}

func loadBoundDecisions() ([]boundDecision, map[string]string) {
	raw, err := os.ReadFile("bound_methods.json")
	if err != nil {
		panic("bound_methods.json: " + err.Error())
	}
	var f struct {
		Methods []boundDecision `json:"methods"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		panic("bound_methods.json is not valid: " + err.Error())
	}
	cats := map[string]string{}
	for _, d := range f.Methods {
		cats[d.Method] = d.Category
	}
	return f.Methods, cats
}

// bridgeBlocked are the methods the headless bridge refuses by name (headless.go methodsNotOverHTTP): they need a native window.
var bridgeBlocked = []string{"AnswerConfirm", "ExportRunResults", "PickWorkspace", "SetClearDataWithoutAsking"}

// automationRPC are the method names the automation endpoint accepts, and automationBackendMethods the methods of App it can reach.
var automationRPC = []string{"cancel", "connect", "connect_wifi", "disconnect", "list_devices", "list_dir", "open_workspace", "read_file", "results", "run_file", "run_state", "screenshot", "status", "test_wifi", "ui_state", "widget_tree"}
var automationBackendMethods = []string{"CancelRun", "Connect", "ConnectWiFi", "Disconnect", "GetWidgetTree", "ListDevices", "ListDir", "ReadFile", "RunState", "Status", "TakeScreenshot", "TestWiFi", "automationRoots", "runFileAsync", "useWorkspace"}

// automationNever are the categories no automation method may belong to: a token for the automation endpoint grants what its methods do,
// and none of these is something a script that drives a device should be able to do.
var automationNever = map[string]bool{catConfirm: true, catWritesWS: true, catSecrets: true, catNetwork: true, catAIProvider: true, catWorkspaceRoot: true, catWritesOutside: true, catUserPrefs: true}

func exportedMethods(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumMethod(); i++ {
		out = append(out, t.Method(i).Name)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func diff(a, b []string) (onlyA, onlyB []string) {
	in := func(s []string, v string) bool {
		for _, x := range s {
			if x == v {
				return true
			}
		}
		return false
	}
	for _, v := range a {
		if !in(b, v) {
			onlyA = append(onlyA, v)
		}
	}
	for _, v := range b {
		if !in(a, v) {
			onlyB = append(onlyB, v)
		}
	}
	return
}

// Every exported method of App has a decision, and every decision is for a method that exists.
func TestEveryBoundMethodHasADecision(t *testing.T) {
	have := exportedMethods(reflect.TypeOf(&App{}))
	if undecided, stale := diff(have, sortedKeys(boundMethods)); len(undecided) > 0 || len(stale) > 0 {
		t.Errorf("a new exported App method is callable from the window and, in the headless build, from any page script with the bridge token.\n"+
			"  methods with no entry in boundMethods (add one, with its category): %v\n  entries for methods that do not exist: %v", undecided, stale)
	}
	for name, cat := range boundMethods {
		switch cat {
		case catUI, catReadsWS, catWritesWS, catWorkspaceRoot, catSecrets, catNetwork, catAIProvider, catDevice, catRunControl, catConfirm, catWritesOutside, catUserPrefs:
		default:
			t.Errorf("%s has the unknown category %q", name, cat)
		}
	}
	// Methods promoted from an embedded type are bound and callable too: App declares all of its own.
	ty := reflect.TypeOf(App{})
	for i := 0; i < ty.NumField(); i++ {
		if ty.Field(i).Anonymous {
			t.Errorf("App embeds %s: its exported methods would be bound without an entry here", ty.Field(i).Type)
		}
	}
}

// The window's generated bindings are the same set: a method added without regenerating them is a method the window cannot call, and one
// removed without regenerating is a call that fails at run time.
func TestTheGeneratedBindingsAreTheSameSet(t *testing.T) {
	src, err := os.ReadFile("frontend/wailsjs/go/main/App.d.ts")
	if err != nil {
		t.Fatal(err)
	}
	var bound []string
	for _, m := range regexp.MustCompile(`(?m)^export function (\w+)\(`).FindAllStringSubmatch(string(src), -1) {
		bound = append(bound, m[1])
	}
	sort.Strings(bound)
	if a, b := diff(bound, exportedMethods(reflect.TypeOf(&App{}))); len(a) > 0 || len(b) > 0 {
		t.Errorf("wailsjs bindings only: %v; App methods without a binding: %v (run `wails generate module`)", a, b)
	}
}

// The headless bridge refuses what needs a window, and nothing else.
func TestTheHeadlessBridgeBlocksExactlyTheWindowOnlyMethods(t *testing.T) {
	var got []string
	for k := range methodsNotOverHTTP {
		got = append(got, k)
	}
	sort.Strings(got)
	want := append([]string(nil), bridgeBlocked...)
	sort.Strings(want)
	if a, b := diff(got, want); len(a) > 0 || len(b) > 0 {
		t.Errorf("methodsNotOverHTTP differs from the decision: only in headless.go %v, only here %v", a, b)
	}
	for _, name := range bridgeBlocked {
		if _, err := callAppMethod(&App{}, name, nil); err == nil {
			t.Errorf("%s was called over the bridge", name)
		}
	}
	// A name that is not an exported method of App is not called, whatever it is spelled like.
	for _, name := range []string{"", "nope", "startup", "useWorkspace", "automationRoots", "runFileAsync", "app", "Lint ", " Lint", "lint"} {
		if _, err := callAppMethod(&App{}, name, nil); err == nil {
			t.Errorf("%q was called over the bridge", name)
		}
	}
}

// The automation endpoint accepts a fixed list of names and reaches a fixed list of methods, and none of them is in a category that
// must never be reachable from it.
func TestTheAutomationEndpointReachesAnExplicitSet(t *testing.T) {
	// The names in the switch of call(), read from the source.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "automation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "call" || fn.Recv == nil {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						s, _ := strconv.Unquote(lit.Value)
						names = append(names, s)
					}
				}
			}
			return true
		})
		return false
	})
	sort.Strings(names)
	if a, b := diff(names, automationRPC); len(a) > 0 || len(b) > 0 {
		t.Errorf("automation method names: only in automation.go %v, only in the decision %v", a, b)
	}
	// The methods of App that the endpoint's backend interface can reach.
	iface := reflect.TypeOf((*automationBackend)(nil)).Elem()
	var reach []string
	for i := 0; i < iface.NumMethod(); i++ {
		reach = append(reach, iface.Method(i).Name)
	}
	sort.Strings(reach)
	if a, b := diff(reach, automationBackendMethods); len(a) > 0 || len(b) > 0 {
		t.Errorf("automation backend methods: only in the interface %v, only in the decision %v", a, b)
	}
	for _, m := range reach {
		if m[0] < 'A' || m[0] > 'Z' {
			continue // unexported helpers (roots, workspace choice by the endpoint's own rules)
		}
		cat, ok := boundMethods[m]
		if !ok {
			t.Errorf("%s is reachable from automation and has no category", m)
		}
		if automationNever[cat] {
			t.Errorf("%s is %s and reachable from automation", m, cat)
		}
	}
	if !reflect.TypeOf(&App{}).Implements(iface) {
		t.Error("App no longer satisfies the automation backend")
	}
}

// The automation endpoint cannot answer a confirmation: it refuses every spelling by name, before any backend method is reached, and the
// method is in a category and outside the lists that keep it out of the endpoint's reach.
type untouchableBackend struct{ automationBackend } // every method panics if it is called: a refused name must never reach the backend

func TestAutomationEndpointCannotAnswerAConfirmation(t *testing.T) {
	if cat := boundMethods["AnswerConfirm"]; cat != catConfirm || !automationNever[cat] {
		t.Errorf("AnswerConfirm is %q, which must be a category no automation method may have", cat)
	}
	s := &automationServer{backend: untouchableBackend{}}
	for _, name := range []string{"answer_confirm", "AnswerConfirm", "confirm", "answerConfirm", "confirm:answer"} {
		_, err := s.call(name, []byte(`{"id":"x","allow":true}`))
		if err == nil || err.Error() != fmt.Sprintf("unknown method %q", name) {
			t.Errorf("the endpoint's answer to %q = %v, want the plain unknown-method refusal", name, err)
		}
	}
	for _, m := range automationBackendMethods {
		if m == "AnswerConfirm" {
			t.Error("AnswerConfirm is reachable from the automation backend")
		}
	}
	for _, m := range automationRPC {
		if strings.Contains(strings.ToLower(m), "confirm") {
			t.Errorf("the automation endpoint accepts %q", m)
		}
	}
}

// skeleton is the entry to paste into bound_methods.json for a method that has none.
func skeleton(m string) string {
	return fmt.Sprintf(`{"method": %q, "category": "<one of the categories in bound_methods_test.go>", "does": "<one sentence: what it does>", `+
		`"callers": ["window", "bridge", "automation"], "touches": ["files", "devices", "network", "keychain"], "confirm": "none|asks|answers|native-dialog"}`, m)
}

var (
	validCallers = map[string]bool{"window": true, "bridge": true, "automation": true}
	validTouches = map[string]bool{"files": true, "devices": true, "network": true, "keychain": true}
	validConfirm = map[string]bool{"none": true, "asks": true, "answers": true, "native-dialog": true}
)

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// checkDecisions returns what is wrong with a set of decisions, given the methods the code has and the facts the code states about
// who can call them. It is a function so that the test can be shown to fail on each kind of mistake.
func checkDecisions(decisions []boundDecision, have, blocked, automation []string) []string {
	var bad []string
	seen := map[string]bool{}
	for _, d := range decisions {
		if seen[d.Method] {
			bad = append(bad, d.Method+": two entries")
		}
		seen[d.Method] = true
	}
	var names []string
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	undecided, stale := diff(have, names)
	for _, m := range undecided {
		bad = append(bad, fmt.Sprintf("%s: an exported App method with no entry in studio/bound_methods.json. Every App method is callable from the window and, in the headless build, from a page script with the bridge token: add\n    %s", m, skeleton(m)))
	}
	for _, m := range stale {
		bad = append(bad, m+": an entry in studio/bound_methods.json for a method that does not exist (renamed or removed): delete or rename the entry")
	}
	for _, d := range decisions {
		if !haveCat(d.Category) {
			bad = append(bad, d.Method+": unknown category "+strconv.Quote(d.Category))
		}
		if len(strings.TrimSpace(d.Does)) < 10 {
			bad = append(bad, d.Method+": 'does' must say what the method does")
		}
		if !validConfirm[d.Confirm] {
			bad = append(bad, d.Method+": confirm must be none, asks, answers or native-dialog")
		}
		if !has(d.Callers, "window") {
			bad = append(bad, d.Method+": every bound method can be called from the window; callers must include it")
		}
		for _, c := range d.Callers {
			if !validCallers[c] {
				bad = append(bad, d.Method+": unknown caller "+strconv.Quote(c))
			}
		}
		for _, x := range d.Touches {
			if !validTouches[x] {
				bad = append(bad, d.Method+": unknown touches value "+strconv.Quote(x))
			}
		}
		// the entry says what the code says
		if has(d.Callers, "bridge") == has(blocked, d.Method) {
			bad = append(bad, d.Method+": callers disagrees with the headless bridge's block list (headless.go): a blocked method must not list 'bridge', every other one must")
		}
		if has(d.Callers, "automation") != has(automation, d.Method) {
			bad = append(bad, d.Method+": callers disagrees with the methods the automation endpoint reaches")
		}
		if has(d.Callers, "automation") && has(d.Touches, "keychain") {
			bad = append(bad, d.Method+": the automation endpoint must not reach the keychain")
		}
		// the category says what it touches
		switch d.Category {
		case catSecrets:
			if !has(d.Touches, "keychain") {
				bad = append(bad, d.Method+": a secrets method touches the keychain")
			}
		case catNetwork, catAIProvider:
			if !has(d.Touches, "network") && d.Method != "PreviewAIChat" && d.Method != "StopWiFiDiscovery" {
				bad = append(bad, d.Method+": a network or AI-provider method touches the network")
			}
		case catDevice, catRunControl:
			if !has(d.Touches, "devices") && d.Method != "DeviceListNote" {
				bad = append(bad, d.Method+": a device or run-control method touches devices")
			}
		case catWritesWS, catWritesOutside, catUserPrefs:
			if !has(d.Touches, "files") {
				bad = append(bad, d.Method+": a method that writes touches files")
			}
		case catConfirm:
			if d.Confirm != "answers" {
				bad = append(bad, d.Method+": a confirm method answers a confirmation")
			}
		case catUI:
			if len(d.Touches) > 0 {
				bad = append(bad, d.Method+": a pure method touches nothing")
			}
		}
		if d.Category == catAIProvider && d.Confirm == "none" && len(strings.TrimSpace(d.Decision)) < 40 {
			bad = append(bad, d.Method+": an AI-provider method with confirm none needs a 'decision' that says why (or what the window does instead)")
		}
		// what may destroy or send is gated, or only a person in a native dialog can drive it
		if (d.Category == catRunControl && (strings.HasPrefix(d.Method, "RunFile"))) && d.Confirm != "asks" {
			bad = append(bad, d.Method+": a run can reach `clear app data`: confirm must be 'asks'")
		}
		if d.Category == catUserPrefs && d.Method != "GetStudioPrefs" && d.Confirm != "native-dialog" {
			bad = append(bad, d.Method+": a preference that answers destructive questions is changed by a person in the window only: confirm must be native-dialog")
		}
	}
	return bad
}

func haveCat(c string) bool {
	switch c {
	case catUI, catReadsWS, catWritesWS, catWorkspaceRoot, catSecrets, catNetwork, catAIProvider, catDevice, catRunControl, catUserPrefs, catWritesOutside, catConfirm:
		return true
	}
	return false
}

// bound_methods.json records, for every exported method of App, what it does, who may call it, what it touches and whether it needs a
// confirmation, and the entry must agree with the code. A new method without an entry fails here, with the entry to add.
func TestEveryBoundMethodIsRecordedInTheAllowlistFile(t *testing.T) {
	have := exportedMethods(reflect.TypeOf(&App{}))
	for _, msg := range checkDecisions(boundDecisions, have, bridgeBlocked, automationBackendMethods) {
		t.Error(msg)
	}
}

// The check itself fails on each kind of mistake (the mutants of the guard, run as data).
func TestTheAllowlistCheckFailsOnEachKindOfMistake(t *testing.T) {
	have := exportedMethods(reflect.TypeOf(&App{}))
	clone := func() []boundDecision {
		out := make([]boundDecision, len(boundDecisions))
		copy(out, boundDecisions)
		return out
	}
	without := func(name string) []boundDecision {
		var out []boundDecision
		for _, d := range boundDecisions {
			if d.Method != name {
				out = append(out, d)
			}
		}
		return out
	}
	if bad := checkDecisions(boundDecisions, have, bridgeBlocked, automationBackendMethods); len(bad) != 0 {
		t.Fatalf("the real file is not clean: %v", bad)
	}
	contains := func(bad []string, want string) bool {
		for _, b := range bad {
			if strings.Contains(b, want) {
				return true
			}
		}
		return false
	}
	// a new method with no entry names the method and the entry to add
	if bad := checkDecisions(boundDecisions, append(append([]string{}, have...), "BrandNewMethod"), bridgeBlocked, automationBackendMethods); !contains(bad, "BrandNewMethod: an exported App method with no entry") || !contains(bad, `"method": "BrandNewMethod"`) {
		t.Errorf("a new method was not caught, or the message does not say what to add: %v", bad)
	}
	// an entry removed
	if bad := checkDecisions(without("WriteFile"), have, bridgeBlocked, automationBackendMethods); !contains(bad, "WriteFile: an exported App method with no entry") {
		t.Errorf("a removed entry was not caught: %v", bad)
	}
	// a stale entry for a method that was removed
	stale := append(clone(), boundDecision{Method: "GoneMethod", Category: catUI, Does: "A method that was removed.", Callers: []string{"window", "bridge"}, Confirm: "none"})
	if bad := checkDecisions(stale, have, bridgeBlocked, automationBackendMethods); !contains(bad, "GoneMethod: an entry in studio/bound_methods.json for a method that does not exist") {
		t.Errorf("a stale entry was not caught: %v", bad)
	}
	// two entries, an empty does, a wrong caller list, a wrong touches, a missing confirm
	dup := append(clone(), boundDecisions[0])
	if bad := checkDecisions(dup, have, bridgeBlocked, automationBackendMethods); !contains(bad, "two entries") {
		t.Errorf("a duplicate was not caught: %v", bad)
	}
	edit := func(name string, f func(d *boundDecision)) []boundDecision {
		out := clone()
		for i := range out {
			if out[i].Method == name {
				f(&out[i])
			}
		}
		return out
	}
	for label, c := range map[string]struct {
		in   []boundDecision
		want string
	}{
		"empty does":                {edit("Lint", func(d *boundDecision) { d.Does = "" }), "'does' must say"},
		"bridge listed but blocked": {edit("AnswerConfirm", func(d *boundDecision) { d.Callers = append(d.Callers, "bridge") }), "headless bridge's block list"},
		"bridge missing":            {edit("Lint", func(d *boundDecision) { d.Callers = []string{"window"} }), "headless bridge's block list"},
		"automation reach hidden":   {edit("Connect", func(d *boundDecision) { d.Callers = []string{"window", "bridge"} }), "automation endpoint reaches"},
		"automation reach added":    {edit("WriteFile", func(d *boundDecision) { d.Callers = append(d.Callers, "automation") }), "automation endpoint reaches"},
		"secret without keychain":   {edit("SetAPIKey", func(d *boundDecision) { d.Touches = nil }), "touches the keychain"},
		"pure method touching":      {edit("Lint", func(d *boundDecision) { d.Touches = []string{"files"} }), "touches nothing"},
		"run without ask":           {edit("RunFile", func(d *boundDecision) { d.Confirm = "none" }), "confirm must be 'asks'"},
		"pref without native":       {edit("SetClearDataWithoutAsking", func(d *boundDecision) { d.Confirm = "none" }), "native-dialog"},
		"unknown category":          {edit("Lint", func(d *boundDecision) { d.Category = "misc" }), "unknown category"},
		"window not a caller":       {edit("Lint", func(d *boundDecision) { d.Callers = []string{"bridge"} }), "callers must include it"},
	} {
		if bad := checkDecisions(c.in, have, bridgeBlocked, automationBackendMethods); !contains(bad, c.want) {
			t.Errorf("%s: not caught (want %q): %v", label, c.want, bad)
		}
	}
}
