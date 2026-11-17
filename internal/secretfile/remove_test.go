//go:build !windows

package secretfile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
