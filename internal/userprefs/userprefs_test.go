package userprefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cfg")
	t.Setenv("PROBE_USER_CONFIG_DIR", dir)
	return dir
}

// Everything is off until the person turns it on, and a file that cannot be read as a whole is off too.
func TestDefaultsAreOffAndABadFileIsOff(t *testing.T) {
	dir := isolate(t)
	if Load().ClearDataWithoutAsking {
		t.Fatal("on with no file")
	}
	_ = os.MkdirAll(dir, 0o700)
	for name, content := range map[string]string{
		"not json": "clearDataWithoutAsking: true", "an unknown field": `{"clearDataWithoutAsking": true, "other": 1}`,
		"the wrong type": `{"clearDataWithoutAsking": "yes"}`, "empty": "", "an array": "[true]",
	} {
		if err := os.WriteFile(filepath.Join(dir, "prefs.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if Load().ClearDataWithoutAsking {
			t.Errorf("%s: a file that is not exactly the preferences turned the setting on", name)
		}
	}
}

func TestSetAndLoadRoundTripAtomicallyAndPrivately(t *testing.T) {
	dir := isolate(t)
	if err := SetClearDataWithoutAsking(true); err != nil {
		t.Fatal(err)
	}
	if !Load().ClearDataWithoutAsking {
		t.Error("not on after Set(true)")
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filepath.Join(dir, "prefs.json")); info.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", info.Mode().Perm())
		}
		if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
			t.Errorf("folder mode = %v", info.Mode().Perm())
		}
	}
	if err := SetClearDataWithoutAsking(false); err != nil {
		t.Fatal(err)
	}
	if Load().ClearDataWithoutAsking {
		t.Error("still on after Set(false)")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder holds %v (a temp file was left)", entries)
	}
}

func TestAPreferencesFileThatIsALinkIsNotWrittenThrough(t *testing.T) {
	dir := isolate(t)
	_ = os.MkdirAll(dir, 0o700)
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	_ = os.WriteFile(target, []byte("keep"), 0o600)
	if err := os.Symlink(target, filepath.Join(dir, "prefs.json")); err != nil {
		t.Skip("no symlinks")
	}
	if err := SetClearDataWithoutAsking(true); err == nil {
		t.Error("a link was written through")
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Error("the linked file changed")
	}
}

// A project cannot reach it: the path never depends on the working directory, so a prefs.json in a cloned repository is never read.
func TestAFileInTheWorkingDirectoryIsNeverRead(t *testing.T) {
	isolate(t)
	work := t.TempDir()
	_ = os.WriteFile(filepath.Join(work, "prefs.json"), []byte(`{"clearDataWithoutAsking": true}`), 0o600)
	here, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(here) })
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	if Load().ClearDataWithoutAsking {
		t.Error("a prefs.json in the working directory turned the setting on")
	}
}
