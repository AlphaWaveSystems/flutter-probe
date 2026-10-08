package migrate_test

import (
	"os"
	"path/filepath"
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
	assertContains(t, probe, "wait for idle")
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

func TestConvertFile_RunFlowBecomesRecipeAndUse(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "in", "helpers"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "in", "flows"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "in", "helpers", "login-flow.yaml"), []byte("- tapOn: \"Login\"\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "in", "flows", "a.yaml"),
		[]byte("- launchApp\n- runFlow: ../helpers/login-flow.yaml\n"), 0o644))

	files, err := migrate.DiscoverYAMLFiles([]string{filepath.Join(dir, "in")})
	must(err)
	opts := migrate.Options{RecipeFiles: migrate.RunFlowTargets(files)}
	out := filepath.Join(dir, "out")
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.Path), ".yaml")
		_, err := migrate.ConvertFileWith(f.Path, filepath.Join(out, f.RelDir, base+".probe"), opts)
		must(err)
	}

	flow, err := os.ReadFile(filepath.Join(out, "flows", "a.probe"))
	must(err)
	if !strings.Contains(string(flow), `use "../helpers/login-flow.probe"`) || !strings.Contains(string(flow), "flow login flow") {
		t.Errorf("flow should use the converted recipe file and call the recipe:\n%s", flow)
	}
	if strings.Contains(string(flow), ".yaml") {
		t.Errorf("no .yaml may remain in the converted flow:\n%s", flow)
	}
	helper, err := os.ReadFile(filepath.Join(out, "helpers", "login-flow.probe"))
	must(err)
	if !strings.HasPrefix(string(helper), `recipe "flow login flow"`) {
		t.Errorf("helper should be a recipe:\n%s", helper)
	}
}

func TestConvertYAML_EvalScriptIsAComment(t *testing.T) {
	probe, warns, err := migrate.ConvertYAML("- evalScript: \"output.x = 1\"\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(probe, "\n") {
		l = strings.TrimSpace(l)
		if strings.Contains(l, "evalScript") && !strings.HasPrefix(l, "#") {
			t.Errorf("evalScript must be a # comment, got %q", l)
		}
	}
	if len(warns) == 0 {
		t.Error("expected a warning")
	}
}

func TestConvertYAML_RegexSelectorGetsATodo(t *testing.T) {
	probe, warns, err := migrate.ConvertYAML("- extendedWaitUntil:\n    visible: \"Got.*it\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probe, "# TODO: Maestro matches") || len(warns) == 0 {
		t.Errorf("regex selector should be flagged:\n%s\n%v", probe, warns)
	}
}

func TestConvertYAML_PlainAlternationBecomesWaitAny(t *testing.T) {
	probe, warns, err := migrate.ConvertYAML("- extendedWaitUntil:\n    visible: \"Got it|Login to X|Open menu\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probe, `wait until any of "Got it", "Login to X", "Open menu" appears`) || strings.Contains(probe, "TODO") {
		t.Errorf("plain alternation should become wait-any without a TODO:\n%s", probe)
	}
	for _, w := range warns {
		if strings.Contains(w, "regex") {
			t.Errorf("unexpected regex warning %q", w)
		}
	}
}

func TestConvertYAML_PressKey(t *testing.T) {
	probe, _, err := migrate.ConvertYAML("- pressKey: Enter\n- pressKey: Back\n- pressKey: Home\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probe, "press enter") || !strings.Contains(probe, "go back") {
		t.Errorf("Enter and Back must convert:\n%s", probe)
	}
	if strings.Contains(probe, "press key") || strings.Contains(probe, "press the home") {
		t.Errorf("no made-up steps may remain:\n%s", probe)
	}
	if !strings.Contains(probe, "# TODO: pressKey Home") {
		t.Errorf("unsupported keys need a TODO:\n%s", probe)
	}
}

func TestConvertFile_RunFlowOutsideRootWarns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	flow := filepath.Join(dir, "flows", "a.yaml")
	if err := os.WriteFile(flow, []byte("- runFlow: ../helpers/login.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(flow)
	out := filepath.Join(dir, "out", "a.probe")
	_, err := migrate.ConvertFileWith(flow, out, migrate.Options{Converted: map[string]bool{abs: true}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "outside the migrate root") {
		t.Errorf("expected a TODO about the helper outside the root:\n%s", b)
	}
}

func TestConvertYAML_ConditionalRunFlow(t *testing.T) {
	probe, _, err := migrate.ConvertYAML(`- runFlow:
    when:
      visible: "Login to X"
    commands:
      - tapOn: "Login"
- runFlow:
    when:
      notVisible: "Home"
    commands:
      - tapOn: "Skip"
`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probe, "if \"Login to X\" appears\n    tap on \"Login\"") {
		t.Errorf("visible condition should become an if block:\n%s", probe)
	}
	if !strings.Contains(probe, "if \"Home\" appears") || !strings.Contains(probe, "otherwise\n    tap on \"Skip\"") {
		t.Errorf("notVisible should become if/otherwise:\n%s", probe)
	}
	if _, err := parser.ParseFile(probe); err != nil {
		t.Errorf("generated file must parse: %v\n%s", err, probe)
	}
}

func TestConvertYAML_LiteralRegexForms(t *testing.T) {
	probe, warns, err := migrate.ConvertYAML(`- assertVisible: ".*Professional Profile.*"
- assertVisible: "Comments (1)"
- extendedWaitUntil:
    visible: "No offers at this time\\.|Subscribe Now"
- assertVisible: "Joined .*!"
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`see "Professional Profile"`, `see "Comments (1)"`,
		`wait until any of "No offers at this time.", "Subscribe Now" appears`} {
		if !strings.Contains(probe, want) {
			t.Errorf("missing %q in:\n%s", want, probe)
		}
	}
	if strings.Count(probe, "# TODO") != 1 || len(warns) != 1 {
		t.Errorf("only the mid-wildcard selector should keep a TODO, got %d TODO, warns %v:\n%s", strings.Count(probe, "# TODO"), warns, probe)
	}
}

func TestConvertYAML_OptionalTypeIntoAndAnimation(t *testing.T) {
	probe, _, err := migrate.ConvertYAML(`- tapOn:
    text: "Allow"
    optional: true
- tapOn:
    id: login_email_field
- eraseText
- inputText: "a@b.c"
- tapOn:
    id: login_password_field
- inputText: "secret"
- assertVisible:
    text: "Welcome"
    optional: true
- waitForAnimationToEnd
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`tap on "Allow" if visible`, `clear #login_email_field`, `type "a@b.c" into #login_email_field`,
		`type "secret" into #login_password_field`, `see "Welcome" optional`, "wait for idle"} {
		if !strings.Contains(probe, want) {
			t.Errorf("missing %q in:\n%s", want, probe)
		}
	}
	if _, err := parser.ParseFile(probe); err != nil {
		t.Errorf("must parse: %v\n%s", err, probe)
	}
}
