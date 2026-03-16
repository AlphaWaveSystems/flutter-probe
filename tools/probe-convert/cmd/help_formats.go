package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var helpCmd = &cobra.Command{
	Use:   "formats [format]",
	Short: "Show format-specific conversion docs and examples",
	Long:  "Display detailed information about a specific format's conversion, including recognized constructs, example input/output, and known limitations.\n\nUsage: probe-convert formats <maestro|gherkin|robot|detox|appium>",
}

var helpMaestroCmd = &cobra.Command{
	Use:   "maestro",
	Short: "Maestro YAML conversion docs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(`Maestro YAML → ProbeScript Conversion

Recognized file types: .yaml, .yml (with Maestro markers like appId, tapOn, launchApp)

Supported commands:
  launchApp          → open the app
  stopApp            → close the app
  tapOn              → tap on "text" / tap on #id
  longPressOn        → long press on "text"
  doubleTapOn        → double tap on "text"
  inputText          → type "text"
  assertVisible      → see "text"
  assertNotVisible   → don't see "text"
  back               → go back
  scroll/scrollDown  → scroll down
  scrollUp           → scroll up
  swipe              → swipe <direction>
  takeScreenshot     → take a screenshot called "name"
  waitForAnimation   → wait for the page to load
  wait               → wait N seconds
  runFlow            → use "path"
  repeat             → repeat N times
  clearState         → clear app data
  hideKeyboard       → close keyboard
  openLink           → open "url"

Example:
  Input (login.yaml):
    appId: com.example.app
    ---
    - launchApp
    - tapOn: "Sign In"
    - inputText: "user@test.com"
    - assertVisible: "Dashboard"

  Output (login.probe):
    # Converted from Maestro — app: com.example.app
    test "login"
      open the app
      tap on "Sign In"
      type "user@test.com"
      see "Dashboard"

Known limitations:
  - evalScript requires manual Dart conversion
  - repeat with nested steps needs manual migration
  - Platform conditionals (ifdef, skipOn, onlyOn) need manual handling
  - setLocation is not yet supported
`)
	},
}

var helpGherkinCmd = &cobra.Command{
	Use:   "gherkin",
	Short: "Gherkin/Cucumber conversion docs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(`Gherkin (.feature) → ProbeScript Conversion

Recognized file types: .feature

Supported constructs:
  Feature:            → file comment
  @tag                → @tag annotations
  Background:         → before each block
  Scenario:           → test "name"
  Scenario Outline:   → test "name" with examples
  Examples: table     → with examples: block
  <variable>          → <variable> (same syntax)

Step pattern matching (~30 patterns):
  tap/click/press "X"          → tap on "X"
  type/enter "X" into "Y"     → type "X" into "Y"
  should see "X"               → see "X"
  should not see "X"           → don't see "X"
  wait N seconds               → wait N seconds
  wait until "X" appears       → wait until "X" appears
  swipe left/right/up/down     → swipe <direction>
  go back / press back         → go back
  take a screenshot            → take a screenshot
  app is launched/opened       → open the app
  long press "X"               → long press on "X"
  clear app data               → clear app data
  restart the app              → restart the app

Example:
  Input (login.feature):
    Feature: Login
    @smoke
    Scenario: User can log in
      Given the app is launched
      When I tap on "Sign In"
      And I type "user@test.com" into "Email"
      Then I should see "Dashboard"

  Output (login.probe):
    # Converted from Gherkin — Feature: Login
    test "User can log in"
      @smoke
      open the app
      tap on "Sign In"
      type "user@test.com" into "Email"
      see "Dashboard"
`)
	},
}

var helpRobotCmd = &cobra.Command{
	Use:   "robot",
	Short: "Robot Framework conversion docs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(`Robot Framework (.robot) → ProbeScript Conversion

Recognized file types: .robot

Supported constructs:
  *** Test Cases ***    → test blocks
  *** Keywords ***      → recipe blocks
  [Tags]                → @tag annotations
  [Arguments]           → recipe parameters
  [Template] + rows     → with examples: block
  Suite/Test Setup      → before each
  Suite/Test Teardown   → after each
  ${VAR}                → <var>

Keyword mappings:
  Click Element id=x          → tap on #x
  Click Element text=x        → tap on "x"
  Input Text id=x val         → type "val" into #x
  Page Should Contain Text x  → see "x"
  Wait Until Element Visible  → wait until ... appears
  Capture Page Screenshot     → take a screenshot
  Sleep 5s                    → wait 5 seconds
  Go Back                     → go back

Locator parsing:
  id=x      → #x
  text=x    → "x"
  xpath=... → extract text attribute → "X"
  accessibility_id=x → "x"
`)
	},
}

var helpDetoxCmd = &cobra.Command{
	Use:   "detox",
	Short: "Detox JS/TS conversion docs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(`Detox (.js/.ts) → ProbeScript Conversion

Recognized file types: .js, .ts (with element(by.) or device.launchApp patterns)

Supported patterns:
  element(by.id('x')).tap()         → tap on #x
  element(by.text('x')).tap()       → tap on "x"
  element(by.id('x')).typeText('v') → type "v" into #x
  element(by.id('x')).longPress()   → long press on #x
  element(by.id('x')).swipe('dir')  → swipe dir on #x
  expect(by.text('x')).toBeVisible  → see "x"
  expect(...).not.toExist()         → don't see ...
  waitFor(by.text('x')).toBeVisible → wait until "x" appears
  device.launchApp()                → open the app
  device.reloadReactNative()        → restart the app

Block mapping:
  describe('name')  → comment
  it('name')        → test "name"
  beforeAll/Each    → before each
  afterAll/Each     → after each

Note: Continuation lines (starting with .) are joined with the previous line.
`)
	},
}

var helpAppiumCmd = &cobra.Command{
	Use:   "appium",
	Short: "Appium (Python/Java/JS) conversion docs",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(`Appium (Python/Java/JS) → ProbeScript Conversion

Recognized file types:
  .py        → Appium Python
  .java/.kt  → Appium Java
  .js/.ts    → Appium JS (when no Detox patterns found)

Python patterns:
  find_element(By.ID, 'x').click()              → tap on #x
  find_element(By.ACCESSIBILITY_ID, 'x').click() → tap on "x"
  .send_keys('text')                            → type "text"
  WebDriverWait(...).until(...)                  → wait until ... appears
  def test_name(self):                          → test "name"
  def setUp/tearDown                            → before each / after each

Java patterns:
  findElement(By.id("x")).click()               → tap on #x
  findElement(AppiumBy.accessibilityId("x"))    → tap on "x"
  .sendKeys("text")                             → type "text"
  @Test void testName()                         → test "name"
  @Before/@After                                → before each / after each

JS patterns:
  $('~x').click()    → tap on "x"
  $('#x').click()     → tap on #x
  .setValue('text')   → type "text"
  it('name', ...)    → test "name"

Note: This is a best-effort converter. Appium tests vary widely in structure
and abstraction level. Review generated .probe files carefully.
`)
	},
}

func init() {
	helpCmd.AddCommand(helpMaestroCmd)
	helpCmd.AddCommand(helpGherkinCmd)
	helpCmd.AddCommand(helpRobotCmd)
	helpCmd.AddCommand(helpDetoxCmd)
	helpCmd.AddCommand(helpAppiumCmd)
	rootCmd.AddCommand(helpCmd)
}
