package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ReadFile reads .probe files inside the workspace a person opened or an automation folder: the check RunFile makes.
func TestReadFileReadsAProbeFileInsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "smoke.probe")
	if err := os.WriteFile(path, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if got, err := a.ReadFile(path); err != nil || got != "test \"x\"\n" {
		t.Fatalf("ReadFile inside the workspace: got %q, err %v", got, err)
	}
}

func TestReadFileRefusesPathsOutsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	if err := os.WriteFile(outside, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ReadFile(outside); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a .probe file outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
}

func TestReadFileRefusesANonProbeFileInsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	notes := filepath.Join(workspace, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ReadFile(notes); err == nil || !strings.Contains(err.Error(), "only .probe files") {
		t.Errorf("a non-.probe file inside the workspace: err %v; want the suffix refusal", err)
	}
}

// A ".." segment is refused outright, even one that would stay inside the workspace, so no path relies on climbing.
func TestReadFileRefusesDotDotSegments(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	if err := os.WriteFile(outside, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "smoke.probe"), []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Built by hand: filepath.Join would clean the ".." away before ReadFile sees it.
	sep := string(filepath.Separator)
	climbOut := workspace + sep + ".." + sep + filepath.Base(filepath.Dir(outside)) + sep + filepath.Base(outside)
	stayInside := workspace + sep + "sub" + sep + ".." + sep + "smoke.probe"
	relativeClimb := ".." + sep + "smoke.probe"
	a := &App{}
	a.SetWorkspace(workspace)
	for _, p := range []string{climbOut, stayInside, relativeClimb} {
		if _, err := a.ReadFile(p); err == nil || !strings.Contains(err.Error(), "path traversal rejected") {
			t.Errorf("ReadFile(%q): err %v; want the path traversal refusal", p, err)
		}
	}
}

// Only absolute paths are read, so a bare name cannot resolve against whatever directory the process is in.
func TestReadFileNeedsAnAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "smoke.probe"), []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ReadFile("smoke.probe"); err == nil || !strings.Contains(err.Error(), "use an absolute path") {
		t.Errorf("a relative path: err %v; want the absolute-path refusal", err)
	}
}

// A symlink inside the workspace that points outside is refused, because the check resolves links before comparing.
func TestReadFileRefusesASymlinkOutOfTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	if err := os.WriteFile(outside, []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "link.probe")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ReadFile(link); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a symlink out of the workspace: err %v; want the outside-the-folders refusal", err)
	}
}
