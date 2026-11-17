package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func createFixture(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	a := NewApp()
	a.SetWorkspace(dir)
	return a, dir
}

func TestCreateFileWritesANewFile(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "new.probe")
	if err := a.CreateFile(p, "test \"x\"\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "test \"x\"\n" {
		t.Errorf("content = %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

// The file appears between the check (the listing the window made) and the write: it is not replaced, and the answer says so.
func TestCreateFileNeverReplacesAFileThatAppearedAfterTheCheck(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "taken.probe")
	names := func() []string {
		es, _ := os.ReadDir(dir)
		var out []string
		for _, e := range es {
			out = append(out, e.Name())
		}
		return out
	}
	if len(names()) != 0 { // the listing the window checked the name against: empty
		t.Fatal("not empty")
	}
	if err := os.WriteFile(p, []byte("someone else's file"), 0o600); err != nil { // created after that listing
		t.Fatal(err)
	}
	err := a.CreateFile(p, "mine")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want a refusal that says the file already exists", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "someone else's file" {
		t.Errorf("the file was replaced: %q", b)
	}
}

func TestCreateFileRefusesALinkAFolderAndWhatIsNotAProbeFileOrOutsideTheWorkspace(t *testing.T) {
	a, dir := createFixture(t)
	target := filepath.Join(dir, "target.probe")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.probe")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := a.CreateFile(link, "x"); err == nil {
			t.Error("a link was written through")
		}
		dangling := filepath.Join(dir, "dangling.probe")
		if err := os.Symlink(filepath.Join(dir, "nowhere.probe"), dangling); err != nil {
			t.Fatal(err)
		}
		if err := a.CreateFile(dangling, "x"); err == nil {
			t.Error("a dangling link was written through")
		}
		if _, err := os.Stat(filepath.Join(dir, "nowhere.probe")); err == nil {
			t.Error("a file was created at the end of a dangling link")
		}
		if b, _ := os.ReadFile(target); string(b) != "keep" {
			t.Errorf("the target of a link was changed: %q", b)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d.probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{
		"a folder":         filepath.Join(dir, "d.probe"),
		"not a probe file": filepath.Join(dir, "x.txt"),
		"outside":          filepath.Join(filepath.Dir(dir), "outside.probe"),
		"a missing folder": filepath.Join(dir, "no", "such", "x.probe"),
	} {
		if err := a.CreateFile(p, "x"); err == nil {
			t.Errorf("%s: written", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "no")); err == nil {
		t.Error("a folder was created for the file")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside.probe")); err == nil {
		t.Error("a file was written outside the workspace")
	}
}

// Case-insensitive file systems: a name that differs by case from an existing file is the same file there, and the exclusive create fails.
func TestCreateFileRefusesTheSameNameInAnotherCaseWhereTheFileSystemIgnoresCase(t *testing.T) {
	a, dir := createFixture(t)
	upper := filepath.Join(dir, "LOGIN.probe")
	if err := os.WriteFile(upper, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	lower := filepath.Join(dir, "login.probe")
	if _, err := os.Stat(lower); err != nil {
		t.Skip("this file system distinguishes case: the two names are two files")
	}
	if err := a.CreateFile(lower, "mine"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v", err)
	}
	if b, _ := os.ReadFile(upper); string(b) != "keep" {
		t.Errorf("replaced: %q", b)
	}
}
