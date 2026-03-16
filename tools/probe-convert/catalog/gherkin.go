package catalog

// Gherkin returns the construct catalog for Cucumber/Gherkin .feature files.
func Gherkin() Language {
	return Language{
		Name:           "gherkin",
		DisplayName:    "Gherkin (Cucumber)",
		FileExtensions: []string{".feature"},
		Version:        "Gherkin 6+",
		StructureEBNF: `
Feature      = "Feature:" TEXT NEWLINE { Tag | Background | Scenario | ScenarioOutline } .
Tag          = "@" IDENT { " @" IDENT } NEWLINE .
Background   = "Background:" NEWLINE { Step } .
Scenario     = [ Tag ] "Scenario:" TEXT NEWLINE { Step } .
ScenarioOutline = [ Tag ] "Scenario Outline:" TEXT NEWLINE { Step } Examples .
Step         = ( "Given" | "When" | "Then" | "And" | "But" | "*" ) TEXT NEWLINE .
Examples     = "Examples:" NEWLINE TableHeader { TableRow } .
TableHeader  = "|" { TEXT "|" } NEWLINE .
TableRow     = "|" { TEXT "|" } NEWLINE .
`,
		Constructs: []Construct{
			// Structure
			{ID: "gherkin.feature", Name: "Feature:", Category: CatStructure, Level: Full,
				EBNF: `"Feature:" TEXT`, Example: "Feature: Login", ProbeTemplate: "# Feature: $1", ProbeExample: "# Converted from Gherkin — Feature: Login"},
			{ID: "gherkin.scenario", Name: "Scenario:", Category: CatStructure, Level: Full,
				EBNF: `"Scenario:" TEXT`, Example: `Scenario: User can log in`, ProbeTemplate: `test "$1"`, ProbeExample: `test "User can log in"`},
			{ID: "gherkin.scenarioOutline", Name: "Scenario Outline:", Category: CatStructure, Level: Full,
				EBNF: `"Scenario Outline:" TEXT`, Example: `Scenario Outline: Login with <role>`, ProbeTemplate: `test "$1"`, ProbeExample: `test "Login with <role>"`},
			{ID: "gherkin.background", Name: "Background:", Category: CatLifecycle, Level: Full,
				EBNF: `"Background:" NEWLINE { Step }`, Example: "Background:\n  Given the app is launched", ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "gherkin.tag", Name: "@tag", Category: CatData, Level: Full,
				EBNF: `"@" IDENT { " @" IDENT }`, Example: `@smoke @critical`, ProbeTemplate: "@$1 @$2", ProbeExample: "@smoke @critical"},
			{ID: "gherkin.examples", Name: "Examples:", Category: CatData, Level: Full,
				EBNF: `"Examples:" NEWLINE TableHeader { TableRow }`, Example: "Examples:\n  | role | email |\n  | admin | a@b.com |",
				ProbeTemplate: "with examples:", ProbeExample: "with examples:\n  role\temail\n  \"admin\"\t\"a@b.com\""},

			// App lifecycle
			{ID: "gherkin.step.openApp", Name: "app is launched", Category: CatAppControl, Level: Full,
				EBNF: `("the" )? "app" ("is" )? ("launched" | "opened" | "started")`, Example: "Given the app is launched",
				ProbeTemplate: "open the app", ProbeExample: "open the app"},
			{ID: "gherkin.step.launchApp", Name: "I launch the app", Category: CatAppControl, Level: Full,
				EBNF: `"I" ("launch" | "open" | "start") "the app"`, Example: "When I launch the app",
				ProbeTemplate: "open the app", ProbeExample: "open the app"},
			{ID: "gherkin.step.restartApp", Name: "I restart the app", Category: CatAppControl, Level: Full,
				EBNF: `"I restart the app"`, Example: "When I restart the app",
				ProbeTemplate: "restart the app", ProbeExample: "restart the app"},
			{ID: "gherkin.step.clearData", Name: "I clear app data", Category: CatAppControl, Level: Full,
				EBNF: `"I clear" ("the" )? "app data"`, Example: "When I clear app data",
				ProbeTemplate: "clear app data", ProbeExample: "clear app data"},

			// Actions — type/enter
			{ID: "gherkin.step.typeInto", Name: `I type "X" into "Y"`, Category: CatAction, Level: Full,
				EBNF: `"I" ("type" | "enter" | "input") QUOTED ("into" | "in" | "on") ("the" )? QUOTED ("field" )?`,
				Example: `When I type "user@test.com" into "Email"`, ProbeTemplate: `type "$1" into "$2"`, ProbeExample: `type "user@test.com" into "Email"`},
			{ID: "gherkin.step.fillWith", Name: `I fill "Y" with "X"`, Category: CatAction, Level: Full,
				EBNF: `"I" ("fill" | "set") ("the" )? QUOTED ("field" )? ("with" | "to") QUOTED`,
				Example: `When I fill "Email" with "user@test.com"`, ProbeTemplate: `type "$2" into "$1"`, ProbeExample: `type "user@test.com" into "Email"`},
			{ID: "gherkin.step.typeVar", Name: `I type <var> into "Y"`, Category: CatAction, Level: Full,
				EBNF: `"I" ("type" | "enter") "<" IDENT ">" ("into" | "in") ("the" )? QUOTED ("field" )?`,
				Example: `When I type <email> into "Email"`, ProbeTemplate: `type <$1> into "$2"`, ProbeExample: `type <email> into "Email"`},

			// Actions — tap
			{ID: "gherkin.step.tap", Name: `I tap on "X"`, Category: CatAction, Level: Full,
				EBNF: `"I" ("tap" | "click" | "press") ("on" | "the" )? QUOTED`, Example: `When I tap on "Sign In"`,
				ProbeTemplate: `tap on "$1"`, ProbeExample: `tap on "Sign In"`},
			{ID: "gherkin.step.tapID", Name: `I tap on #id`, Category: CatAction, Level: Full,
				EBNF: `"I" ("tap" | "click" | "press") ("on" | "the" )? "#" IDENT`, Example: `When I tap on #loginBtn`,
				ProbeTemplate: `tap on #$1`, ProbeExample: `tap on #loginBtn`},

			// Gestures
			{ID: "gherkin.step.longPress", Name: `I long press on "X"`, Category: CatGesture, Level: Full,
				EBNF: `"I long press" ("on" )? QUOTED`, Example: `When I long press on "Delete"`,
				ProbeTemplate: `long press on "$1"`, ProbeExample: `long press on "Delete"`},
			{ID: "gherkin.step.doubleTap", Name: `I double tap on "X"`, Category: CatGesture, Level: Full,
				EBNF: `"I double tap" ("on" )? QUOTED`, Example: `When I double tap on "Like"`,
				ProbeTemplate: `double tap on "$1"`, ProbeExample: `double tap on "Like"`},

			// Assertions
			{ID: "gherkin.step.shouldSee", Name: `I should see "X"`, Category: CatAssertion, Level: Full,
				EBNF: `"I should" ("be able to" )? "see" QUOTED`, Example: `Then I should see "Dashboard"`,
				ProbeTemplate: `see "$1"`, ProbeExample: `see "Dashboard"`},
			{ID: "gherkin.step.canSee", Name: `I see "X"`, Category: CatAssertion, Level: Full,
				EBNF: `"I" ("can" )? "see" QUOTED`, Example: `Then I see "Dashboard"`,
				ProbeTemplate: `see "$1"`, ProbeExample: `see "Dashboard"`},
			{ID: "gherkin.step.isVisible", Name: `"X" is visible`, Category: CatAssertion, Level: Full,
				EBNF: `QUOTED ("is" | "should be") ("displayed" | "visible" | "shown")`, Example: `Then "Login" should be visible`,
				ProbeTemplate: `see "$1"`, ProbeExample: `see "Login"`},
			{ID: "gherkin.step.shouldNotSee", Name: `I should not see "X"`, Category: CatAssertion, Level: Full,
				EBNF: `"I should not see" QUOTED`, Example: `Then I should not see "Error"`,
				ProbeTemplate: `don't see "$1"`, ProbeExample: `don't see "Error"`},
			{ID: "gherkin.step.cantSee", Name: `I can't see "X"`, Category: CatAssertion, Level: Full,
				EBNF: `"I" ("can't" | "cannot" | "don't") "see" QUOTED`, Example: `Then I can't see "Error"`,
				ProbeTemplate: `don't see "$1"`, ProbeExample: `don't see "Error"`},

			// Wait
			{ID: "gherkin.step.waitSeconds", Name: `I wait N seconds`, Category: CatWait, Level: Full,
				EBNF: `"I wait" INT "seconds"?`, Example: `When I wait 3 seconds`,
				ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 3 seconds"},
			{ID: "gherkin.step.waitUntil", Name: `I wait until "X" appears`, Category: CatWait, Level: Full,
				EBNF: `"I wait until" QUOTED ("appears" | "is visible")`, Example: `When I wait until "Dashboard" appears`,
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "Dashboard" appears`},
			{ID: "gherkin.step.waitFor", Name: `I wait for "X" to appear`, Category: CatWait, Level: Full,
				EBNF: `"I wait for" QUOTED "to" ("appear" | "be visible")`, Example: `When I wait for "Dashboard" to appear`,
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "Dashboard" appears`},
			{ID: "gherkin.step.waitPageLoad", Name: "I wait for the page to load", Category: CatWait, Level: Full,
				EBNF: `"I wait for the page to load"`, Example: "When I wait for the page to load",
				ProbeTemplate: "wait for the page to load", ProbeExample: "wait for the page to load"},

			// Swipe / scroll
			{ID: "gherkin.step.swipe", Name: "I swipe direction", Category: CatAction, Level: Full,
				EBNF: `"I swipe" ("left" | "right" | "up" | "down")`, Example: "When I swipe up",
				ProbeTemplate: "swipe $1", ProbeExample: "swipe up"},
			{ID: "gherkin.step.scroll", Name: "I scroll direction", Category: CatAction, Level: Full,
				EBNF: `"I scroll" ("up" | "down")`, Example: "When I scroll down",
				ProbeTemplate: "scroll $1", ProbeExample: "scroll down"},

			// Navigation
			{ID: "gherkin.step.goBack", Name: "I go back", Category: CatNavigation, Level: Full,
				EBNF: `"I" ("go" | "press" | "navigate") "back"`, Example: "When I go back",
				ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "gherkin.step.closeKeyboard", Name: "I close the keyboard", Category: CatNavigation, Level: Full,
				EBNF: `"I close the keyboard"`, Example: "When I close the keyboard",
				ProbeTemplate: "close keyboard", ProbeExample: "close keyboard"},

			// Screenshot
			{ID: "gherkin.step.screenshot", Name: "I take a screenshot", Category: CatScreenshot, Level: Full,
				EBNF: `"I take a screenshot" ("called" QUOTED)?`, Example: "When I take a screenshot",
				ProbeTemplate: "take a screenshot", ProbeExample: "take a screenshot"},

			// Permissions
			{ID: "gherkin.step.allowPerm", Name: `I allow "X" permission`, Category: CatPermission, Level: Full,
				EBNF: `"I" ("allow" | "grant") ("the" )? QUOTED "permission"`, Example: `When I allow the "camera" permission`,
				ProbeTemplate: `allow permission "$1"`, ProbeExample: `allow permission "camera"`},
			{ID: "gherkin.step.denyPerm", Name: `I deny "X" permission`, Category: CatPermission, Level: Full,
				EBNF: `"I" ("deny" | "revoke") ("the" )? QUOTED "permission"`, Example: `When I deny the "location" permission`,
				ProbeTemplate: `deny permission "$1"`, ProbeExample: `deny permission "location"`},
			{ID: "gherkin.step.grantAll", Name: "I grant all permissions", Category: CatPermission, Level: Full,
				EBNF: `"I grant all permissions"`, Example: "When I grant all permissions",
				ProbeTemplate: "grant all permissions", ProbeExample: "grant all permissions"},
		},
	}
}
