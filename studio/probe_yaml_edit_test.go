package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goldenYAML = `# FlutterProbe config for the demo app
project:
  name: Demo        # shown in reports
  app: com.example.demo

# Where the agent listens.
agent:
    port: 48686   # the default
    dial_timeout: 45s

defaults:
  timeout: 30s
  platform: ios    # android | ios | both
  screenshots: "on_failure"

list:
- a
- b
# trailing note
`

func writeGolden(t *testing.T, content string, mode os.FileMode) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "probe.yaml")
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// GOLDEN: a commented probe.yaml with blank lines, odd indentation, quoting and a list. The knobs are added, one value is changed, and the
// result is the original with exactly those lines different: every comment, blank line and unmanaged key is as it was, byte for byte.
func TestSaveKeepsACommentedProbeYAMLByteForByte(t *testing.T) {
	dir, path := writeGolden(t, goldenYAML, 0o644)
	a := &App{}
	loaded, err := a.LoadWorkspaceSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded.AgentPort = 48800
	loaded.TestTimeout = "90s"
	loaded.RetryFailedTests = 2
	if err := a.SaveWorkspaceSettings(dir, loaded); err != nil {
		t.Fatal(err)
	}
	want := strings.NewReplacer(
		"    port: 48686   # the default\n", "    port: 48800   # the default\n",
		"  screenshots: \"on_failure\"\n", "  screenshots: \"on_failure\"\n  test_timeout: 90s\n  retry_failed_tests: 2\n",
	).Replace(goldenYAML)
	if got := read(t, path); got != want {
		t.Errorf("probe.yaml differs from the original in more than the changed lines:\n--- got\n%s\n--- want\n%s", got, want)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644 kept", info.Mode().Perm())
	}
}

// A save with nothing changed writes nothing (the same bytes, and the file is not replaced), whatever the form shows for defaults.
func TestSaveWithNothingChangedLeavesTheFileAlone(t *testing.T) {
	dir, path := writeGolden(t, goldenYAML, 0o640)
	before, _ := os.Stat(path)
	a := &App{}
	loaded, _ := a.LoadWorkspaceSettings(dir)
	if err := a.SaveWorkspaceSettings(dir, loaded); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if read(t, path) != goldenYAML || !os.SameFile(before, after) {
		t.Error("a save with no change rewrote the file")
	}
}

// A form that shows "1m30s" for a file that says 90s is not a change: the line stays as the person wrote it.
func TestAnUnchangedValueKeepsTheSpellingInTheFile(t *testing.T) {
	src := "defaults:\n  test_timeout: 90s # long\n  retry_failed_tests: 1\n"
	dir, path := writeGolden(t, src, 0o600)
	a := &App{}
	loaded, _ := a.LoadWorkspaceSettings(dir)
	loaded.AgentPort = 48800
	if err := a.SaveWorkspaceSettings(dir, loaded); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != src+"agent:\n  port: 48800\n" {
		t.Errorf("got %q", got)
	}
}

// A probe.yaml that does not parse is refused with one line and never overwritten, by either writer.
func TestAnUnparsableProbeYAMLIsNeverWritten(t *testing.T) {
	bad := "agent: [unclosed\n# keep me\n"
	dir, path := writeGolden(t, bad, 0o600)
	a := &App{}
	err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{TestTimeout: "90s"})
	if err == nil || !strings.Contains(err.Error(), "cannot be parsed") || !strings.Contains(err.Error(), "Fix it by hand first") || strings.Contains(err.Error(), dir) {
		t.Errorf("err = %v", err)
	}
	if err := writeAISettings(dir, AISettings{Provider: "local", Endpoint: "http://127.0.0.1:11434/v1", Model: "m", Vision: true}); err == nil {
		t.Error("the AI writer wrote through a broken file")
	}
	if read(t, path) != bad {
		t.Error("the broken file changed")
	}
}

// probe.yaml as a link is not written through (it could lead outside the workspace) and not replaced.
func TestAProbeYAMLThatIsALinkIsNotWritten(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere.yaml")
	if err := os.WriteFile(target, []byte("agent:\n  port: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "probe.yaml")); err != nil {
		t.Skip("no symlinks here")
	}
	err := (&App{}).SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 9})
	if err == nil || !strings.Contains(err.Error(), "link") {
		t.Errorf("err = %v", err)
	}
	if read(t, target) != "agent:\n  port: 1\n" {
		t.Error("the linked file was written")
	}
	if info, _ := os.Lstat(filepath.Join(dir, "probe.yaml")); info.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
}

// The write is a rename of a temp file in the same folder: no temp file is left, also after a refused save, and a new file is private.
func TestTheWriteLeavesNoTempFilesAndANewFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	a := &App{}
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{TestTimeout: "90s"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{TestTimeout: "90s", RetryFailedTests: 99}); err == nil {
		t.Error("retries 99 were accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "probe.yaml" {
		t.Errorf("the folder holds %v", entries)
	}
	if info, _ := os.Stat(filepath.Join(dir, "probe.yaml")); info.Mode().Perm() != 0o600 {
		t.Errorf("a new probe.yaml has mode %v, want 0600", info.Mode().Perm())
	}
	if got := read(t, filepath.Join(dir, "probe.yaml")); got != "defaults:\n  test_timeout: 90s\n" {
		t.Errorf("got %q", got)
	}
}

// Files of other shapes: Windows line endings, a file of only comments, a bare "defaults:" key, a key with a quoted value, no final newline.
func TestEditsOnOtherShapesOfProbeYAML(t *testing.T) {
	a := &App{}
	for name, tc := range map[string]struct{ src, want string }{
		"CRLF":                   {"project:\r\n  app: a.b\r\ndefaults:\r\n  platform: ios\r\n", "project:\r\n  app: a.b\r\ndefaults:\r\n  platform: ios\r\n  test_timeout: 90s\r\n"},
		"only comments":          {"# nothing yet\n", "# nothing yet\ndefaults:\n  test_timeout: 90s\n"},
		"a bare defaults key":    {"defaults:\nproject:\n  app: a.b\n", "defaults:\n  test_timeout: 90s\nproject:\n  app: a.b\n"},
		"a quoted old value":     {"defaults:\n  test_timeout: \"45s\"  # was 45\n", "defaults:\n  test_timeout: 90s  # was 45\n"},
		"no trailing newline":    {"project:\n  app: a.b", "project:\n  app: a.b\ndefaults:\n  test_timeout: 90s"},
		"defaults is not last":   {"defaults:\n  platform: ios\n\n# next\nagent:\n  port: 1\n", "defaults:\n  platform: ios\n  test_timeout: 90s\n\n# next\nagent:\n  port: 1\n"},
		"an indented 4 defaults": {"defaults:\n    platform: ios\n", "defaults:\n    platform: ios\n    test_timeout: 90s\n"},
	} {
		dir, path := writeGolden(t, tc.src, 0o600)
		form, _ := a.LoadWorkspaceSettings(dir) // the form as it was loaded, with one field changed
		form.TestTimeout = "90s"
		if err := a.SaveWorkspaceSettings(dir, form); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := read(t, path); got != tc.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, tc.want)
		}
	}
	// These are refused and left as they were: the line cannot be edited in place.
	for name, src := range map[string]string{
		"a flow mapping":   "defaults: {platform: ios}\n",
		"defaults a list":  "defaults:\n  - a\n",
		"a block scalar":   "defaults:\n  test_timeout: |\n    90s\n",
		"a top-level list": "- a\n- b\n",
	} {
		dir, path := writeGolden(t, src, 0o600)
		if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{TestTimeout: "90s"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if read(t, path) != src {
			t.Errorf("%s: the file changed", name)
		}
	}
}

// Clearing removes the line and the parent that is left empty, and keeps a parent that still has keys or a comment.
func TestClearingRemovesTheLineAndAnEmptiedParent(t *testing.T) {
	a := &App{}
	dir, path := writeGolden(t, "project:\n  app: a.b\ndefaults:\n  test_timeout: 90s\nagent:\n  port: 1\n", 0o600)
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{AgentPort: 1}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "project:\n  app: a.b\nagent:\n  port: 1\n" {
		t.Errorf("got %q", got)
	}
	dir, path = writeGolden(t, "defaults: # my defaults\n  test_timeout: 90s\n", 0o600)
	if err := a.SaveWorkspaceSettings(dir, WorkspaceSettings{}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "defaults: # my defaults\n" {
		t.Errorf("a parent with a comment must stay: %q", got)
	}
}

// The AI form's writer keeps comments and unmanaged keys too, and touches only what changed.
func TestTheAIWriterKeepsCommentsAndOtherKeys(t *testing.T) {
	src := "# demo\nproject:\n  app: a.b   # id\n\nai:\n  provider: local   # was a test\n  model: old\nredact:\n  - secret\n"
	dir, path := writeGolden(t, src, 0o644)
	if err := writeAISettings(dir, AISettings{Provider: "local", Endpoint: "http://127.0.0.1:11434/v1", Model: "new", KeySource: "keychain", Vision: true}); err != nil {
		t.Fatal(err)
	}
	want := "# demo\nproject:\n  app: a.b   # id\n\nai:\n  provider: local   # was a test\n  model: new\n  endpoint: http://127.0.0.1:11434/v1\nredact:\n  - secret\n"
	if got := read(t, path); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
