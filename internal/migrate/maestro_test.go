package migrate_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/migrate"
	"github.com/alphawavesystems/flutter-probe/internal/parser"
)

func TestConvertYAML_LoginFlow(t *testing.T) {
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
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(warnings) > 0 {
		t.Logf("warnings: %v", warnings)
	}

	assertContains(t, probe, "open the app")
	assertContains(t, probe, `tap on "Sign In"`)
	assertContains(t, probe, `type "user@test.com"`)
	assertContains(t, probe, `see "Dashboard"`)
}

func TestConvertYAML_Assertions(t *testing.T) {
	yaml := `---
- assertVisible: "Welcome"
- assertNotVisible: "Loading"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, `see "Welcome"`)
	assertContains(t, probe, `don't see "Loading"`)
}

func TestConvertYAML_Navigation(t *testing.T) {
	yaml := `---
- back
- scroll
- swipe:
    direction: UP
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "go back")
	assertContains(t, probe, "scroll")
	assertContains(t, probe, "swipe up")
}

func TestConvertYAML_Screenshot(t *testing.T) {
	yaml := `---
- launchApp
- takeScreenshot: "home_screen"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "take a screenshot called")
	assertContains(t, probe, "home_screen")
}

func TestConvertYAML_Wait(t *testing.T) {
	yaml := `---
- waitForAnimationToEnd
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "wait for the page to load")
}

func TestConvertYAML_LongPress(t *testing.T) {
	yaml := `---
- longPressOn: "Delete"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "long press on")
}

func TestConvertYAML_EmptyFlow(t *testing.T) {
	probe, _, err := migrate.ConvertYAML("---\n")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	// Should produce a minimal test block
	if probe == "" {
		t.Error("expected non-empty output")
	}
}

func TestConvertYAML_UnknownCommand_GeneratesComment(t *testing.T) {
	yaml := `---
- unknownFutureCommand: "value"
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "# TODO")
	if len(warnings) == 0 {
		t.Error("expected warnings for unknown command")
	}
}

// ---- G-3: 2.x syntax hardening ----
//
// The four commands named in the roadmap (setPermissions, relativePoint,
// retry, assertScreenshot) turned out not to appear anywhere in
// nect-flutter's real 76-flow suite — the actual, evidence-based gaps found
// by running the converter against that corpus were extendedWaitUntil (341
// uses), scrollUntilVisible (73), and eraseText (27). Both sets are covered
// below.

func mustParseProbe(t *testing.T, probe string) {
	t.Helper()
	if _, err := parser.ParseFile(probe); err != nil {
		t.Fatalf("converted output does not parse as valid ProbeScript: %v\noutput:\n%s", err, probe)
	}
}

func TestConvertYAML_SetPermissions(t *testing.T) {
	yaml := `---
- setPermissions:
    permissions:
      camera: allow
      location: deny
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, `allow permission "camera"`)
	assertContains(t, probe, `deny permission "location"`)
	mustParseProbe(t, probe)
}

func TestConvertYAML_SetPermissions_UnknownValueWarns(t *testing.T) {
	yaml := `---
- setPermissions:
    permissions:
      microphone: unset
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "# TODO")
	if len(warnings) == 0 {
		t.Error("expected a warning for an unset permission value")
	}
}

func TestConvertYAML_RelativePoint_NotSilentlyMangled(t *testing.T) {
	yaml := `---
- tapOn:
    point: "47%,83%"
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "# TODO")
	assertContains(t, probe, "47%,83%")
	if strings.Contains(probe, "map[point:") {
		t.Errorf("expected the relative point to be flagged, not dumped as a raw Go map: %s", probe)
	}
	if len(warnings) == 0 {
		t.Error("expected a warning for a relativePoint selector")
	}
}

func TestConvertYAML_Retry(t *testing.T) {
	yaml := `---
- retry:
    maxRetries: 3
    commands:
      - tapOn: "Submit"
      - assertVisible: "Success"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "retry 3 times")
	assertContains(t, probe, `tap on "Submit"`)
	assertContains(t, probe, `see "Success"`)
	mustParseProbe(t, probe)
}

func TestConvertYAML_Repeat_NestedStepsConverted(t *testing.T) {
	// Regression guard: `repeat`'s nested commands used to be dropped
	// entirely with a "requires manual migration" warning. They must now
	// actually convert, the same as retry's.
	yaml := `---
- repeat:
    times: 5
    commands:
      - tapOn: "Next"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "repeat 5 times")
	assertContains(t, probe, `tap on "Next"`)
	mustParseProbe(t, probe)
}

func TestConvertYAML_AssertScreenshot(t *testing.T) {
	yaml := `---
- assertScreenshot:
    name: "home-screen"
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, `compare screenshot "home-screen"`)
	mustParseProbe(t, probe)
}

func TestConvertYAML_AssertScreenshot_BareString(t *testing.T) {
	yaml := `---
- assertScreenshot
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "compare screenshot")
	mustParseProbe(t, probe)
}

func TestConvertYAML_ExtendedWaitUntil(t *testing.T) {
	yaml := `---
- extendedWaitUntil:
    visible: "ACCOUNT"
    timeout: 10000
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, `wait until "ACCOUNT" appears`)
	if len(warnings) == 0 {
		t.Error("expected a warning about the dropped custom timeout")
	}
	mustParseProbe(t, probe)
}

func TestConvertYAML_ScrollUntilVisible(t *testing.T) {
	yaml := `---
- scrollUntilVisible:
    element:
      id: "delete_button"
    direction: DOWN
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "scroll down")
	if len(warnings) == 0 {
		t.Error("expected a warning that scrollUntilVisible is approximated")
	}
	mustParseProbe(t, probe)
}

func TestConvertYAML_EraseText(t *testing.T) {
	yaml := `---
- tapOn:
    id: "login_email_field"
- eraseText: 60
`
	probe, warnings, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "clear")
	if len(warnings) == 0 {
		t.Error("expected a warning that eraseText is approximated as clear")
	}
	mustParseProbe(t, probe)
}

func TestConvertYAML_SetLocation(t *testing.T) {
	yaml := `---
- setLocation:
    latitude: 37.7749
    longitude: -122.4194
`
	probe, _, err := migrate.ConvertYAML(yaml)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	assertContains(t, probe, "set location 37.7749, -122.4194")
	mustParseProbe(t, probe)
}

// ---- Self-healer tests ----

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
