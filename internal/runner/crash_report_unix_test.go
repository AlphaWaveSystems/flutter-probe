//go:build !windows

package runner

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The folders and files take the modes the other report writers use (0755 and 0644 under the usual umask), not something looser or stricter.
func TestCrashFilesHaveTheModesOfTheOtherReports(t *testing.T) {
	// with no umask at all the modes asked for are what the disk gets: 0755 and 0644, not 0777 or 0666
	old := syscall.Umask(0)
	dir0 := t.TempDir()
	files0, err0 := writeCrashFiles(filepath.Join(dir0, "crash"), "a.probe", "t", 1, hostileCrash())
	syscall.Umask(old)
	if err0 != nil || len(files0) != 2 {
		t.Fatalf("%v %v", files0, err0)
	}
	for _, f := range files0 {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("umask 0: %s mode %v, want 0644 (%v)", f, fi.Mode().Perm(), err)
		}
	}
	if fi, err := os.Stat(filepath.Dir(files0[0])); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("umask 0: folder mode %v, want 0755 (%v)", fi.Mode().Perm(), err)
	}
	old = syscall.Umask(0o022)
	defer syscall.Umask(old)
	dir := t.TempDir()
	files, err := writeCrashFiles(filepath.Join(dir, "crash"), "a.probe", "t", 1, hostileCrash())
	if err != nil || len(files) != 2 {
		t.Fatalf("%v %v", files, err)
	}
	for _, f := range files {
		if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o644 {
			t.Errorf("%s: mode %v, want 0644 (%v)", f, fi.Mode().Perm(), err)
		}
	}
	for _, d := range []string{filepath.Join(dir, "crash"), filepath.Dir(files[0])} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("%s: mode %v, want 0755 (%v)", d, fi.Mode().Perm(), err)
		}
	}
	// the report file itself is made the same way (os.Create under the same umask): 0644
	rf, err := NewFileReporter(FormatJSON, filepath.Join(dir, "r", "report.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "r", "report.json")); err != nil || fi.Mode().Perm() != 0o644 || rf == nil {
		t.Errorf("the report file's mode is %v: the crash files should match it", fi.Mode().Perm())
	}
}
