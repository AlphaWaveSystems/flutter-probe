package catalog

// AppiumPython returns the construct catalog for Appium Python tests.
func AppiumPython() Language {
	return Language{
		Name:           "appium_python",
		DisplayName:    "Appium (Python)",
		FileExtensions: []string{".py"},
		Version:        "Appium Python Client 3+",
		StructureEBNF: `
TestFile     = { ImportStmt | ClassDef } .
ClassDef     = "class" NAME "(" NAME ")" ":" NEWLINE { MethodDef } .
MethodDef    = "def" NAME "(self)" ":" NEWLINE { Statement } .
Statement    = EXPR NEWLINE .
FindAction   = "self.driver.find_element(" Strategy "," STRING ")" "." Action "(" { Arg } ")" .
Strategy     = ( "By" | "AppiumBy" ) "." ( "ID" | "ACCESSIBILITY_ID" | "XPATH" | "NAME" ) .
Action       = "click" | "send_keys" | "clear" .
`,
		Constructs: []Construct{
			// Structure
			{ID: "appium_py.class", Name: "class TestName", Category: CatStructure, Level: Full,
				EBNF: `"class" NAME "(unittest.TestCase)" ":"`, Example: "class LoginTest(unittest.TestCase):",
				ProbeTemplate: "# Test class: $1", ProbeExample: "# Test class: LoginTest"},
			{ID: "appium_py.testMethod", Name: "def test_name(self)", Category: CatStructure, Level: Full,
				EBNF: `"def" "test_" NAME "(self)" ":"`, Example: "def test_login_success(self):",
				ProbeTemplate: `test "$1"`, ProbeExample: `test "login success"`},
			{ID: "appium_py.setUp", Name: "def setUp(self)", Category: CatLifecycle, Level: Full,
				EBNF: `"def setUp(self)" ":"`, Example: "def setUp(self):", ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "appium_py.tearDown", Name: "def tearDown(self)", Category: CatLifecycle, Level: Full,
				EBNF: `"def tearDown(self)" ":"`, Example: "def tearDown(self):", ProbeTemplate: "after each", ProbeExample: "after each"},

			// Actions
			{ID: "appium_py.findClick", Name: "find_element().click()", Category: CatAction, Level: Full,
				EBNF: `"find_element(" STRATEGY "," STRING ").click()"`, Example: "find_element(AppiumBy.ID, 'btn').click()",
				ProbeTemplate: "tap on $locator", ProbeExample: "tap on #btn"},
			{ID: "appium_py.findSendKeys", Name: "find_element().send_keys()", Category: CatAction, Level: Full,
				EBNF: `"find_element(" STRATEGY "," STRING ").send_keys(" STRING ")"`,
				Example: "find_element(AppiumBy.ID, 'email').send_keys('user@test.com')",
				ProbeTemplate: `type "$value" into $locator`, ProbeExample: `type "user@test.com" into #email`},
			{ID: "appium_py.findClear", Name: "find_element().clear()", Category: CatAction, Level: Full,
				EBNF: `"find_element(" STRATEGY "," STRING ").clear()"`, Example: "find_element(AppiumBy.ID, 'field').clear()",
				ProbeTemplate: "clear $locator", ProbeExample: "clear #field"},

			// Wait
			{ID: "appium_py.webDriverWait", Name: "WebDriverWait().until()", Category: CatWait, Level: Full,
				EBNF: `"WebDriverWait" "(" DRIVER "," INT ")" ".until(" EC "." CONDITION "(" "(" STRATEGY "," STRING ")" ")" ")"`,
				Example: "WebDriverWait(self.driver, 10).until(EC.visibility_of_element_located((AppiumBy.ID, 'spinner')))",
				ProbeTemplate: `wait until $locator appears`, ProbeExample: `wait until #spinner appears`},
			{ID: "appium_py.sleep", Name: "time.sleep()", Category: CatWait, Level: Full,
				EBNF: `("time." )? "sleep(" INT ")"`, Example: "time.sleep(3)",
				ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 3 seconds"},

			// Navigation
			{ID: "appium_py.back", Name: ".back()", Category: CatNavigation, Level: Full,
				EBNF: `".back()"`, Example: "self.driver.back()", ProbeTemplate: "go back", ProbeExample: "go back"},

			// Screenshot
			{ID: "appium_py.screenshot", Name: "save_screenshot()", Category: CatScreenshot, Level: Full,
				EBNF: `("get_screenshot" | "save_screenshot") "(" STRING ")"`, Example: "self.driver.save_screenshot('home.png')",
				ProbeTemplate: `take a screenshot called "$1"`, ProbeExample: `take a screenshot called "home.png"`},

			// Locators
			{ID: "appium_py.locator.id", Name: "By.ID", Category: CatData, Level: Full,
				EBNF: `("By" | "AppiumBy") ".ID"`, Example: "AppiumBy.ID, 'loginBtn'", ProbeTemplate: "#$1", ProbeExample: "#loginBtn"},
			{ID: "appium_py.locator.a11y", Name: "By.ACCESSIBILITY_ID", Category: CatData, Level: Full,
				EBNF: `("By" | "AppiumBy") ".ACCESSIBILITY_ID"`, Example: "AppiumBy.ACCESSIBILITY_ID, 'Submit'",
				ProbeTemplate: `"$1"`, ProbeExample: `"Submit"`},
			{ID: "appium_py.locator.xpath", Name: "By.XPATH", Category: CatData, Level: Partial,
				EBNF: `("By" | "AppiumBy") ".XPATH"`, Example: `AppiumBy.XPATH, '//btn[@text="OK"]'`,
				ProbeTemplate: `"$text"`, ProbeExample: `"OK"`, Notes: "Text attribute extracted from XPath if present; otherwise comment"},
		},
	}
}

// AppiumJava returns the construct catalog for Appium Java tests.
func AppiumJava() Language {
	return Language{
		Name:           "appium_java",
		DisplayName:    "Appium (Java)",
		FileExtensions: []string{".java", ".kt"},
		Version:        "java-client 9+",
		StructureEBNF: `
TestFile     = { ImportStmt | ClassDef } .
ClassDef     = "public class" NAME "{" { MethodDef } "}" .
MethodDef    = { Annotation } AccessMod "void" NAME "()" "{" { Statement } "}" .
Annotation   = "@" ( "Test" | "Before" | "After" | "BeforeEach" | "AfterEach" ) .
Statement    = EXPR ";" .
FindAction   = "driver.findElement(" ByExpr ")" "." Action "(" { Arg } ")" .
ByExpr       = ( "By" | "AppiumBy" ) "." ( "id" | "accessibilityId" | "xpath" ) "(" STRING ")" .
Action       = "click" | "sendKeys" | "clear" .
`,
		Constructs: []Construct{
			{ID: "appium_java.class", Name: "class TestName", Category: CatStructure, Level: Full,
				EBNF: `"class" NAME`, Example: "public class LoginTest {", ProbeTemplate: "# Test class: $1", ProbeExample: "# Test class: LoginTest"},
			{ID: "appium_java.testMethod", Name: "@Test void testName()", Category: CatStructure, Level: Full,
				EBNF: `"@Test" AccessMod "void" "test" NAME "()"`, Example: "@Test\npublic void testLoginWithEmail() {",
				ProbeTemplate: `test "$1"`, ProbeExample: `test "login with email"`},
			{ID: "appium_java.before", Name: "@Before", Category: CatLifecycle, Level: Full,
				EBNF: `"@" ("Before" | "BeforeEach" | "BeforeAll")`, Example: "@Before\npublic void setUp() {",
				ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "appium_java.after", Name: "@After", Category: CatLifecycle, Level: Full,
				EBNF: `"@" ("After" | "AfterEach" | "AfterAll")`, Example: "@After\npublic void tearDown() {",
				ProbeTemplate: "after each", ProbeExample: "after each"},
			{ID: "appium_java.findClick", Name: "findElement().click()", Category: CatAction, Level: Full,
				EBNF: `"findElement(" BY_EXPR ").click()"`, Example: `findElement(AppiumBy.id("btn")).click()`,
				ProbeTemplate: "tap on $locator", ProbeExample: "tap on #btn"},
			{ID: "appium_java.findSendKeys", Name: "findElement().sendKeys()", Category: CatAction, Level: Full,
				EBNF: `"findElement(" BY_EXPR ").sendKeys(" STRING ")"`, Example: `findElement(AppiumBy.id("email")).sendKeys("user@test.com")`,
				ProbeTemplate: `type "$value" into $locator`, ProbeExample: `type "user@test.com" into #email`},
			{ID: "appium_java.findClear", Name: "findElement().clear()", Category: CatAction, Level: Full,
				EBNF: `"findElement(" BY_EXPR ").clear()"`, Example: `findElement(AppiumBy.id("field")).clear()`,
				ProbeTemplate: "clear $locator", ProbeExample: "clear #field"},
			{ID: "appium_java.threadSleep", Name: "Thread.sleep()", Category: CatWait, Level: Full,
				EBNF: `"Thread.sleep(" INT ")"`, Example: "Thread.sleep(3000)",
				ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 3 seconds"},
			{ID: "appium_java.back", Name: ".navigate().back()", Category: CatNavigation, Level: Full,
				EBNF: `".navigate().back()" | ".back()"`, Example: "driver.navigate().back()",
				ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "appium_java.screenshot", Name: "getScreenshotAs()", Category: CatScreenshot, Level: Full,
				EBNF: `"getScreenshotAs" | "takeScreenshot"`, Example: "driver.getScreenshotAs(OutputType.FILE)",
				ProbeTemplate: "take a screenshot", ProbeExample: "take a screenshot"},
			{ID: "appium_java.locator.id", Name: "By.id()", Category: CatData, Level: Full,
				EBNF: `("By" | "AppiumBy") ".id(" STRING ")"`, Example: `AppiumBy.id("loginBtn")`, ProbeTemplate: "#$1", ProbeExample: "#loginBtn"},
			{ID: "appium_java.locator.a11y", Name: "AppiumBy.accessibilityId()", Category: CatData, Level: Full,
				EBNF: `("By" | "AppiumBy") ".accessibilityId(" STRING ")"`, Example: `AppiumBy.accessibilityId("Submit")`,
				ProbeTemplate: `"$1"`, ProbeExample: `"Submit"`},
		},
	}
}

// AppiumJS returns the construct catalog for Appium WebdriverIO JS tests.
func AppiumJS() Language {
	return Language{
		Name:           "appium_js",
		DisplayName:    "Appium (JS / WebdriverIO)",
		FileExtensions: []string{".js", ".ts"},
		Version:        "WebdriverIO 8+",
		StructureEBNF: `
TestFile     = { ImportStmt | DescribeBlock } .
DescribeBlock = "describe(" STRING "," AsyncFn ")" .
ItBlock      = "it(" STRING "," AsyncFn ")" .
BeforeBlock  = "before" ("All" | "Each") "(" AsyncFn ")" .
AfterBlock   = "after" ("All" | "Each") "(" AsyncFn ")" .
Statement    = "await" ( SelectorAction | BrowserAction ) ";" .
SelectorAction = "$(" STRING ")" "." Action "(" { Arg } ")" .
BrowserAction  = ( "browser" | "driver" ) "." Action "(" { Arg } ")" .
`,
		Constructs: []Construct{
			{ID: "appium_js.describe", Name: "describe()", Category: CatStructure, Level: Full,
				EBNF: `"describe(" STRING ","`, Example: "describe('Login', () => {", ProbeTemplate: "# $1", ProbeExample: "# Login"},
			{ID: "appium_js.it", Name: "it()", Category: CatStructure, Level: Full,
				EBNF: `"it(" STRING ","`, Example: "it('should login', async () => {", ProbeTemplate: `test "$1"`, ProbeExample: `test "should login"`},
			{ID: "appium_js.before", Name: "beforeEach()", Category: CatLifecycle, Level: Full,
				EBNF: `"before" ("All" | "Each") "("`, Example: "beforeEach(async () => {", ProbeTemplate: "before each", ProbeExample: "before each"},
			{ID: "appium_js.after", Name: "afterEach()", Category: CatLifecycle, Level: Full,
				EBNF: `"after" ("All" | "Each") "("`, Example: "afterEach(async () => {", ProbeTemplate: "after each", ProbeExample: "after each"},
			{ID: "appium_js.clickA11y", Name: "$('~x').click()", Category: CatAction, Level: Full,
				EBNF: `"$('~" TEXT "').click()"`, Example: "await $('~Sign In').click()",
				ProbeTemplate: `tap on "$1"`, ProbeExample: `tap on "Sign In"`},
			{ID: "appium_js.clickID", Name: "$('#x').click()", Category: CatAction, Level: Full,
				EBNF: `"$('#" IDENT "').click()"`, Example: "await $('#loginBtn').click()",
				ProbeTemplate: "tap on #$1", ProbeExample: "tap on #loginBtn"},
			{ID: "appium_js.setValue", Name: "$('~x').setValue()", Category: CatAction, Level: Full,
				EBNF: `"$('" SELECTOR "').setValue(" STRING ")"`, Example: "await $('~emailInput').setValue('user@test.com')",
				ProbeTemplate: `type "$2" into "$1"`, ProbeExample: `type "user@test.com" into "emailInput"`},
			{ID: "appium_js.clearValue", Name: "$('~x').clearValue()", Category: CatAction, Level: Full,
				EBNF: `"$('" SELECTOR "').clearValue()"`, Example: "await $('~field').clearValue()",
				ProbeTemplate: `clear "$1"`, ProbeExample: `clear "field"`},
			{ID: "appium_js.waitForExist", Name: "$('~x').waitForExist()", Category: CatWait, Level: Full,
				EBNF: `"$('" SELECTOR "').waitForExist(" { OPT } ")"`, Example: "await $('~title').waitForExist({ timeout: 5000 })",
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "title" appears`},
			{ID: "appium_js.waitForDisplayed", Name: "$('~x').waitForDisplayed()", Category: CatWait, Level: Full,
				EBNF: `"$('" SELECTOR "').waitForDisplayed(" { OPT } ")"`, Example: "await $('~title').waitForDisplayed()",
				ProbeTemplate: `wait until "$1" appears`, ProbeExample: `wait until "title" appears`},
			{ID: "appium_js.pause", Name: "browser.pause()", Category: CatWait, Level: Full,
				EBNF: `("browser" | "driver") ".pause(" INT ")"`, Example: "await browser.pause(2000)",
				ProbeTemplate: "wait $1 seconds", ProbeExample: "wait 2 seconds"},
			{ID: "appium_js.back", Name: "browser.back()", Category: CatNavigation, Level: Full,
				EBNF: `("browser" | "driver") ".back()"`, Example: "await browser.back()",
				ProbeTemplate: "go back", ProbeExample: "go back"},
			{ID: "appium_js.screenshot", Name: "browser.saveScreenshot()", Category: CatScreenshot, Level: Full,
				EBNF: `("browser" | "driver") "." ("saveScreenshot" | "takeScreenshot") "(" STRING ")"`,
				Example: "await browser.saveScreenshot('home.png')",
				ProbeTemplate: `take a screenshot called "$1"`, ProbeExample: `take a screenshot called "home.png"`},
		},
	}
}
