package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Save never writes into a hidden folder of the workspace (.git, .github, a tool's own folder) or to a file whose name starts with a dot, and a saved
// file has a size cap. The refusal comes before any folder is created.

func hiddenFixture(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(dir)
	return a, dir
}

func TestWriteFileRefusesHiddenFoldersAndNames(t *testing.T) {
	a, dir := hiddenFixture(t)
	for _, rel := range []string{
		".git/x.probe",
		".git/hooks/pre-commit.probe",
		".github/workflows/ci.probe",
		".hg/x.probe",
		"sub/.hidden/x.probe",
		"a/b/.c/d.probe",
		".GIT/x.probe",
		".config/probe/x.probe",
		".x.probe",
		"sub/.x.probe",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		err := a.WriteFile(p, "test \"x\"\n")
		if err == nil || !strings.Contains(err.Error(), "starts with a dot") {
			t.Errorf("WriteFile(%s): err = %v, want a refusal for the hidden part", rel, err)
		}
		if _, statErr := os.Lstat(p); statErr == nil {
			t.Errorf("WriteFile(%s) wrote the file", rel)
		}
		if err := a.CreateFile(p, "x"); err == nil {
			t.Errorf("CreateFile(%s) was allowed", rel)
		}
	}
	// the refusal comes before any folder is made for the file
	es, _ := os.ReadDir(dir)
	if len(es) != 0 {
		var names []string
		for _, e := range es {
			names = append(names, e.Name())
		}
		t.Errorf("a refused write left %v behind", names)
	}
}

func TestWriteFileStillWritesWhereADotIsNotAComponentStart(t *testing.T) {
	a, dir := hiddenFixture(t)
	for _, rel := range []string{"x.probe", "sub.dir/x.probe", "a/b.c/x.y.probe", "flows/login/smoke.probe"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := a.WriteFile(p, "test \"x\"\n"); err != nil {
			t.Errorf("WriteFile(%s): %v", rel, err)
		}
	}
}

// Only the part below the folder the person opened is judged: a workspace that sits under a dotted folder is a workspace.
func TestAWorkspaceUnderADottedFolderCanStillBeSavedTo(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	base, _ := filepath.EvalSymlinks(t.TempDir())
	ws := filepath.Join(base, ".config", "my.project")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.SetWorkspace(ws)
	if err := a.WriteFile(filepath.Join(ws, "ok.probe"), "x"); err != nil {
		t.Errorf("a workspace under a dotted folder was refused: %v", err)
	}
	if err := a.WriteFile(filepath.Join(ws, ".git", "no.probe"), "x"); err == nil {
		t.Error("a hidden folder inside such a workspace was allowed")
	}
}

// A visible link that leads into a hidden folder is judged by where it leads.
func TestALinkIntoAHiddenFolderIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	a, dir := hiddenFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, ".git"), filepath.Join(dir, "visible")); err != nil {
		t.Fatal(err)
	}
	if err := a.WriteFile(filepath.Join(dir, "visible", "x.probe"), "x"); err == nil || !strings.Contains(err.Error(), "starts with a dot") {
		t.Errorf("err = %v, want a refusal", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git", "x.probe")); err == nil {
		t.Error("the file was written into .git through the link")
	}
}

func TestWriteFileHasTheSameSizeCapAsCreateFile(t *testing.T) {
	a, dir := hiddenFixture(t)
	p := filepath.Join(dir, "big.probe")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := a.WriteFile(p, strings.Repeat("x", maxSaveBytes+1))
	if err == nil || !strings.Contains(err.Error(), "at most 1048576") {
		t.Fatalf("err = %v, want the cap named in the refusal", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Errorf("a refused save changed the file: %d bytes", len(b))
	}
	if err := a.WriteFile(p, strings.Repeat("x", maxSaveBytes)); err != nil {
		t.Errorf("a file of exactly the cap was refused: %v", err)
	}
	// the refusal comes before a folder is made
	err = a.WriteFile(filepath.Join(dir, "newfolder", "big.probe"), strings.Repeat("x", maxSaveBytes+1))
	if err == nil {
		t.Fatal("an over-cap file in a new folder was written")
	}
	if _, e := os.Stat(filepath.Join(dir, "newfolder")); e == nil {
		t.Error("a refused save created its folder")
	}
}
