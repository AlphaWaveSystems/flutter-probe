package catalog

// Detox returns the construct catalog for Detox JS/TS test files.
func Detox() Language {
	return Language{
		Name:           "detox",
		DisplayName:    "Detox",
		FileExtensions: []string{".js", ".ts"},
		Version:        "Detox 20+",
		StructureEBNF: `
DetoxFile    = { ImportStmt | DescribeBlock } .
DescribeBlock = "describe(" STRING "," AsyncFn ")" .
AsyncFn      = "async () =>" "{" { Statement } "}" .
ItBlock      = "it(" STRING "," AsyncFn ")" .
BeforeBlock  = ( "beforeAll" | "beforeEach" ) "(" AsyncFn ")" .
AfterBlock   = ( "afterAll" | "afterEach" ) "(" AsyncFn ")" .
Statement    = "await" ( ElementAction | Expectation | WaitFor | DeviceAction ) ";" .
ElementAction = "element(" Matcher ")" "." Action "(" { Arg } ")" .
Matcher      = "by." ( "id" | "text" | "label" ) "(" STRING ")" .
Action       = "tap" | "typeText" | "replaceText" | "clearText" | "longPress" | "swipe" | "scroll" .
Expectation  = "expect(" "element(" Matcher ")" ")" "." [ "not." ] Assertion .
Assertion    = "toBeVisible()" | "toExist()" | "toHaveText(" STRING ")" .
WaitFor      = "waitFor(" "element(" Matcher ")" ")" ".toBeVisible()" .
DeviceAction = "device." ( "launchApp" | "reloadReactNative" | "takeScreenshot" ) "(" { Arg } ")" .
`,
		Constructs: []Construct{
			// Structure
			{ID: "detox.describe", Name: "describe()", Category: CatStructure, Level: Full,
				EBNF: `"describe(" STRING ","`, Example: "describe('Login', () => {", ProbeTemplate: "# $1", ProbeExample: "# Login"},
			{ID: "detox.it", Name: "it()", Category: CatStructure, Level: Full,
				EBNF: `"it(" STRING ","`, Example: "it('should log in', async () => {", ProbeTemplate: `test "$1"`, ProbeExample: `test "should log in"`},
			{ID: "detox.beforeEach", Name: "beforeEach()", Category: CatLifecycle, Level: Full,
				EBNF: `"before" ("All" | "Each") "("`, Example: "beforeEach(async () => {", ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "detox.afterEach", Name: "afterEach()", Category: CatLifecycle, Level: Full,
				EBNF: `"after" ("All" | "Each") "("`, Example: "afterEach(async () => {", ProbeTemplate: "after each", ProbeExample: "after each"},

			// Actions — tap
			{ID: "detox.tap.id", Name: "element(by.id()).tap()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).tap()"`, Example: "element(by.id('loginBtn')).tap()",
				ProbeTemplate: "tap on #$1", ProbeExample: "tap on #loginBtn"},
			{ID: "detox.tap.text", Name: "element(by.text()).tap()", Category: CatAction, Level: Full,
				EBNF: `"element(by.text(" STRING ")).tap()"`, Example: "element(by.text('Sign In')).tap()",
				ProbeTemplate: `tap on "$1"`, ProbeExample: `tap on "Sign In"`},
			{ID: "detox.tap.label", Name: "element(by.label()).tap()", Category: CatAction, Level: Full,
				EBNF: `"element(by.label(" STRING ")).tap()"`, Example: "element(by.label('Submit')).tap()",
				ProbeTemplate: `tap on "$1"`, ProbeExample: `tap on "Submit"`},

			// Actions — type
			{ID: "detox.typeText.id", Name: "element(by.id()).typeText()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).typeText(" STRING ")"`, Example: "element(by.id('emailInput')).typeText('user@test.com')",
				ProbeTemplate: `type "$2" into #$1`, ProbeExample: `type "user@test.com" into #emailInput`},
			{ID: "detox.replaceText", Name: "element(by.id()).replaceText()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).replaceText(" STRING ")"`, Example: "element(by.id('input')).replaceText('new')",
				ProbeTemplate: `type "$2" into #$1`, ProbeExample: `type "new" into #input`},
			{ID: "detox.clearText", Name: "element(by.id()).clearText()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).clearText()"`, Example: "element(by.id('input')).clearText()",
				ProbeTemplate: "clear #$1", ProbeExample: "clear #input"},

			// Gestures
			{ID: "detox.longPress.id", Name: "element(by.id()).longPress()", Category: CatGesture, Level: Full,
				EBNF: `"element(by.id(" STRING ")).longPress()"`, Example: "element(by.id('item')).longPress()",
				ProbeTemplate: "long press on #$1", ProbeExample: "long press on #item"},
			{ID: "detox.swipe.id", Name: "element(by.id()).swipe()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).swipe(" STRING ")"`, Example: "element(by.id('list')).swipe('up')",
				ProbeTemplate: "swipe $2 on #$1", ProbeExample: "swipe up on #list"},
			{ID: "detox.scroll.id", Name: "element(by.id()).scroll()", Category: CatAction, Level: Full,
				EBNF: `"element(by.id(" STRING ")).scroll(" INT "," STRING ")"`, Example: "element(by.id('view')).scroll(200, 'down')",
				ProbeTemplate: "scroll $2 on #$1", ProbeExample: "scroll down on #view"},

			// Assertions
			{ID: "detox.expect.text.visible", Name: "expect(by.text()).toBeVisible()", Category: CatAssertion, Level: Full,
				EBNF: `"expect(element(by.text(" STRING "))).toBeVisible()"`, Example: "expect(element(by.text('Dashboard'))).toBeVisible()",
				ProbeTemplate: `see "$1"`, ProbeExample: `see "Dashboard"`},
			{ID: "detox.expect.id.visible", Name: "expect(by.id()).toBeVisible()", Category: CatAssertion, Level: Full,
				EBNF: `"expect(element(by.id(" STRING "))).toBeVisible()"`, Example: "expect(element(by.id('title'))).toBeVisible()",
				ProbeTemplate: `see #$1`, ProbeExample: `see #title`},
			{ID: "detox.expect.text.notVisible", Name: "expect(by.text()).not.toBeVisible()", Category: CatAssertion, Level: Full,
				EBNF: `"expect(element(by.text(" STRING "))).not." ("toBeVisible()" | "toExist()")`,
				Example: "expect(element(by.text('Error'))).not.toBeVisible()",
				ProbeTemplate: `don't see "$1"`, ProbeExample: `don't see "Error"`},
			{ID: "detox.expect.haveText", Name: "expect(by.id()).toHaveText()", Category: CatAssertion, Level: Full,
				EBNF: `"expect(element(by.id(" STRING "))).toHaveText(" STRING ")"`,
				Example: "expect(element(by.id('title'))).toHaveText('Hello')",
				ProbeTemplate: `see "$2"`, ProbeExample: `see "Hello"`},

			// Wait
			{ID: "detox.waitFor.text", Name: "waitFor(by.text()).toBeVisible()", Category: CatWait, Level: Full,
				EBNF: `"waitFor(element(by.text(" STRING "))).toBeVisible()"`, Example: "waitFor(element(by.text('Loading'))).toBeVisible()",
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "Loading" appears`},
			{ID: "detox.waitFor.id", Name: "waitFor(by.id()).toBeVisible()", Category: CatWait, Level: Full,
				EBNF: `"waitFor(element(by.id(" STRING "))).toBeVisible()"`, Example: "waitFor(element(by.id('spinner'))).toBeVisible()",
				ProbeTemplate: `wait until #$1 appears`, ProbeExample: `wait until #spinner appears`},

			// Device
			{ID: "detox.device.launch", Name: "device.launchApp()", Category: CatAppControl, Level: Full,
				EBNF: `"device.launchApp("`, Example: "device.launchApp()", ProbeTemplate: "open the app", ProbeExample: "open the app"},
			{ID: "detox.device.reload", Name: "device.reloadReactNative()", Category: CatAppControl, Level: Full,
				EBNF: `"device.reloadReactNative()"`, Example: "device.reloadReactNative()", ProbeTemplate: "restart the app", ProbeExample: "restart the app"},
			{ID: "detox.device.screenshot", Name: "device.takeScreenshot()", Category: CatScreenshot, Level: Full,
				EBNF: `"device.takeScreenshot(" STRING ")"`, Example: "device.takeScreenshot('home')",
				ProbeTemplate: `take a screenshot called "$1"`, ProbeExample: `take a screenshot called "home"`},
		},
	}
}
