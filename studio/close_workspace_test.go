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

// Closing is refused while a run or a recording is going, by Go and not only by the page: a script with the bridge token cannot pull the workspace from
// under either. Refused, nothing changes (the workspace stays open and its folder stays reachable).
func TestCloseWorkspaceIsRefusedWhileARunOrARecordingIsGoing(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	open := func() *App {
		a := NewApp()
		a.SetWorkspace(dir)
		return a
	}
	// a run holds the slot
	a := open()
	_, cancel, err := a.claimRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CloseWorkspace(); err != errCloseDuringRun {
		t.Errorf("during a run: %v, want %v", err, errCloseDuringRun)
	}
	if len(a.automationRoots()) != 1 {
		t.Error("a refused close changed the workspace")
	}
	a.releaseRun(cancel)
	if err := a.CloseWorkspace(); err != nil || len(a.automationRoots()) != 0 {
		t.Errorf("after the run: %v, roots %v", err, a.automationRoots())
	}
	// a recording is on
	b := open()
	b.mu.Lock()
	b.recActive = true
	b.mu.Unlock()
	if err := b.CloseWorkspace(); err != errCloseDuringRecording || len(b.automationRoots()) != 1 {
		t.Errorf("during a recording: %v, roots %v", err, b.automationRoots())
	}
}

// The refusal and the change are one step under the run slot's lock and a.mu: a run that claims the slot while closes are going on either gets the slot
// before a close (and the close is refused) or after it (and the run belongs to the closed, empty workspace); no run is left with a workspace that
// was closed under it. Run with -race.
func TestCloseWorkspaceRacingARunNeverClosesUnderIt(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	for i := 0; i < 200; i++ {
		a := NewApp()
		a.SetWorkspace(dir)
		done := make(chan error, 1)
		go func() { done <- a.CloseWorkspace() }()
		_, cancel, claimErr := a.claimRun()
		closeErr := <-done
		if claimErr == nil {
			// the run holds the slot: if the close came after the claim it was refused and the workspace is open; if it came before, it was closed first
			if closeErr == nil && a.runWorkspace != "" && len(a.automationRoots()) != 0 {
				t.Fatalf("round %d: closed, yet the run was given a workspace and the roots are open", i)
			}
			if closeErr == errCloseDuringRun && len(a.automationRoots()) != 1 {
				t.Fatalf("round %d: a refused close changed the workspace", i)
			}
			a.releaseRun(cancel)
		}
	}
}
