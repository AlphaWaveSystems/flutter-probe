package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	pp "github.com/alphawavesystems/flutter-probe/internal/parser"
)

// Replace the line may only produce, and only replace, the steps on a closed list (fixVerbs, fixWaits, and the kinds in fixProfile). Every
// action verb, wait kind and step type the parser has is named here as allowed or denied, so a new one fails this test until someone
// decides, on purpose, which it is.

var allowedVerbConsts = []string{"VerbTap", "VerbType", "VerbLongPress", "VerbDoubleTap"}

var deniedVerbConsts = []string{
	"VerbOpen", "VerbSwipe", "VerbScroll", "VerbGoBack", "VerbPressEnter", "VerbClear", "VerbClose", "VerbDrag", "VerbPinch", "VerbRotate", "VerbToggle",
	"VerbShake", "VerbPress", "VerbPressKey", "VerbTakeShot", "VerbCompareShot", "VerbDumpTree", "VerbSaveLogs", "VerbPause", "VerbLog", "VerbRestart",
	"VerbClearAppData", "VerbAllowPermission", "VerbDenyPermission", "VerbGrantAllPerms", "VerbRevokeAllPerms", "VerbKill", "VerbCopyClipboard",
	"VerbPasteClipboard", "VerbSetLocation", "VerbSetLanguage", "VerbSetAppearance", "VerbVerifyBrowser", "VerbSetAirplaneMode", "VerbOpenLink", "VerbStore",
	"VerbEnrollBiometric", "VerbBiometricMatch", "VerbBiometricNoMatch", "VerbDeliverSignal", "VerbReadWithAI", "VerbAddMedia", "VerbTapNative",
	"VerbTypeNative", "VerbTapWebView", "VerbTypeWebView", "VerbStoreWebView",
}

var allowedWaitConsts = []string{"WaitAppears", "WaitDisappears", "WaitSelector", "WaitAny"}

var deniedWaitConsts = []string{"WaitDuration", "WaitPageLoad", "WaitNetworkIdle", "WaitAnimations", "WaitIdle", "WaitWebView"}

var allowedStepTypes = []string{"ActionStep", "AssertStep", "WaitStep"}

var deniedStepTypes = []string{
	"SystemDialogStep", "AssertNoDefectsStep", "ConditionalStep", "LoopStep", "RetryStep", "TravelStep", "DartBlock", "PerfStep", "TimedStep", "HTTPStep",
	"MockBlock", "RecipeCall", "DeviceStep", "SyncStep", "HTTPCallStep",
}

// astNames reads ast.go: the constants of a type, and the struct types that have a stepType method.
func astNames(t *testing.T) (consts map[string][]string, steps []string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../internal/parser/ast.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	consts = map[string][]string{}
	hasStepType := map[string]bool{}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			if d.Tok != token.CONST {
				continue
			}
			typ := ""
			for _, sp := range d.Specs {
				vs := sp.(*ast.ValueSpec)
				if id, ok := vs.Type.(*ast.Ident); ok {
					typ = id.Name
				}
				for _, n := range vs.Names {
					consts[typ] = append(consts[typ], n.Name)
				}
			}
		case *ast.FuncDecl:
			if d.Name.Name == "stepType" && d.Recv != nil && len(d.Recv.List) == 1 {
				if id, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					hasStepType[id.Name] = true
				}
			}
		}
	}
	for n := range hasStepType {
		steps = append(steps, n)
	}
	sort.Strings(steps)
	return consts, steps
}

func classified(t *testing.T, what string, found, allowed, denied []string) {
	t.Helper()
	known := map[string]string{}
	for _, n := range allowed {
		known[n] = "allowed"
	}
	for _, n := range denied {
		if known[n] != "" {
			t.Errorf("%s %s is listed as allowed and denied", what, n)
		}
		known[n] = "denied"
	}
	have := map[string]bool{}
	for _, n := range found {
		have[n] = true
		if known[n] == "" {
			t.Errorf("the parser has %s %s, which Replace the line has not classified: add it to the allowed or the denied list in replace_line_classify_test.go, and to fixVerbs, fixWaits or fixProfile if it is allowed", what, n)
		}
	}
	for n := range known {
		if !have[n] {
			t.Errorf("%s %s is classified but the parser no longer has it", what, n)
		}
	}
}

func TestEveryParserVerbWaitAndStepIsClassified(t *testing.T) {
	consts, steps := astNames(t)
	classified(t, "action verb", consts["ActionVerb"], allowedVerbConsts, deniedVerbConsts)
	// WaitKind's constants are an iota block: the first has the type, the rest follow it
	classified(t, "wait kind", consts["WaitKind"], allowedWaitConsts, deniedWaitConsts)
	var stepTypes []string
	for _, s := range steps {
		stepTypes = append(stepTypes, s)
	}
	classified(t, "step type", stepTypes, allowedStepTypes, deniedStepTypes)
}

// What the code allows is exactly what the test says is allowed.
func TestTheClosedListMatchesTheClassification(t *testing.T) {
	verbs := map[string]pp.ActionVerb{
		"VerbTap": pp.VerbTap, "VerbType": pp.VerbType, "VerbLongPress": pp.VerbLongPress, "VerbDoubleTap": pp.VerbDoubleTap,
	}
	if len(fixVerbs) != len(allowedVerbConsts) {
		t.Errorf("fixVerbs has %d verbs, the test lists %d", len(fixVerbs), len(allowedVerbConsts))
	}
	for _, n := range allowedVerbConsts {
		if !fixVerbs[verbs[n]] {
			t.Errorf("%s is listed as allowed and is not in fixVerbs", n)
		}
	}
	waits := map[string]pp.WaitKind{"WaitAppears": pp.WaitAppears, "WaitDisappears": pp.WaitDisappears, "WaitSelector": pp.WaitSelector, "WaitAny": pp.WaitAny}
	if len(fixWaits) != len(allowedWaitConsts) {
		t.Errorf("fixWaits has %d kinds, the test lists %d", len(fixWaits), len(allowedWaitConsts))
	}
	for _, n := range allowedWaitConsts {
		if !fixWaits[waits[n]] {
			t.Errorf("%s is listed as allowed and is not in fixWaits", n)
		}
	}
	// a step type that is denied has no profile, whatever its fields say
	for _, s := range []pp.Step{pp.SystemDialogStep{Op: pp.SysTap}, pp.LoopStep{}, pp.RecipeCall{}, pp.AssertNoDefectsStep{}, pp.HTTPStep{}} {
		if _, ok := fixProfile(s); ok {
			t.Errorf("%T has a profile", s)
		}
	}
	if strings.Contains(strings.Join(allowedStepTypes, ","), "SystemDialog") {
		t.Error("a system dialog step is allowed")
	}
}
