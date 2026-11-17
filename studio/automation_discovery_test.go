package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The discovery file holds the automation token, so it is written through the one writer of secret files and judged the way the MCP token file is:
// a folder or file that is a link, that others can reach or that is not a plain private file is refused, never repaired and never written through.

func discoveryDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not describe who can read a file here")
	}
	d, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(d, 0o700)
	return d
}

func startRefused(t *testing.T, file string, want ...string) {
	t.Helper()
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	s, err := startAutomation(&fakeBackend{})
	if err == nil {
		s.Close()
		t.Fatalf("the server started with the discovery file %s", file)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("the refusal %q lacks %q", err, w)
		}
	}
}

func TestADiscoveryFileThatIsALinkOrOpenToOthersIsRefused(t *testing.T) {
	dir := discoveryDir(t)
	file := filepath.Join(dir, "automation.json")

	target := filepath.Join(discoveryDir(t), "elsewhere")
	_ = os.WriteFile(target, []byte("keep\n"), 0o600)
	if err := os.Symlink(target, file); err != nil {
		t.Fatal(err)
	}
	startRefused(t, file, "studio automation", "is a symlink or not a regular file")
	if b, _ := os.ReadFile(target); string(b) != "keep\n" {
		t.Error("the token was written through the link")
	}
	_ = os.Remove(file)

	for _, mode := range []os.FileMode{0o644, 0o640, 0o604} {
		_ = os.WriteFile(file, []byte("old\n"), 0o600)
		_ = os.Chmod(file, mode)
		startRefused(t, file, "is open to other users")
		if b, _ := os.ReadFile(file); string(b) != "old\n" {
			t.Errorf("mode %04o: the refused file was written", mode)
		}
		_ = os.Remove(file)
	}
}

func TestADiscoveryFolderThatIsALinkOrWritableByOthersIsRefused(t *testing.T) {
	real := discoveryDir(t)
	link := filepath.Join(discoveryDir(t), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	startRefused(t, filepath.Join(link, "automation.json"), "token directory", "is a symlink or not a directory")
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Error("the token was written through the folder link")
	}
	for _, mode := range []os.FileMode{0o775, 0o757, 0o777} {
		d := discoveryDir(t)
		_ = os.Chmod(d, mode)
		startRefused(t, filepath.Join(d, "automation.json"), "can be written by other users")
		if entries, _ := os.ReadDir(d); len(entries) != 0 {
			t.Errorf("mode %04o: written into the refused folder", mode)
		}
	}
}

// A folder of the person's own that others may read (a home folder) is fine for a file they named; the file itself is 0600.
func TestANamedDiscoveryFileInAReadableFolderIsWrittenPrivate(t *testing.T) {
	d := discoveryDir(t)
	_ = os.Chmod(d, 0o755)
	file := filepath.Join(d, "automation.json")
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	s, err := startAutomation(&fakeBackend{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file: %v %v", fi, err)
	}
}

// With no user config folder and no file named, there is no private place to keep the file: the server does not start, and nothing is written to
// the system's shared temporary folder.
func TestNoConfigFolderMeansNoDiscoveryFileInTheTempFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the config folder comes from %AppData% here")
	}
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	s, err := startAutomation(&fakeBackend{})
	if err == nil {
		s.Close()
		t.Fatal("the server started with nowhere private to keep its file")
	}
	if !strings.Contains(err.Error(), "PROBE_STUDIO_AUTOMATION_FILE") {
		t.Errorf("the refusal does not say what to do: %v", err)
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("something was written to the temp folder: %v", entries)
	}
}

// A file left by a server that was killed is replaced (it is the person's own plain private file), and a server removes only its own document.
func TestAStaleDiscoveryFileIsReplacedAndOnlyOwnIsRemoved(t *testing.T) {
	d := discoveryDir(t)
	file := filepath.Join(d, "automation.json")
	_ = os.WriteFile(file, []byte(`{"pid":1}`), 0o600)
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	first, err := startAutomation(&fakeBackend{})
	if err != nil {
		t.Fatalf("a stale private file was refused: %v", err)
	}
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), first.token) {
		t.Fatal("the stale file was not replaced")
	}
	second, err := startAutomation(&fakeBackend{}) // a later server puts its own document there
	if err != nil {
		t.Fatal(err)
	}
	first.Close() // the first one must not remove the second's file
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), second.token) {
		t.Error("the first server removed the second server's discovery file")
	}
	second.Close()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the last server did not remove its own file")
	}
}
