package migrate_test

import (
	"strings"
	"testing"

	"github.com/alphawavesystems/flutter-probe/internal/migrate"
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

// ---- Self-healer tests ----

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}
