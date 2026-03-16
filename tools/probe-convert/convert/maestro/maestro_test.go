package maestro

import (
	"strings"
	"testing"
)

func TestConvert_LoginFlow(t *testing.T) {
	yaml := `appId: com.example.app
---
- launchApp
- tapOn: "Sign In"
- tapOn: "Email"
- inputText: "user@test.com"
- tapOn: "Password"
- inputText: "pass123"
- tapOn: "Continue"
- assertVisible: "Dashboard"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "login.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	assertContains(t, result.ProbeCode, "open the app")
	assertContains(t, result.ProbeCode, `tap on "Sign In"`)
	assertContains(t, result.ProbeCode, `type "user@test.com"`)
	assertContains(t, result.ProbeCode, `see "Dashboard"`)
	assertContains(t, result.ProbeCode, `test "login"`)
	assertContains(t, result.ProbeCode, "com.example.app")
}

func TestConvert_Assertions(t *testing.T) {
	yaml := `---
- assertVisible: "Welcome"
- assertNotVisible: "Loading"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "test.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, `see "Welcome"`)
	assertContains(t, result.ProbeCode, `don't see "Loading"`)
}

func TestConvert_Navigation(t *testing.T) {
	yaml := `---
- back
- scroll
- swipe:
    direction: UP
`
	c := New()
	result, err := c.Convert([]byte(yaml), "nav.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "go back")
	assertContains(t, result.ProbeCode, "scroll")
	assertContains(t, result.ProbeCode, "swipe up")
}

func TestConvert_Screenshot(t *testing.T) {
	yaml := `---
- launchApp
- takeScreenshot: "home_screen"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "shot.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "take a screenshot called")
	assertContains(t, result.ProbeCode, "home_screen")
}

func TestConvert_WaitForAnimation(t *testing.T) {
	yaml := `---
- waitForAnimationToEnd
`
	c := New()
	result, err := c.Convert([]byte(yaml), "wait.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "wait for the page to load")
}

func TestConvert_LongPress(t *testing.T) {
	yaml := `---
- longPressOn: "Delete"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "press.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "long press on")
}

func TestConvert_UnknownCommand(t *testing.T) {
	yaml := `---
- unknownFutureCommand: "value"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "unknown.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "# TODO")
	if len(result.Warnings) == 0 {
		t.Error("expected warnings for unknown command")
	}
}

func TestConvert_EmptyFlow(t *testing.T) {
	c := New()
	result, err := c.Convert([]byte("---\n"), "empty.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if result.ProbeCode == "" {
		t.Error("expected non-empty output")
	}
}

func TestConvert_IDSelector(t *testing.T) {
	yaml := `---
- tapOn:
    id: "loginButton"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "id.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "tap on #loginButton")
}

func TestConvert_ClearState(t *testing.T) {
	yaml := `---
- clearState
`
	c := New()
	result, err := c.Convert([]byte(yaml), "clear.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// clearState as string command → unknown, but as map → clear app data
	// In this case it's a string command (no value)
	if result.ProbeCode == "" {
		t.Error("expected non-empty output")
	}
}

func TestConvert_Repeat(t *testing.T) {
	yaml := `---
- repeat:
    times: 3
    commands:
      - tapOn: "Next"
      - assertVisible: "Page"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "repeat.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "repeat 3 times")
	assertContains(t, result.ProbeCode, `tap on "Next"`)
	assertContains(t, result.ProbeCode, `see "Page"`)
}

func TestConvert_TestNameFromFilename(t *testing.T) {
	yaml := `---
- launchApp
`
	c := New()
	result, err := c.Convert([]byte(yaml), "user_signup_flow.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, `test "user signup flow"`)
}

func TestConvert_EvalScript(t *testing.T) {
	yaml := `---
- evalScript: "console.log('Checking state...')"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "eval.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "run dart:")
	assertContains(t, result.ProbeCode, "// Migrated from Maestro evalScript")
	assertContains(t, result.ProbeCode, "print('Checking state...')")
	assertContains(t, result.ProbeCode, "// Original: console.log")
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(result.Warnings))
	}
}

func TestConvert_EvalScript_Transforms(t *testing.T) {
	yaml := `---
- evalScript: "const x = 1 === 2"
`
	c := New()
	result, err := c.Convert([]byte(yaml), "eval2.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "final x = 1 == 2")
}

func TestConvert_SetAirplaneMode(t *testing.T) {
	yaml := `---
- setAirplaneMode: true
`
	c := New()
	result, err := c.Convert([]byte(yaml), "airplane.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "toggle wifi off")
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(result.Warnings))
	}
}

func TestConvert_SetAirplaneModeOff(t *testing.T) {
	yaml := `---
- setAirplaneMode: false
`
	c := New()
	result, err := c.Convert([]byte(yaml), "airplane.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "toggle wifi on")
}

func TestConvert_Ifdef(t *testing.T) {
	yaml := `---
- ifdef:
    platform: android
`
	c := New()
	result, err := c.Convert([]byte(yaml), "ifdef.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "# Platform conditional (migrated from Maestro ifdef: android)")
	assertContains(t, result.ProbeCode, "run dart:")
	assertContains(t, result.ProbeCode, "Platform.isAndroid")
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(result.Warnings))
	}
}

func TestConvert_SkipOn(t *testing.T) {
	yaml := `---
- skipOn:
    platform: ios
`
	c := New()
	result, err := c.Convert([]byte(yaml), "skipon.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "migrated from Maestro skipOn: ios")
	assertContains(t, result.ProbeCode, "Platform.isIos")
}

func TestConvert_OnlyOn(t *testing.T) {
	yaml := `---
- onlyOn:
    platform: android
`
	c := New()
	result, err := c.Convert([]byte(yaml), "onlyon.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "migrated from Maestro onlyOn: android")
	assertContains(t, result.ProbeCode, "Platform.isAndroid")
	// onlyOn uses the negated guard (same as ifdef)
	assertContains(t, result.ProbeCode, "!Platform.isAndroid")
}

func TestConvert_SetLocation(t *testing.T) {
	yaml := `---
- setLocation:
    latitude: 37.7749
    longitude: -122.4194
`
	c := New()
	result, err := c.Convert([]byte(yaml), "location.yaml")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, result.ProbeCode, "# Location: 37.7749, -122.4194")
	assertContains(t, result.ProbeCode, "adb shell cmd location set-location")
	assertContains(t, result.ProbeCode, "xcrun simctl location set")
	assertContains(t, result.ProbeCode, "run dart:")
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got %d", len(result.Warnings))
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
