package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RunFile runs only .probe files inside the workspace a person opened or an automation folder. The check runs before
// the connection check, so a refused path fails the same way whether or not a device is connected.
func TestRunFileRefusesPathsOutsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "smoke.probe")
	notProbe := filepath.Join(workspace, "notes.txt")
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	for _, p := range []string{inside, notProbe, outside} {
		if err := os.WriteFile(p, []byte("test \"x\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{}
	a.SetWorkspace(workspace)

	if _, err := a.RunFile(outside); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a .probe file outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
	if _, err := a.RunFile(notProbe); err == nil || !strings.Contains(err.Error(), "only .probe files") {
		t.Errorf("a non-.probe file in the workspace: err %v; want the suffix refusal", err)
	}
	// A file inside the workspace passes the check; with no device connected it stops at the connection check.
	if _, err := a.RunFile(inside); err != errNotConnected {
		t.Errorf("a .probe file inside the workspace: err %v; want errNotConnected (the path check passed)", err)
	}
}

// RunFileAsync refuses the path at once, so the caller hears about it instead of a background run failing silently.
func TestRunFileAsyncRefusesPathsOutsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	if err := os.WriteFile(outside, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{conn: &connection{}}
	a.SetWorkspace(workspace)

	if err := a.RunFileAsync(outside); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("RunFileAsync on a path outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
}
