package toolpath

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Without a file identity to compare (some Windows network shares and file systems report none), Within fails closed: the path is
// treated as inside, so a probe.yaml tools path there is refused rather than run.
func TestWithinFailsClosedWithoutAFileIdentity(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Within(root, outside) {
		t.Fatal("with identities, a file in another folder is not within")
	}
	old := hasIdentity
	t.Cleanup(func() { hasIdentity = old })
	hasIdentity = func(string) bool { return false }
	if !Within(root, outside) {
		t.Error("without a file identity, Within must answer within (fail closed)")
	}
	hasIdentity = func(p string) bool { return p != filepath.Dir(outside) } // one folder on the way has none
	if !Within(root, outside) {
		t.Error("a folder on the way without an identity must fail closed")
	}
	if runtime.GOOS != "windows" {
		hasIdentity = old
		if _, err := Check(outside, []string{"adb"}, root, runtime.GOOS); err != nil {
			t.Errorf("with identities the outside tool passes: %v", err)
		}
		hasIdentity = func(string) bool { return false }
		if _, err := Check(outside, []string{"adb"}, root, runtime.GOOS); err == nil {
			t.Error("without identities Check must refuse")
		}
	}
}

// A folder that cannot be read fails closed too.
func TestWithinFailsClosedForAFolderThatCannotBeRead(t *testing.T) {
	if !Within(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "adb")) {
		t.Error("a protected folder that cannot be read must count as within")
	}
}

// File names are case-insensitive on Windows: ADB.EXE is adb.exe there, and only there.
func TestToolNameIsCaseInsensitiveOnlyOnWindows(t *testing.T) {
	if !nameIn("ADB.EXE", Names("adb", "windows"), "windows") || !nameIn("Flutter.BAT", Names("flutter", "windows"), "windows") {
		t.Error("on Windows the name must match in any case")
	}
	if nameIn("ADB", Names("adb", "linux"), "linux") || nameIn("Adb", Names("adb", "darwin"), "darwin") {
		t.Error("on macOS and Linux the name must match exactly")
	}
}
