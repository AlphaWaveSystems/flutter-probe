package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workspaceWith makes a workspace whose probe.yaml sets the two AI-context switches, and returns its path.
func workspaceWith(t *testing.T, runResults, widgetTree bool) string {
	t.Helper()
	dir := t.TempDir()
	body := fmt.Sprintf("studio:\n  ai_context:\n    run_results: %t\n    widget_tree: %t\n", runResults, widgetTree)
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// realSwitches makes the chat read the switches from the workspace's probe.yaml, as it does in the app.
func realSwitches(t *testing.T) {
	t.Helper()
	prev := contextSwitches
	t.Cleanup(func() { contextSwitches = prev })
	contextSwitches = workspaceSwitches
}

// recordingTree makes the device answer with a fixed widget tree and counts how often it was asked.
func recordingTree(t *testing.T, tree string) *int {
	t.Helper()
	calls := new(int)
	prev := fetchTree
	t.Cleanup(func() { fetchTree = prev })
	fetchTree = func(*App) (string, error) {
		*calls++
		return tree, nil
	}
	return calls
}

// A widget tree with a secret in a key, an obscured field, and a credential-looking token in a text key.
const secretTree = "Scaffold(key=null)\n" +
	"  TextField(key=[<'email'>])\n" +
	"  EditableText(key=[<'pin_code'>])\n" +
	"  Text(key=[<'sk-abcdefgh12345678'>])\n" +
	"  Text(key=[<'password_hint'>])\n" +
	"  EditableText(key=[<'notes'>], obscureText: true)\n"

func TestAIContextIsOffUnlessTheWorkspaceTurnsItOn(t *testing.T) {
	noSection := t.TempDir()
	if err := os.WriteFile(filepath.Join(noSection, "probe.yaml"), []byte("agent:\n  port: 48686\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, dir := range map[string]string{"no studio section": noSection, "no probe.yaml": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			got, err := readAIContext(dir)
			if err != nil || got.RunResults || got.WidgetTree {
				t.Fatalf("switches = %+v, err = %v; want both off", got, err)
			}
		})
	}
	if sw := workspaceSwitches(&App{}); sw.RunResults || sw.WidgetTree {
		t.Fatal("with no workspace open, both switches must be off")
	}
}

func TestChatSendsNeitherTheRunNorTheScreenWhenTheSettingIsOff(t *testing.T) {
	f := newFakeProvider(t, 200, "ok")
	useProvider(t, f, "test-key")
	realSwitches(t)
	calls := recordingTree(t, secretTree)

	a := failedRunApp()
	a.workspace = workspaceWith(t, false, false)
	if _, err := a.Chat([]ChatMessage{{Role: "user", Content: "why?"}}, "test \"x\""); err != nil {
		t.Fatal(err)
	}
	sys := f.sent(t).System
	if !strings.Contains(sys, "Current file:") {
		t.Fatal("the open file should still be sent")
	}
	for _, leak := range []string{"Last run:", "FAILED test", "Widget tree", "Scaffold"} {
		if strings.Contains(sys, leak) {
			t.Fatalf("with the setting off, the prompt still carries %q", leak)
		}
	}
	if *calls != 0 {
		t.Fatalf("the device was asked for its widget tree %d time(s) with the setting off", *calls)
	}
}

func TestChatSendsTheRunAndTheRedactedScreenWhenTheSettingIsOn(t *testing.T) {
	f := newFakeProvider(t, 200, "ok")
	useProvider(t, f, "test-key")
	realSwitches(t)
	calls := recordingTree(t, secretTree)

	a := failedRunApp()
	a.workspace = workspaceWith(t, true, true)
	if _, err := a.Chat([]ChatMessage{{Role: "user", Content: "why?"}}, ""); err != nil {
		t.Fatal(err)
	}
	sys := f.sent(t).System
	for _, want := range []string{"Last run:", `FAILED test "sign in"`, "Widget tree of the connected device", "Scaffold(key=null)", "TextField(key=[<'email'>])"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("with the settings on, the prompt lacks %q", want)
		}
	}
	if *calls != 1 {
		t.Fatalf("the device was asked for its tree %d time(s), want 1", *calls)
	}
	// A plain Text label keeps its key (it holds no user input); the editable fields and the token must not show.
	for _, secret := range []string{"sk-abcdefgh12345678", "pin_code", "'notes'"} {
		if strings.Contains(sys, secret) {
			t.Fatalf("the prompt carries %q, which the redaction must remove", secret)
		}
	}
}

func TestEachSwitchSendsOnlyItsOwnPart(t *testing.T) {
	cases := []struct {
		name          string
		runResults    bool
		widgetTree    bool
		wantRun, want bool
	}{
		{name: "run results only", runResults: true, widgetTree: false, wantRun: true, want: false},
		{name: "widget tree only", runResults: false, widgetTree: true, wantRun: false, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeProvider(t, 200, "ok")
			useProvider(t, f, "test-key")
			realSwitches(t)
			calls := recordingTree(t, secretTree)

			a := failedRunApp()
			a.workspace = workspaceWith(t, tc.runResults, tc.widgetTree)
			if _, err := a.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err != nil {
				t.Fatal(err)
			}
			sys := f.sent(t).System
			if got := strings.Contains(sys, "Last run:"); got != tc.wantRun {
				t.Fatalf("last run in prompt = %v, want %v", got, tc.wantRun)
			}
			if got := strings.Contains(sys, "Widget tree of the connected device"); got != tc.want {
				t.Fatalf("widget tree in prompt = %v, want %v", got, tc.want)
			}
			if (*calls == 1) != tc.want {
				t.Fatalf("device asked %d time(s), want asked = %v", *calls, tc.want)
			}
		})
	}
}

func TestSuggestSelectorFixNeedsTheRunSwitch(t *testing.T) {
	f := newFakeProvider(t, 200, "```probe\ntap \"Sign in\"\n```\nok")
	useProvider(t, f, "test-key")
	realSwitches(t)

	a := failedRunApp()
	a.workspace = workspaceWith(t, false, true)
	_, err := a.SuggestSelectorFix("")
	if err == nil || !strings.Contains(err.Error(), "Include run results") {
		t.Fatalf("err = %v, want a message that names the run-results setting", err)
	}
	if n := f.requests.Load(); n != 0 {
		t.Fatalf("%d request(s) sent with the run-results setting off", n)
	}
}

func TestCredentialsAreRedactedBeforeTheyReachTheProvider(t *testing.T) {
	f := newFakeProvider(t, 200, "ok")
	useProvider(t, f, "test-key")
	realSwitches(t)
	recordingTree(t, "Text(key=[<'ghp_abcdefghijklmnopqrstuvwxyz0123456789'>])\n"+
		"Text(key=[<'Bearer abcdefghijklmnop'>])\n")

	a := failedRunApp()
	a.workspace = workspaceWith(t, true, true)
	a.track.step(RunStep{File: "tests/login.probe", Test: "sign in", Line: 5, Description: `tap "Token"`, Status: "failed",
		Error: `value sk-liveabcdefgh12345678 rejected`})
	if _, err := a.Chat([]ChatMessage{{Role: "user", Content: "hi"}}, ""); err != nil {
		t.Fatal(err)
	}
	sys := f.sent(t).System
	for _, secret := range []string{"ghp_abcdefghijklmnopqrstuvwxyz0123456789", "abcdefghijklmnop", "sk-liveabcdefgh12345678"} {
		if strings.Contains(sys, secret) {
			t.Fatalf("the prompt carries %q", secret)
		}
	}
	if !strings.Contains(sys, "[redacted]") {
		t.Fatal("the prompt shows no redaction marker")
	}
}

func TestRedactCredentialsCoversTheCommonShapes(t *testing.T) {
	cases := map[string]string{
		"key sk-abcdefgh12345678":                                                 "key [redacted]",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789":                          "token [redacted]",
		"Authorization: Bearer abc.def-ghi_jkl":                                   "Authorization: [redacted]",
		"digest 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08": "digest [redacted]",
		"blob AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/AbCdEfGhIj":                   "blob [redacted]",
		"_WidgetStateMachineForLongScreenNameThatIsLong":                          "_WidgetStateMachineForLongScreenNameThatIsLong",
		"task-manager-widget":                                                     "task-manager-widget",
	}
	for in, want := range cases {
		if got := redactCredentials(in); got != want {
			t.Errorf("redactCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactTreeHidesObscuredAndSecretFields(t *testing.T) {
	in := strings.Join([]string{
		"TextField(key=[<'email'>])",
		"EditableText(key=[<'pin_code'>])",
		"EditableText(key=[<'notes'>], obscureText: true)",
		"Text(key=[<'password_hint'>])",
		"TextFormField(key=[<'api_key'>])",
		"Text(key=[<'sk-abcdefgh12345678'>])",
		"Scaffold(key=null)",
	}, "\n")
	want := strings.Join([]string{
		"TextField(key=[<'email'>])",
		"EditableText(key=[redacted], obscured)",
		"EditableText(key=[redacted], obscured)",
		"Text(key=[<'password_hint'>])",
		"TextFormField(key=[redacted], obscured)",
		"Text(key=[<'[redacted]'>])",
		"Scaffold(key=null)",
	}, "\n")
	if got := redactTree(in); got != want {
		t.Fatalf("redactTree =\n%s\nwant\n%s", got, want)
	}
}

func TestSaveAndLoadKeepTheSwitchesAndTheRestOfProbeYaml(t *testing.T) {
	dir := t.TempDir()
	existing := "agent:\n  port: 1234\nrecipes_folder: tests/recipes\n"
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 1234, AIContextRunResults: true}); err != nil {
		t.Fatal(err)
	}
	got, err := a.LoadWorkspaceSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AIContextRunResults || got.AIContextWidgetTree || got.AgentPort != 1234 {
		t.Fatalf("loaded = %+v", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml"))
	for _, want := range []string{"recipes_folder", "run_results: true"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("probe.yaml lacks %q after save:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "widget_tree") {
		t.Fatal("an off switch was written to probe.yaml")
	}

	// Both switches off again: the studio section goes, and the rest of the file stays.
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 1234}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "probe.yaml"))
	if strings.Contains(string(data), "studio") || !strings.Contains(string(data), "recipes_folder") {
		t.Fatalf("after switching both off, probe.yaml is:\n%s", data)
	}
}

func TestSaveRefusesAStudioKeyThatIsNotAMap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.yaml"), []byte("studio: enabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AIContextRunResults: true})
	if err == nil || !strings.Contains(err.Error(), "not a map") {
		t.Fatalf("err = %v, want a refusal that the studio key is not a map", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "probe.yaml")); string(data) != "studio: enabled\n" {
		t.Fatalf("the refused save changed probe.yaml: %q", data)
	}
}
