package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// WriteFile and ListDir follow the rule ReadFile and RunFile follow: an absolute path, no ".." segment on either
// separator, every symlink resolved, and a place inside the workspace a person opened or an automation folder.

func TestWriteFileWritesANewProbeFileInsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	path := filepath.Join(workspace, "flows", "login", "smoke.probe") // the folders do not exist yet
	if err := a.WriteFile(path, "test \"x\"\n"); err != nil {
		t.Fatalf("WriteFile inside the workspace: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "test \"x\"\n" {
		t.Fatalf("the file is not where WriteFile put it: %q, %v", got, err)
	}
}

func TestWriteFileRefusesPathsOutsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	a := &App{}
	a.SetWorkspace(workspace)
	if err := a.WriteFile(outside, "x"); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a new .probe file outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Errorf("a refused write still created the file: %v", err)
	}
}

func TestWriteFileRefusesANonProbeName(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	if err := a.WriteFile(filepath.Join(workspace, "notes.txt"), "x"); err == nil || !strings.Contains(err.Error(), "only .probe files can be written") {
		t.Errorf("a non-.probe name: err %v; want the suffix refusal", err)
	}
}

// Both separators count, and a ".." that would stay inside the workspace is refused too.
func TestWriteFileRefusesDotDotSegments(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	sep := string(filepath.Separator)
	for _, p := range []string{
		workspace + sep + ".." + sep + "evil.probe",
		workspace + sep + "sub" + sep + ".." + sep + "smoke.probe",
		workspace + "/..\\evil.probe",
	} {
		if err := a.WriteFile(p, "x"); err == nil || !strings.Contains(err.Error(), "path traversal rejected") {
			t.Errorf("WriteFile(%q): err %v; want the path traversal refusal", p, err)
		}
	}
}

func TestWriteFileRefusesAFileThatLinksOut(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere.probe")
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "link.probe")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if err := a.WriteFile(link, "overwritten"); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a link out of the workspace: err %v; want the outside-the-folders refusal", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "original" {
		t.Errorf("the file outside the workspace was changed through the link: %q", got)
	}
}

// The folders may not exist yet, so a new file under a folder that is a link out of the workspace must be refused.
func TestWriteFileRefusesANewFileUnderAFolderThatLinksOut(t *testing.T) {
	workspace := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(workspace, "linked")); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	path := filepath.Join(workspace, "linked", "new.probe")
	if err := a.WriteFile(path, "x"); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a new file under a linked folder: err %v; want the outside-the-folders refusal", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "new.probe")); !os.IsNotExist(err) {
		t.Errorf("a file was created outside the workspace through the link: %v", err)
	}
}

func TestWriteFileNeedsAnAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	if err := a.WriteFile("smoke.probe", "x"); err == nil || !strings.Contains(err.Error(), "use an absolute path") {
		t.Errorf("a relative path: err %v; want the absolute-path refusal", err)
	}
}

func TestWriteFileRefusesAFolderNamedLikeAProbeFile(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "folder.probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	if err := a.WriteFile(filepath.Join(workspace, "folder.probe"), "x"); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Errorf("a folder with a .probe name: err %v; want the is-a-folder refusal", err)
	}
}

func TestListDirListsTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "smoke.probe"), []byte("test \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "flows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hidden"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	a.SetWorkspace(workspace)
	entries, err := a.ListDir(workspace)
	if err != nil {
		t.Fatalf("ListDir of the workspace: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name] = true
		if e.Path != filepath.Join(workspace, e.Name) {
			t.Errorf("entry %q has path %q, want the path under the folder it was listed from", e.Name, e.Path)
		}
	}
	if len(entries) != 2 || !seen["smoke.probe"] || !seen["flows"] {
		t.Errorf("ListDir of the workspace: got %v; want smoke.probe and flows, hidden entries left out", entries)
	}
}

func TestListDirRefusesFoldersOutsideTheWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ListDir(outside); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a folder outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
	// A missing folder outside is refused the same way, so the answer does not reveal what exists there.
	if _, err := a.ListDir(filepath.Join(outside, "missing")); err == nil || !strings.Contains(err.Error(), "outside the folders automation may use") {
		t.Errorf("a missing folder outside the workspace: err %v; want the outside-the-folders refusal", err)
	}
}

func TestListDirRefusesDotDotSegments(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	if _, err := a.ListDir(workspace + string(filepath.Separator) + ".."); err == nil || !strings.Contains(err.Error(), "path traversal rejected") {
		t.Errorf("a .. folder: err %v; want the path traversal refusal", err)
	}
}

func TestListDirShowsAMissingFolderInsideTheWorkspaceAsEmpty(t *testing.T) {
	workspace := t.TempDir()
	a := &App{}
	a.SetWorkspace(workspace)
	if entries, err := a.ListDir(filepath.Join(workspace, "not-yet")); err != nil || len(entries) != 0 {
		t.Errorf("a missing folder inside the workspace: entries %v, err %v; want an empty list and no error", entries, err)
	}
}

// Before a workspace is open the file tree starts on a relative folder; there is nothing to list yet.
func TestListDirWithNoWorkspaceListsNothing(t *testing.T) {
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	a := &App{}
	if entries, err := a.ListDir("tests"); err != nil || len(entries) != 0 {
		t.Errorf("no workspace open: entries %v, err %v; want an empty list and no error", entries, err)
	}
}
