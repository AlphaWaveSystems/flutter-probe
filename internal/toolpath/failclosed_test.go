package toolpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// Only the protected folder itself has no file identity: Within still fails closed, and Check says it cannot verify the location
// instead of claiming the tool is inside the repository.
func TestWithinFailsClosedWhenOnlyTheProtectedFolderHasNoIdentity(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "adb")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	old := hasIdentity
	t.Cleanup(func() { hasIdentity = old })
	hasIdentity = func(p string) bool { return p != root && p != realRoot }
	if !Within(root, outside) {
		t.Error("the protected folder without an identity must fail closed")
	}
	if _, err := within(root, outside); err == nil || !strings.Contains(err.Error(), "has no file identity to compare") {
		t.Errorf("within: err = %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	_, err = Check(outside, []string{"adb"}, root, runtime.GOOS)
	if err == nil || !strings.Contains(err.Error(), "where it is cannot be verified, so it is refused: ") ||
		!strings.Contains(err.Error(), "has no file identity to compare") || strings.Contains(err.Error(), "inside the") {
		t.Errorf("Check: err = %v", err)
	}
}

// A folder on the way to the tool that cannot be read: the tool may exist, so the message says its location cannot be verified, not
// that it cannot be found; and within names the folder and the cause.
func TestAnUnreadableFolderIsReportedAsUnverifiable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("needs POSIX permissions and a user that they apply to")
	}
	base := t.TempDir()
	locked := filepath.Join(base, "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(locked, "adb")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	_, err := Check(tool, []string{"adb"}, t.TempDir(), runtime.GOOS)
	if err == nil || err.Error() != "where it is cannot be verified: permission denied" {
		t.Errorf("Check: err = %v", err)
	}
	if _, err := Check(filepath.Join(base, "missing", "adb"), []string{"adb"}, t.TempDir(), runtime.GOOS); err == nil || err.Error() != "the file cannot be found" {
		t.Errorf("a missing file: err = %v", err)
	}
	in, err := within(t.TempDir(), tool)
	if in || err == nil || !strings.Contains(err.Error(), "cannot be read (permission denied)") || !strings.Contains(err.Error(), "adb") {
		t.Errorf("within: %v, %v", in, err)
	}
	if !Within(t.TempDir(), tool) {
		t.Error("Within must fail closed for an unreadable folder")
	}
}
