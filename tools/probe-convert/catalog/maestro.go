package catalog

// Maestro returns the construct catalog for Maestro YAML flows.
func Maestro() Language {
	return Language{
		Name:           "maestro",
		DisplayName:    "Maestro",
		FileExtensions: []string{".yaml", ".yml"},
		Version:        "1.36+",
		StructureEBNF: `
Flow         = Header "---" StepList .
Header       = "appId:" STRING { EnvEntry } .
EnvEntry     = "env:" NEWLINE { IDENT ":" STRING } .
StepList     = "[" { Step "," } "]" .
Step         = MapStep | STRING .
MapStep      = "{" IDENT ":" Value "}" .
Value        = STRING | MapValue | ListValue .
MapValue     = "{" { IDENT ":" Value } "}" .
ListValue    = "[" { Step "," } "]" .
`,
		Constructs: []Construct{
			// App control
			{ID: "maestro.launchApp", Name: "launchApp", Category: CatAppControl, Level: Full,
				EBNF: `"launchApp"`, Example: `- launchApp`, ProbeTemplate: "open the app", ProbeExample: "open the app"},
			{ID: "maestro.stopApp", Name: "stopApp", Category: CatAppControl, Level: Full,
				EBNF: `"stopApp"`, Example: `- stopApp`, ProbeTemplate: "close the app", ProbeExample: "close the app"},
			{ID: "maestro.clearState", Name: "clearState", Category: CatAppControl, Level: Full,
				EBNF: `"clearState" [ ":" Value ]`, Example: `- clearState`, ProbeTemplate: "clear app data", ProbeExample: "clear app data"},

			// Actions
			{ID: "maestro.tapOn", Name: "tapOn", Category: CatAction, Level: Full,
				EBNF: `"tapOn:" ( STRING | { "id:" STRING } | { "text:" STRING } )`, Example: `- tapOn: "Sign In"`,
				ProbeTemplate: `tap on $1`, ProbeExample: `tap on "Sign In"`},
			{ID: "maestro.tapOn.id", Name: "tapOn (by id)", Category: CatAction, Level: Full,
				EBNF: `"tapOn:" NEWLINE "id:" STRING`, Example: "- tapOn:\n    id: \"loginBtn\"",
				ProbeTemplate: `tap on #$1`, ProbeExample: `tap on #loginBtn`},
			{ID: "maestro.longPressOn", Name: "longPressOn", Category: CatGesture, Level: Full,
				EBNF: `"longPressOn:" ( STRING | { "id:" STRING } )`, Example: `- longPressOn: "Delete"`,
				ProbeTemplate: `long press on $1`, ProbeExample: `long press on "Delete"`},
			{ID: "maestro.doubleTapOn", Name: "doubleTapOn", Category: CatGesture, Level: Full,
				EBNF: `"doubleTapOn:" ( STRING | { "id:" STRING } )`, Example: `- doubleTapOn: "Like"`,
				ProbeTemplate: `double tap on $1`, ProbeExample: `double tap on "Like"`},
			{ID: "maestro.inputText", Name: "inputText", Category: CatAction, Level: Full,
				EBNF: `"inputText:" STRING`, Example: `- inputText: "user@test.com"`,
				ProbeTemplate: `type $1`, ProbeExample: `type "user@test.com"`},

			// Assertions
			{ID: "maestro.assertVisible", Name: "assertVisible", Category: CatAssertion, Level: Full,
				EBNF: `"assertVisible:" ( STRING | { "id:" STRING } )`, Example: `- assertVisible: "Dashboard"`,
				ProbeTemplate: `see $1`, ProbeExample: `see "Dashboard"`},
			{ID: "maestro.assertNotVisible", Name: "assertNotVisible", Category: CatAssertion, Level: Full,
				EBNF: `"assertNotVisible:" ( STRING | { "id:" STRING } )`, Example: `- assertNotVisible: "Loading"`,
				ProbeTemplate: `don't see $1`, ProbeExample: `don't see "Loading"`},

			// Navigation
			{ID: "maestro.back", Name: "back", Category: CatNavigation, Level: Full,
				EBNF: `"back"`, Example: `- back`, ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "maestro.pressKey", Name: "pressKey", Category: CatNavigation, Level: Full,
				EBNF: `"pressKey:" ( "back" | "home" | STRING )`, Example: `- pressKey: "back"`,
				ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "maestro.hideKeyboard", Name: "hideKeyboard", Category: CatNavigation, Level: Full,
				EBNF: `"hideKeyboard" | "closeKeyboard"`, Example: `- hideKeyboard`,
				ProbeTemplate: "close keyboard", ProbeExample: "close keyboard"},

			// Scroll / swipe
			{ID: "maestro.scroll", Name: "scroll", Category: CatAction, Level: Full,
				EBNF: `( "scroll" | "scrollDown" )`, Example: `- scroll`, ProbeTemplate: "scroll down", ProbeExample: "scroll down"},
			{ID: "maestro.scrollUp", Name: "scrollUp", Category: CatAction, Level: Full,
				EBNF: `"scrollUp"`, Example: `- scrollUp`, ProbeTemplate: "scroll up", ProbeExample: "scroll up"},
			{ID: "maestro.swipe", Name: "swipe", Category: CatAction, Level: Full,
				EBNF: `"swipe:" NEWLINE "direction:" ( "UP" | "DOWN" | "LEFT" | "RIGHT" )`,
				Example: "- swipe:\n    direction: LEFT", ProbeTemplate: `swipe $1`, ProbeExample: "swipe left"},

			// Wait
			{ID: "maestro.waitForAnimationToEnd", Name: "waitForAnimationToEnd", Category: CatWait, Level: Full,
				EBNF: `"waitForAnimationToEnd"`, Example: `- waitForAnimationToEnd`,
				ProbeTemplate: "wait for the page to load", ProbeExample: "wait for the page to load"},
			{ID: "maestro.wait", Name: "wait", Category: CatWait, Level: Full,
				EBNF: `"wait:" NEWLINE "for:" INT`, Example: "- wait:\n    for: 3000",
				ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 3.0 seconds"},

			// Screenshot
			{ID: "maestro.takeScreenshot", Name: "takeScreenshot", Category: CatScreenshot, Level: Full,
				EBNF: `"takeScreenshot:" STRING`, Example: `- takeScreenshot: "home_screen"`,
				ProbeTemplate: `take a screenshot called $1`, ProbeExample: `take a screenshot called "home_screen"`},

			// Flow
			{ID: "maestro.runFlow", Name: "runFlow", Category: CatFlow, Level: Full,
				EBNF: `"runFlow:" STRING`, Example: `- runFlow: "login.yaml"`,
				ProbeTemplate: `use $1`, ProbeExample: `use "login.yaml"`},
			{ID: "maestro.repeat", Name: "repeat", Category: CatFlow, Level: Full,
				EBNF: `"repeat:" NEWLINE "times:" INT [ NEWLINE "commands:" StepList ]`,
				Example: "- repeat:\n    times: 3\n    commands:\n      - tapOn: \"Next\"",
				ProbeTemplate: "repeat $1 times", ProbeExample: "repeat 3 times"},
			{ID: "maestro.openLink", Name: "openLink", Category: CatAction, Level: Full,
				EBNF: `"openLink:" STRING`, Example: `- openLink: "https://example.com"`,
				ProbeTemplate: `open $1`, ProbeExample: `open "https://example.com"`},

			// Partial / manual
			{ID: "maestro.evalScript", Name: "evalScript", Category: CatFlow, Level: Full,
				EBNF: `"evalScript:" STRING`, Example: `- evalScript: "console.log('test')"`,
				ProbeTemplate: "run dart: ...", ProbeExample: "run dart:\n  print('test')",
				Notes: "JS source is transpiled to a run dart: block with simple JS→Dart transforms (console.log→print, const→final)"},
			{ID: "maestro.setAirplaneMode", Name: "setAirplaneMode", Category: CatAppControl, Level: Full,
				EBNF: `"setAirplaneMode:" BOOL`, Example: `- setAirplaneMode: true`,
				ProbeTemplate: "toggle wifi off", ProbeExample: "toggle wifi off",
				Notes: "Mapped to wifi toggle; for true airplane mode use adb shell settings put global airplane_mode_on 1"},
			{ID: "maestro.ifdef", Name: "ifdef", Category: CatFlow, Level: Partial,
				EBNF: `( "ifdef" | "skipOn" | "onlyOn" ) ":" Value`, Example: `- ifdef:\n    platform: android`,
				ProbeTemplate: "run dart: // Platform guard", ProbeExample: "run dart:\n  // if (!Platform.isAndroid) throw Exception('Skip');",
				Notes: "Emits a run dart: block with platform guard comment; user must uncomment and adapt"},
			{ID: "maestro.setLocation", Name: "setLocation", Category: CatAppControl, Level: Partial,
				EBNF: `"setLocation:" NEWLINE "latitude:" FLOAT NEWLINE "longitude:" FLOAT`,
				Example: "- setLocation:\n    latitude: 37.77\n    longitude: -122.41",
				ProbeTemplate: "run dart: // GPS mock", ProbeExample: "run dart:\n  // Coordinates: 37.77, -122.41",
				Notes: "Emits platform-specific GPS commands as comments and a run dart: block; actual GPS mocking requires adb/simctl"},
		},
	}
}
