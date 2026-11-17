//go:build !windows

package secretfile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func within(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %v", what, d)
	}
}

// A pipe planted at the file's name must not make Remove wait for a writer (an open of a pipe blocks): availability of the program that calls it.
func TestRemoveDoesNotBlockOnAPipeAtTheName(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	path := filepath.Join(dir, "secret")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("no pipes here: %v", err)
	}
	within(t, 3*time.Second, "Remove on a pipe", func() { Remove(path, "token-1\n") })
	if fi, err := os.Lstat(path); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		t.Error("Remove touched the pipe")
	}
	// and Write refuses it, without waiting either
	within(t, 3*time.Second, "Write over a pipe", func() {
		_, _, err := Write(opts(dir))
		wantRefusal(t, err, "is a symlink or not a regular file")
	})
}

// Remove follows no link, opens nothing that is not a regular file of the right size, and never removes a file with other content.
func TestRemoveOnlyOpensItsOwnPlainFile(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	target := filepath.Join(priv(t), "elsewhere")
	_ = os.WriteFile(target, []byte("token-1\n"), 0o600)
	link := filepath.Join(dir, "secret")
	_ = os.Symlink(target, link)
	Remove(link, "token-1\n")
	if _, err := os.Stat(target); err != nil {
		t.Error("Remove followed a link and removed the target")
	}
	if _, err := os.Lstat(link); err != nil {
		t.Error("Remove removed a link it did not make")
	}
	_ = os.Remove(link)
	big := filepath.Join(dir, "big")
	_ = os.WriteFile(big, []byte(strings.Repeat("x", 1<<20)), 0o600)
	Remove(big, "token-1\n") // a size that is not the content's is not even opened
	if _, err := os.Stat(big); err != nil {
		t.Error("a file of another size was removed")
	}
	same := filepath.Join(dir, "same")
	_ = os.WriteFile(same, []byte("token-2\n"), 0o600) // same size, other content
	Remove(same, "token-1\n")
	if _, err := os.Stat(same); err != nil {
		t.Error("a file with other content was removed")
	}
	Remove(filepath.Join(dir, "none"), "token-1\n") // not there: nothing happens
}

// A write that failed after its temporary file was made removes that file: the secret does not stay behind in a name nobody knows.
func TestAFailedWriteLeavesNoTemporaryFile(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	saved := renameFile
	defer func() { renameFile = saved }()
	renameFile = func(string, string) error { return errors.New("disk said no") }
	_, _, err := Write(opts(dir))
	if err == nil || !strings.Contains(err.Error(), "disk said no") {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("a temporary file with the secret was left: %v", entries)
	}
}

// The temporary files an earlier write left (the process died between making one and renaming it) are removed by the next write, but only ours, old
// ones, plain files named like them.
func TestStaleTemporaryFilesAreCleanedAtTheNextWrite(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	old := time.Now().Add(-time.Hour)
	mk := func(name string, age time.Time) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("leaked-token\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, age, age)
		return p
	}
	stale := mk("secret.1234567", old)
	fresh := mk("secret.7654321", time.Now()) // a write that is under way
	other := mk("other.1234567", old)         // another name's
	unrelated := mk("secret-note", old)       // not a temporary file of ours
	linkTarget := mk("target", old)
	link := filepath.Join(dir, "secret.9999999")
	_ = os.Symlink(linkTarget, link)
	_ = os.Chtimes(linkTarget, old, old)
	if err := exec.Command("touch", "-h", "-t", "200001010000", link).Run(); err != nil { // the link itself is old too
		t.Skipf("cannot age a link here: %v", err)
	}
	if _, _, err := Write(opts(dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Error("a stale temporary file with the secret was left")
	}
	for name, p := range map[string]string{"a fresh temporary file": fresh, "another name's file": other, "an unrelated file": unrelated, "a link": link, "a link's target": linkTarget} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s was removed", name)
		}
	}
	// files of another user are not ours to remove
	foreign := mk("secret.5555555", old)
	savedOwner := ownedByUs
	defer func() { ownedByUs = savedOwner }()
	ownedByUs = func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "5555555") }
	if _, _, err := Write(opts(dir)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(foreign); err != nil {
		t.Error("a temporary file of another user was removed")
	}
}

// Only the temporary files this package makes are removed: a name that merely starts with the file's name is the person's own file.
func TestOnlyExactTemporaryNamesAreRemoved(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	old := time.Now().Add(-time.Hour)
	mk := func(name string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("data\n"), mode); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(p, mode)
		_ = os.Chtimes(p, old, old)
		return p
	}
	gone := []string{mk("secret.123", 0o600), mk("secret.4294967295", 0o600), mk("secret.0", 0o600)}
	kept := map[string]string{
		"a backup":                   mk("secret.bak", 0o600),
		"notes":                      mk("secret.notes.txt", 0o600),
		"digits and letters":         mk("secret.123abc", 0o600),
		"letters and digits":         mk("secret.abc123", 0o600),
		"too many digits":            mk("secret.12345678901", 0o600),
		"a dot and nothing":          mk("secret.", 0o600),
		"two suffixes":               mk("secret.12.34", 0o600),
		"a temporary name, mode 644": mk("secret.555", 0o644),
		"another name":               mk("secrets.123", 0o600),
		"a prefix only":              mk("secre.123", 0o600),
		"the file itself":            mk("secret", 0o600),
	}
	_ = os.Mkdir(filepath.Join(dir, "secret.456"), 0o700)
	if err := os.Symlink(filepath.Join(dir, "secret"), filepath.Join(dir, "secret.789")); err == nil {
		_ = exec.Command("touch", "-h", "-t", "200001010000", filepath.Join(dir, "secret.789")).Run()
	}
	if _, _, err := Write(opts(dir)); err != nil {
		t.Fatal(err)
	}
	for _, p := range gone {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s: a stale temporary file was left", filepath.Base(p))
		}
	}
	for what, p := range kept {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s (%s) was removed: it is not a temporary file of this package", what, filepath.Base(p))
		}
	}
	if fi, err := os.Lstat(filepath.Join(dir, "secret.456")); err != nil || !fi.IsDir() {
		t.Error("a folder named like a temporary file was removed")
	}
	if _, err := os.Lstat(filepath.Join(dir, "secret.789")); err != nil {
		t.Error("a link named like a temporary file was removed")
	}
}

// A name made of glob characters widens nothing: with the file named `*`, the person's other files stay.
func TestAGlobNameRemovesNothingElse(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	old := time.Now().Add(-time.Hour)
	for _, n := range []string{"keep.txt", "keep.123", "a.123", "b.123", "x.123"} {
		p := filepath.Join(dir, n)
		_ = os.WriteFile(p, []byte("mine\n"), 0o600)
		_ = os.Chtimes(p, old, old)
	}
	for _, name := range []string{"*", "[ab]", "?"} {
		o := opts(dir)
		o.Name = name
		if _, _, err := Write(o); err != nil {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	for _, n := range []string{"keep.txt", "keep.123", "a.123", "b.123", "x.123"} {
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s was removed through a glob name", n)
		}
	}
}

// The name is swapped between the check and the open (a race an attacker in a writable folder could try): what was swapped in is not removed, and
// a pipe swapped in does not make Remove wait.
func TestARaceBetweenTheCheckAndTheOpenRemovesNothingElse(t *testing.T) {
	skipWithoutModes(t)
	saved := removeHook
	defer func() { removeHook = saved }()
	t.Run("another file with the same content", func(t *testing.T) {
		dir := priv(t)
		path := filepath.Join(dir, "secret")
		_ = os.WriteFile(path, []byte("token-1\n"), 0o600)
		other := filepath.Join(dir, "other")
		_ = os.WriteFile(other, []byte("token-1\n"), 0o600)
		removeHook = func() { _ = os.Rename(other, path) } // a different file, same size and content, is at the name now
		Remove(path, "token-1\n")
		if b, err := os.ReadFile(path); err != nil || string(b) != "token-1\n" {
			t.Error("a file that was swapped in after the check was removed")
		}
	})
	t.Run("a link to a file with the same content", func(t *testing.T) {
		dir := priv(t)
		path := filepath.Join(dir, "secret")
		_ = os.WriteFile(path, []byte("token-1\n"), 0o600)
		target := filepath.Join(priv(t), "t")
		_ = os.WriteFile(target, []byte("token-1\n"), 0o600)
		removeHook = func() { _ = os.Remove(path); _ = os.Symlink(target, path) }
		Remove(path, "token-1\n")
		if _, err := os.Stat(target); err != nil {
			t.Error("the link's target was removed")
		}
		if _, err := os.Lstat(path); err != nil {
			t.Error("a link that was swapped in after the check was removed")
		}
	})
	t.Run("a pipe", func(t *testing.T) {
		dir := priv(t)
		path := filepath.Join(dir, "secret")
		_ = os.WriteFile(path, []byte("token-1\n"), 0o600)
		removeHook = func() {
			_ = os.Remove(path)
			_ = syscall.Mkfifo(path, 0o600)
		}
		within(t, 3*time.Second, "Remove with a pipe swapped in", func() { Remove(path, "token-1\n") })
	})
}

// A huge file at the name is not read into memory: the size is checked before the open, and a read is capped at the content's size plus one.
func TestRemoveDoesNotReadAHugeFile(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	path := filepath.Join(dir, "secret")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil { // a sparse file of a gigabyte
		f.Close()
		t.Skipf("no sparse files here: %v", err)
	}
	f.Close()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	Remove(path, "token-1\n")
	runtime.ReadMemStats(&after)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20 {
		t.Errorf("Remove allocated %d bytes for a file it should not have read", grew)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("a file of another size was removed")
	}
}
