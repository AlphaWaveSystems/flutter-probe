package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Closing the workspace (the window calls SetWorkspace with an empty path) leaves no folder that reads, lists or writes are allowed in: the one that was
// open is not reachable any more, from the window, the bridge or the automation endpoint (all use the same roots).
func TestAnEmptyWorkspaceReachesNoFolder(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"a\"\n  open the app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(dir)
	if _, err := a.ReadFile(file); err != nil {
		t.Fatalf("the open workspace must be readable: %v", err)
	}
	if roots := a.automationRoots(); len(roots) != 1 {
		t.Fatalf("roots with a workspace open = %v", roots)
	}
	a.SetWorkspace("")
	if roots := a.automationRoots(); len(roots) != 0 {
		t.Errorf("roots after closing = %v, want none", roots)
	}
	if _, err := a.ReadFile(file); err == nil {
		t.Error("ReadFile reached the closed workspace")
	}
	if got, err := a.ListDir(dir); err != nil || len(got) != 0 {
		t.Errorf("ListDir of the closed workspace = %v, %v: with no workspace there is nothing to list (an empty list, no error, no entry)", got, err)
	}
	if err := a.WriteFile(file, "x"); err == nil {
		t.Error("WriteFile reached the closed workspace")
	}
	if b, _ := os.ReadFile(file); string(b) == "x" {
		t.Error("the closed workspace was written")
	}
	// the empty path is not "the current folder": nothing relative resolves either
	for _, p := range []string{"", ".", "a.probe", "tests"} {
		if _, err := a.ReadFile(p); err == nil {
			t.Errorf("ReadFile(%q) worked with no workspace", p)
		}
		if got, _ := a.ListDir(p); len(got) != 0 {
			t.Errorf("ListDir(%q) listed %v with no workspace", p, got)
		}
	}
}
