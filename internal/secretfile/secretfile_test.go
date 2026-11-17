package secretfile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skipWithoutModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not describe who can read a file here")
	}
}

func priv(t *testing.T) string {
	t.Helper()
	d, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

func opts(dir string) Options {
	return Options{Label: "test file", Dir: dir, Name: "secret", Content: "token-1\n"}
}

func wantRefusal(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("no refusal")
	}
	for _, p := range append([]string{"test file: "}, parts...) {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("the refusal %q lacks %q", err, p)
		}
	}
}

// A new file is written 0600 with exactly the content, in a folder made 0700, and no temporary file is left.
func TestWritesAPrivateFileAndLeavesNothingElse(t *testing.T) {
	skipWithoutModes(t)
	dir := filepath.Join(priv(t), "made", "here")
	path, replaced, err := Write(opts(dir))
	if err != nil || replaced {
		t.Fatalf("write: %v replaced=%v", err, replaced)
	}
	if b, _ := os.ReadFile(path); string(b) != "token-1\n" {
		t.Errorf("content %q", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %04o", fi.Mode().Perm())
	}
	if di, _ := os.Stat(dir); di.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %04o", di.Mode().Perm())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("%d entries in the folder, want only the file", len(entries))
	}
	// a second write replaces it and says so
	o := opts(dir)
	o.Content = "token-2\n"
	if _, replaced, err := Write(o); err != nil || !replaced {
		t.Fatalf("second write: %v replaced=%v", err, replaced)
	}
	if b, _ := os.ReadFile(path); string(b) != "token-2\n" {
		t.Errorf("content after a second write %q", b)
	}
}

// Everything that is not the plain private file is refused, and nothing is written into or through it.
func TestRefusals(t *testing.T) {
	skipWithoutModes(t)
	t.Run("existing file open to others", func(t *testing.T) {
		dir := priv(t)
		path := filepath.Join(dir, "secret")
		for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o660} {
			_ = os.WriteFile(path, []byte("old\n"), 0o600)
			_ = os.Chmod(path, mode)
			_, _, err := Write(opts(dir))
			wantRefusal(t, err, "token file", path, "is open to other users")
			if b, _ := os.ReadFile(path); string(b) != "old\n" {
				t.Errorf("mode %04o: the refused file was written", mode)
			}
		}
	})
	t.Run("file link", func(t *testing.T) {
		dir := priv(t)
		target := filepath.Join(priv(t), "elsewhere")
		_ = os.WriteFile(target, []byte("keep\n"), 0o600)
		_ = os.Symlink(target, filepath.Join(dir, "secret"))
		_, _, err := Write(opts(dir))
		wantRefusal(t, err, "is a symlink or not a regular file")
		if b, _ := os.ReadFile(target); string(b) != "keep\n" {
			t.Error("the link target was written")
		}
	})
	t.Run("dangling link", func(t *testing.T) {
		dir := priv(t)
		target := filepath.Join(priv(t), "not-there")
		_ = os.Symlink(target, filepath.Join(dir, "secret"))
		_, _, err := Write(opts(dir))
		wantRefusal(t, err, "is a symlink or not a regular file")
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Error("the secret was written through the link")
		}
	})
	t.Run("not a regular file", func(t *testing.T) {
		dir := priv(t)
		_ = os.Mkdir(filepath.Join(dir, "secret"), 0o700)
		_, _, err := Write(opts(dir))
		wantRefusal(t, err, "is a symlink or not a regular file")
	})
	t.Run("private folder open to others", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o755, 0o750, 0o701} {
			dir := priv(t)
			_ = os.Chmod(dir, mode)
			_, _, err := Write(opts(dir))
			wantRefusal(t, err, "token directory", dir, "is open to other users")
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Errorf("mode %04o: written into the refused folder", mode)
			}
		}
	})
	t.Run("folder link", func(t *testing.T) {
		real := priv(t)
		link := filepath.Join(priv(t), "link")
		_ = os.Symlink(real, link)
		for _, shared := range []bool{false, true} {
			o := opts(link)
			o.SharedDir = shared
			_, _, err := Write(o)
			wantRefusal(t, err, "token directory", link, "is a symlink or not a directory")
			if entries, _ := os.ReadDir(real); len(entries) != 0 {
				t.Errorf("shared=%v: written through the folder link", shared)
			}
		}
	})
	t.Run("another user's folder and file", func(t *testing.T) {
		dir := priv(t)
		path := filepath.Join(dir, "secret")
		_ = os.WriteFile(path, []byte("old\n"), 0o600)
		saved := ownedByUs
		defer func() { ownedByUs = saved }()
		ownedByUs = func(os.FileInfo) bool { return false }
		_, _, err := Write(opts(dir))
		wantRefusal(t, err, "token directory", "belongs to another user")
		// only the file is someone else's
		calls := 0
		ownedByUs = func(os.FileInfo) bool { calls++; return calls == 1 }
		_, _, err = Write(opts(dir))
		wantRefusal(t, err, "token file", path, "belongs to another user")
		if b, _ := os.ReadFile(path); string(b) != "old\n" {
			t.Error("a file of another user was replaced")
		}
	})
}

// A folder the person named is judged as a folder: a shared one such as a home folder (0755) is fine, one that others can write to is not.
func TestASharedFolderMayBeReadableButNotWritableByOthers(t *testing.T) {
	skipWithoutModes(t)
	dir := priv(t)
	_ = os.Chmod(dir, 0o755)
	o := opts(dir)
	o.SharedDir = true
	if _, _, err := Write(o); err != nil {
		t.Fatalf("a readable folder of the person's own was refused: %v", err)
	}
	for _, mode := range []os.FileMode{0o775, 0o757, 0o777} {
		d := priv(t)
		_ = os.Chmod(d, mode)
		o := opts(d)
		o.SharedDir = true
		_, _, err := Write(o)
		wantRefusal(t, err, "token directory", "can be written by other users")
		if entries, _ := os.ReadDir(d); len(entries) != 0 {
			t.Errorf("mode %04o: written into the refused folder", mode)
		}
	}
}

// Remove removes the file only while it still holds this process's content.
func TestRemoveOnlyRemovesItsOwnContent(t *testing.T) {
	dir := priv(t)
	path, _, err := Write(opts(dir))
	if err != nil {
		t.Fatal(err)
	}
	Remove(path, "someone else's\n")
	if _, err := os.Stat(path); err != nil {
		t.Error("a file with another server's content was removed")
	}
	Remove(path, "token-1\n")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("its own file was not removed")
	}
	Remove(path, "token-1\n") // already gone: nothing happens
}
