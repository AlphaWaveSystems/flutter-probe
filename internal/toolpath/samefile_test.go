package toolpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// execTool writes an executable called name at p.
func execTool(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// On a case-insensitive file system (macOS, Windows), /x/evil/tools/adb opens /x/Evil/Tools/adb: it is inside the repository, whatever its
// spelling. Skipped where the file system is case-sensitive (the other spelling is then another folder).
func TestAnotherSpellingOfTheRepositoryIsStillTheRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tool is a shell script")
	}
	base := t.TempDir()
	root := filepath.Join(base, "Evil")
	repo(t, root)
	execTool(t, filepath.Join(root, "Tools", "adb"))
	lower := filepath.Join(base, "evil", "tools", "adb")
	a, errA := os.Stat(lower)
	b, errB := os.Stat(filepath.Join(root, "Tools", "adb"))
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Skip("case-sensitive file system: the other spelling is another folder")
	}
	if _, err := Check(lower, []string{"adb"}, root, runtime.GOOS); err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Errorf("the repository's tool under another spelling must be refused, got %v", err)
	}
	if !Within(root, lower) {
		t.Error("Within must compare the folders, not their spelling")
	}
}

// A probe.yaml in a nested repository (a submodule, a checkout inside another) protects the outer repository too.
func TestTheOuterRepositoryIsProtectedToo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tool is a shell script")
	}
	outer := t.TempDir()
	repo(t, outer)
	inner := filepath.Join(outer, "sub", "inner")
	repo(t, inner)
	adb := filepath.Join(outer, "tools", "adb")
	execTool(t, adb)
	if _, err := Check(adb, []string{"adb"}, inner, runtime.GOOS); err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Errorf("a tool in the outer repository must be refused for a probe.yaml in the inner one, got %v", err)
	}
	if got := RepoRoots(inner); len(got) < 2 {
		t.Errorf("RepoRoots(%q) = %q, want the inner and the outer repository", inner, got)
	}
}

// A probe.yaml reached through a symlink is protected where it really is: CheckIn checks every folder it is given.
func TestCheckInProtectsEveryFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake tool is a shell script")
	}
	real := t.TempDir()
	repo(t, real)
	adb := filepath.Join(real, "tools", "adb")
	execTool(t, adb)
	linkDir := t.TempDir() // no repository: on its own it would protect only itself
	if _, err := Check(adb, []string{"adb"}, linkDir, runtime.GOOS); err != nil {
		t.Fatalf("from the link's folder alone the tool passes (that is the gap): %v", err)
	}
	if _, err := CheckIn(adb, []string{"adb"}, []string{linkDir, real}, runtime.GOOS); err == nil {
		t.Error("CheckIn must refuse a tool inside the folder the probe.yaml really is in")
	}
	outside := filepath.Join(t.TempDir(), "adb")
	execTool(t, outside)
	if got, err := CheckIn(outside, []string{"adb"}, []string{linkDir, real}, runtime.GOOS); err != nil || got == "" {
		t.Errorf("a tool outside both passes: %q, %v", got, err)
	}
}
