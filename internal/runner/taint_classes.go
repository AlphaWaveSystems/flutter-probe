package runner

// taintClass says what a step may do with a variable's value that came from a page (see taint.go and docs/proposals/webview-read-and-taint.md).
type taintClass int

const (
	// classSink: the step sends a string out of the UI (a shell, a file, an address, a request, a baseline key). Its handler takes every script string through
	// forSink and calls no plain resolver.
	classSink taintClass = iota + 1
	// classLiteral: the handler uses its strings as written and expands no variable. Its handler calls no resolver at all; a step that starts to expand
	// variables must become a sink in the same change.
	classLiteral
	// classUI: the text is matched on the screen, typed, compared or sent along with a screenshot to the configured AI provider; the handler may use resolve.
	classUI
	// classPropagates: the step passes a value on (store, recipe parameters); taint follows through resolveT and setVar.
	classPropagates
	// classNoString: the step takes no script string, or only a number.
	classNoString
)

// taintClasses classifies every ActionVerb of the parser and every Step type, by the names used in internal/parser/ast.go. TestEveryStepIsClassifiedForTaint
// fails when one is missing, twice listed or unknown, so a new verb or step cannot be added without deciding this. ActionStep is classified through its verbs.
var taintClasses = map[string]taintClass{
	// ---- sinks
	"VerbOpenLink":    classSink,
	"VerbAddMedia":    classSink,
	"VerbSetLanguage": classSink,
	"VerbTypeNative":  classSink,
	"VerbTakeShot":    classSink,
	"VerbCompareShot": classSink,
	"HTTPCallStep":    classSink,
	"MockBlock":       classSink,
	"PerfStep":        classSink,

	// ---- literal only
	"VerbLog":             classLiteral,
	"VerbSetLocation":     classLiteral,
	"VerbSetAppearance":   classLiteral,
	"VerbSetAirplaneMode": classLiteral,
	"VerbAllowPermission": classLiteral,
	"VerbDenyPermission":  classLiteral,
	"VerbDeliverSignal":   classLiteral,
	"DartBlock":           classLiteral,
	"TravelStep":          classLiteral,

	// ---- UI only
	"VerbType":           classUI,
	"VerbTap":            classUI,
	"VerbLongPress":      classUI,
	"VerbDoubleTap":      classUI,
	"VerbClear":          classUI,
	"VerbDrag":           classUI,
	"VerbSwipe":          classUI,
	"VerbScroll":         classUI,
	"VerbToggle":         classUI,
	"VerbTapNative":      classUI,
	"VerbTapWebView":     classUI,
	"VerbTypeWebView":    classUI,
	"VerbCopyClipboard":  classUI,
	"VerbPasteClipboard": classUI,
	"VerbReadWithAI":     classUI,
	"AssertStep":         classUI,
	"WaitStep":           classUI,
	"SystemDialogStep":   classUI,
	"HTTPStep":           classUI,
	"ConditionalStep":    classUI,

	// ---- propagates
	"VerbStore":  classPropagates,
	"RecipeCall": classPropagates,

	// ---- no string
	"VerbOpen":             classNoString,
	"VerbGoBack":           classNoString,
	"VerbPressEnter":       classNoString,
	"VerbClose":            classNoString,
	"VerbPinch":            classNoString,
	"VerbRotate":           classNoString,
	"VerbShake":            classNoString,
	"VerbPress":            classNoString,
	"VerbPressKey":         classNoString,
	"VerbDumpTree":         classNoString,
	"VerbSaveLogs":         classNoString,
	"VerbPause":            classNoString,
	"VerbRestart":          classNoString,
	"VerbClearAppData":     classNoString,
	"VerbGrantAllPerms":    classNoString,
	"VerbRevokeAllPerms":   classNoString,
	"VerbKill":             classNoString,
	"VerbVerifyBrowser":    classNoString,
	"VerbEnrollBiometric":  classNoString,
	"VerbBiometricMatch":   classNoString,
	"VerbBiometricNoMatch": classNoString,
	"AssertNoDefectsStep":  classNoString,
	"LoopStep":             classNoString,
	"RetryStep":            classNoString,
	"TimedStep":            classNoString,
	"DeviceStep":           classNoString,
	"SyncStep":             classNoString,
}

// sinkHandlers says where the strings of each sink are taken: a case of runAction (verbs) or a function (step types). The test requires the guard there.
var sinkHandlers = map[string]struct{ fn string }{
	"HTTPCallStep": {"runHTTPCall"},
	"MockBlock":    {"runMock"},
	"PerfStep":     {"runPerf"},
	"VerbTakeShot": {"runAction"}, "VerbCompareShot": {"runAction"}, "VerbOpenLink": {"runAction"}, "VerbAddMedia": {"runAction"},
	"VerbSetLanguage": {"runAction"}, "VerbTypeNative": {"runAction"},
}

// literalHandlers is the same for the literal-only steps.
var literalHandlers = map[string]struct{ fn string }{
	"DartBlock": {"runDart"}, "TravelStep": {"runTravel"},
	"VerbLog": {"runAction"}, "VerbSetLocation": {"runAction"}, "VerbSetAppearance": {"runAction"}, "VerbSetAirplaneMode": {"runAction"},
	"VerbAllowPermission": {"runAction"}, "VerbDenyPermission": {"runAction"}, "VerbDeliverSignal": {"runAction"},
}
