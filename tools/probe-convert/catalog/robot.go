package catalog

// Robot returns the construct catalog for Robot Framework .robot files.
func Robot() Language {
	return Language{
		Name:           "robot",
		DisplayName:    "Robot Framework",
		FileExtensions: []string{".robot"},
		Version:        "Robot Framework 5+ / AppiumLibrary",
		StructureEBNF: `
RobotFile    = { Section } .
Section      = SectionHeader { Line } .
SectionHeader = "***" ( "Settings" | "Variables" | "Test Cases" | "Keywords" | "Tasks" ) "***" .
SettingLine  = ( "Suite Setup" | "Suite Teardown" | "Test Setup" | "Test Teardown" ) SEP KeywordCall .
TestCase     = NAME NEWLINE { TestBody } .
TestBody     = SEP ( Setting | KeywordCall ) NEWLINE .
Setting      = "[" ( "Tags" | "Arguments" | "Template" | "Setup" | "Teardown" | "Documentation" ) "]" SEP { VALUE } .
KeywordCall  = KEYWORD { SEP VALUE } .
Keyword      = NAME NEWLINE { KeywordBody } .
SEP          = "  " | TAB .
`,
		Constructs: []Construct{
			// Structure
			{ID: "robot.testCase", Name: "Test Case name", Category: CatStructure, Level: Full,
				EBNF: `NAME NEWLINE`, Example: "Login Test", ProbeTemplate: `test "$1"`, ProbeExample: `test "Login Test"`},
			{ID: "robot.keyword", Name: "Keyword definition", Category: CatStructure, Level: Full,
				EBNF: `NAME NEWLINE`, Example: "Login With Credentials", ProbeTemplate: `recipe "$1"`, ProbeExample: `recipe "Login With Credentials"`},
			{ID: "robot.tags", Name: "[Tags]", Category: CatData, Level: Full,
				EBNF: `"[Tags]" SEP TAG { SEP TAG }`, Example: "[Tags]    smoke    critical", ProbeTemplate: "@$1 @$2", ProbeExample: "@smoke @critical"},
			{ID: "robot.arguments", Name: "[Arguments]", Category: CatData, Level: Full,
				EBNF: `"[Arguments]" SEP "${" IDENT "}" { SEP "${" IDENT "}" }`, Example: "[Arguments]    ${email}    ${password}",
				ProbeTemplate: "recipe params ($1, $2)", ProbeExample: "# arguments: email, password"},
			{ID: "robot.suiteSetup", Name: "Suite Setup", Category: CatLifecycle, Level: Full,
				EBNF: `"Suite Setup" SEP KeywordCall`, Example: "Suite Setup    Open Application    ...", ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "robot.suiteTeardown", Name: "Suite Teardown", Category: CatLifecycle, Level: Full,
				EBNF: `"Suite Teardown" SEP KeywordCall`, Example: "Suite Teardown    Close Application", ProbeTemplate: "after each", ProbeExample: "after each"},
			{ID: "robot.variable", Name: "${VAR}", Category: CatData, Level: Full,
				EBNF: `"${" IDENT "}"`, Example: "${EMAIL}", ProbeTemplate: "<$1>", ProbeExample: "<email>"},

			// Keywords → ProbeScript
			{ID: "robot.clickElement", Name: "Click Element", Category: CatAction, Level: Full,
				EBNF: `"Click Element" SEP LOCATOR`, Example: "Click Element    id=loginBtn",
				ProbeTemplate: "tap on $locator", ProbeExample: "tap on #loginBtn"},
			{ID: "robot.clickText", Name: "Click Text", Category: CatAction, Level: Full,
				EBNF: `"Click Text" SEP TEXT`, Example: "Click Text    Continue",
				ProbeTemplate: `tap on "$1"`, ProbeExample: `tap on "Continue"`},
			{ID: "robot.inputText", Name: "Input Text", Category: CatAction, Level: Full,
				EBNF: `"Input Text" SEP LOCATOR SEP VALUE`, Example: "Input Text    id=emailField    user@test.com",
				ProbeTemplate: `type "$2" into $locator`, ProbeExample: `type "user@test.com" into #emailField`},
			{ID: "robot.clearText", Name: "Clear Text", Category: CatAction, Level: Full,
				EBNF: `"Clear Text" SEP LOCATOR`, Example: "Clear Text    id=field",
				ProbeTemplate: `clear $locator`, ProbeExample: `clear #field`},
			{ID: "robot.pageContainsText", Name: "Page Should Contain Text", Category: CatAssertion, Level: Full,
				EBNF: `"Page Should Contain Text" SEP TEXT`, Example: "Page Should Contain Text    Dashboard",
				ProbeTemplate: `see "$1"`, ProbeExample: `see "Dashboard"`},
			{ID: "robot.pageNotContainsText", Name: "Page Should Not Contain Text", Category: CatAssertion, Level: Full,
				EBNF: `"Page Should Not Contain Text" SEP TEXT`, Example: "Page Should Not Contain Text    Error",
				ProbeTemplate: `don't see "$1"`, ProbeExample: `don't see "Error"`},
			{ID: "robot.elemVisible", Name: "Element Should Be Visible", Category: CatAssertion, Level: Full,
				EBNF: `"Element Should Be Visible" SEP LOCATOR`, Example: "Element Should Be Visible    id=banner",
				ProbeTemplate: `see $locator`, ProbeExample: `see #banner`},
			{ID: "robot.elemNotVisible", Name: "Element Should Not Be Visible", Category: CatAssertion, Level: Full,
				EBNF: `"Element Should Not Be Visible" SEP LOCATOR`, Example: "Element Should Not Be Visible    id=spinner",
				ProbeTemplate: `don't see $locator`, ProbeExample: `don't see #spinner`},
			{ID: "robot.waitVisible", Name: "Wait Until Element Is Visible", Category: CatWait, Level: Full,
				EBNF: `"Wait Until Element Is Visible" SEP LOCATOR`, Example: "Wait Until Element Is Visible    id=spinner",
				ProbeTemplate: `wait until $locator appears`, ProbeExample: `wait until #spinner appears`},
			{ID: "robot.waitPageContains", Name: "Wait Until Page Contains", Category: CatWait, Level: Full,
				EBNF: `"Wait Until Page Contains" SEP TEXT`, Example: "Wait Until Page Contains    Ready",
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "Ready" appears`},
			{ID: "robot.screenshot", Name: "Capture Page Screenshot", Category: CatScreenshot, Level: Full,
				EBNF: `"Capture Page Screenshot"`, Example: "Capture Page Screenshot",
				ProbeTemplate: "take a screenshot", ProbeExample: "take a screenshot"},
			{ID: "robot.sleep", Name: "Sleep", Category: CatWait, Level: Full,
				EBNF: `"Sleep" SEP DURATION`, Example: "Sleep    5s", ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 5 seconds"},
			{ID: "robot.goBack", Name: "Go Back", Category: CatNavigation, Level: Full,
				EBNF: `"Go Back"`, Example: "Go Back", ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "robot.longPress", Name: "Long Press", Category: CatGesture, Level: Full,
				EBNF: `"Long Press" SEP LOCATOR`, Example: "Long Press    id=item",
				ProbeTemplate: `long press on $locator`, ProbeExample: `long press on #item`},
			{ID: "robot.openApp", Name: "Open Application", Category: CatAppControl, Level: Full,
				EBNF: `"Open Application" SEP URL { SEP CAP }`, Example: "Open Application    http://localhost:4723",
				ProbeTemplate: "open the app", ProbeExample: "open the app"},
			{ID: "robot.closeApp", Name: "Close Application", Category: CatAppControl, Level: Full,
				EBNF: `"Close Application"`, Example: "Close Application", ProbeTemplate: "close the app", ProbeExample: "close the app"},
			{ID: "robot.resetApp", Name: "Reset Application", Category: CatAppControl, Level: Full,
				EBNF: `"Reset Application"`, Example: "Reset Application", ProbeTemplate: "clear app data", ProbeExample: "clear app data"},
			{ID: "robot.hideKeyboard", Name: "Hide Keyboard", Category: CatNavigation, Level: Full,
				EBNF: `"Hide Keyboard"`, Example: "Hide Keyboard", ProbeTemplate: "close keyboard", ProbeExample: "close keyboard"},

			// Locators
			{ID: "robot.locator.id", Name: "id= locator", Category: CatData, Level: Full,
				EBNF: `"id=" IDENT`, Example: "id=loginBtn", ProbeTemplate: "#$1", ProbeExample: "#loginBtn"},
			{ID: "robot.locator.text", Name: "text= locator", Category: CatData, Level: Full,
				EBNF: `"text=" TEXT`, Example: "text=Sign In", ProbeTemplate: `"$1"`, ProbeExample: `"Sign In"`},
			{ID: "robot.locator.a11y", Name: "accessibility_id= locator", Category: CatData, Level: Full,
				EBNF: `"accessibility_id=" TEXT`, Example: "accessibility_id=Submit", ProbeTemplate: `"$1"`, ProbeExample: `"Submit"`},
			{ID: "robot.locator.xpath", Name: "xpath= locator", Category: CatData, Level: Partial,
				EBNF: `"xpath=" XPATH_EXPR`, Example: `xpath=//btn[@text='OK']`, ProbeTemplate: `"$text"`,
				ProbeExample: `"OK"`, Notes: "XPath text attribute extracted if present; otherwise emitted as comment"},
		},
	}
}
