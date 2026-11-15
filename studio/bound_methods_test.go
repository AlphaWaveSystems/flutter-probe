package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
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
)

var boundMethods = map[string]string{
	// pure
	"AIChatLabel": catUI, "AIChatLabelParts": catUI, "ExplainAvailability": catUI, "InitialWorkspace": catUI, "Lint": catUI, "RunState": catUI,
	"Status": catUI, "BuildCallStep": catUI,
	// the workspace
	"ListDir": catReadsWS, "ReadFile": catReadsWS, "LoadAISettings": catReadsWS, "LoadWorkspaceSettings": catReadsWS,
	"WriteFile": catWritesWS, "SaveAISettings": catWritesWS, "SaveWorkspaceSettings": catWritesWS,
	"SetWorkspace": catWorkspaceRoot, "PickWorkspace": catWorkspaceRoot,
	// the keychain
	"SetAPIKey": catSecrets, "GetAPIKey": catSecrets, "DeleteAPIKey": catSecrets, "SaveWiFiToken": catSecrets, "ForgetWiFiToken": catSecrets, "WiFiTokenSaved": catSecrets,
	// the network
	"AllowHost": catNetwork, "CheckTarget": catNetwork, "SendRequest": catNetwork, "FindLocalModels": catNetwork,
	"StartWiFiDiscovery": catNetwork, "StopWiFiDiscovery": catNetwork,
	// an AI provider
	"Chat": catAIProvider, "PreviewAIChat": catAIProvider, "ExplainLastRun": catAIProvider, "ExplainPreviewed": catAIProvider, "PreviewExplain": catAIProvider,
	"ListAIModels": catAIProvider, "TestAIConnection": catAIProvider, "PreviewSelectorFix": catAIProvider, "SelectorFixPreviewed": catAIProvider,
	// a device
	"Connect": catDevice, "ConnectWiFi": catDevice, "ConnectWiFiSaved": catDevice, "Disconnect": catDevice, "ListDevices": catDevice, "TestWiFi": catDevice,
	"ApplyDeviceSetting": catDevice, "GetDeviceSettings": catDevice, "GetDeviceView": catDevice, "GetWidgetTree": catDevice, "TakeScreenshot": catDevice,
	"HTTPExchange": catDevice, "HTTPExchanges": catDevice,
	// runs and recordings
	"RunFile": catRunControl, "RunFileAsync": catRunControl, "RunFileScoped": catRunControl, "CancelRun": catRunControl, "StartRecording": catRunControl, "StopRecording": catRunControl,
}

// bridgeBlocked are the methods the headless bridge refuses by name (headless.go methodsNotOverHTTP): they need a native window.
var bridgeBlocked = []string{"PickWorkspace"}

// automationRPC are the method names the automation endpoint accepts, and automationBackendMethods the methods of App it can reach.
var automationRPC = []string{"cancel", "connect", "connect_wifi", "disconnect", "list_devices", "list_dir", "open_workspace", "read_file", "results", "run_file", "run_state", "screenshot", "status", "test_wifi", "ui_state", "widget_tree"}
var automationBackendMethods = []string{"CancelRun", "Connect", "ConnectWiFi", "Disconnect", "GetWidgetTree", "ListDevices", "ListDir", "ReadFile", "RunState", "Status", "TakeScreenshot", "TestWiFi", "automationRoots", "runFileAsync", "useWorkspace"}

// automationNever are the categories no automation method may belong to: a token for the automation endpoint grants what its methods do,
// and none of these is something a script that drives a device should be able to do.
var automationNever = map[string]bool{catWritesWS: true, catSecrets: true, catNetwork: true, catAIProvider: true, catWorkspaceRoot: true}

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
		case catUI, catReadsWS, catWritesWS, catWorkspaceRoot, catSecrets, catNetwork, catAIProvider, catDevice, catRunControl:
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
