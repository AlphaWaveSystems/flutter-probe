package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Save in place (WriteFile) replaces a file through a new file in the same folder, so a write that fails part way leaves the person's file as it was.

func atomicFixture(t *testing.T, original string) (*App, string) {
	t.Helper()
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	a := NewApp()
	a.SetWorkspace(dir)
	p := filepath.Join(dir, "keep.probe")
	if err := os.WriteFile(p, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	return a, p
}

// onlyFile fails unless dir holds exactly the named file: no temporary file is left behind.
func onlyFile(t *testing.T, dir, name string) {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range es {
		got = append(got, e.Name())
	}
	if len(got) != 1 || got[0] != name {
		t.Errorf("the folder holds %v, want only %s", got, name)
	}
}

func TestWriteFileReplacesTheContentAndKeepsTheMode(t *testing.T) {
	a, p := atomicFixture(t, "old\n")
	if err := a.WriteFile(p, "new content\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new content\n" {
		t.Errorf("content = %q", b)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want the existing 0600 kept", fi.Mode().Perm())
		}
	}
	onlyFile(t, filepath.Dir(p), "keep.probe")
}

func TestWriteFileOfANewFileIs0644AndCreatesItsFolder(t *testing.T) {
	a, p := atomicFixture(t, "x")
	n := filepath.Join(filepath.Dir(p), "sub", "deeper", "new.probe")
	if err := a.WriteFile(n, "hello"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(n); string(b) != "hello" {
		t.Errorf("content = %q", b)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(n); fi.Mode().Perm() != 0o644 {
			t.Errorf("mode = %v", fi.Mode().Perm())
		}
	}
}

// A writer that fails after N bytes: the original is byte for byte what it was, the error is the writer's, and no temporary file is left.
func TestAFailingWriterAtAnyPointLeavesTheOriginalAndNoTemporaryFile(t *testing.T) {
	const original = "test \"keep\"\n  log \"the person's file\"\n"
	content := strings.Repeat("replacement line\n", 50)
	for _, n := range []int{0, 1, len(content) / 2, len(content) - 1} {
		a, p := atomicFixture(t, original)
		boom := errors.New("disk full")
		atomicWrite = func(f *os.File, b []byte) (int, error) {
			w, _ := f.Write(b[:n])
			return w, boom
		}
		err := a.WriteFile(p, content)
		atomicWrite = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
		if !errors.Is(err, boom) {
			t.Fatalf("n=%d: err = %v, want the writer's error", n, err)
		}
		if b, _ := os.ReadFile(p); string(b) != original {
			t.Errorf("n=%d: the original changed: %q", n, b)
		}
		onlyFile(t, filepath.Dir(p), "keep.probe")
	}
}

func TestAFailingSyncOrRenameLeavesTheOriginalAndNoTemporaryFile(t *testing.T) {
	const original = "the original\n"
	for _, c := range []struct {
		name string
		set  func() (restore func())
	}{
		{"sync", func() func() {
			atomicSync = func(*os.File) error { return errors.New("sync failed") }
			return func() { atomicSync = func(f *os.File) error { return f.Sync() } }
		}},
		{"rename", func() func() {
			atomicRename = func(string, string) error { return errors.New("rename failed") }
			return func() { atomicRename = os.Rename }
		}},
	} {
		a, p := atomicFixture(t, original)
		restore := c.set()
		err := a.WriteFile(p, "new")
		restore()
		if err == nil || !strings.Contains(err.Error(), c.name+" failed") {
			t.Fatalf("%s: err = %v", c.name, err)
		}
		if b, _ := os.ReadFile(p); string(b) != original {
			t.Errorf("%s: the original changed: %q", c.name, b)
		}
		onlyFile(t, filepath.Dir(p), "keep.probe")
	}
}

// The new file is flushed to disk before it takes the name: the order the writes happen in.
func TestTheContentIsFlushedBeforeTheRename(t *testing.T) {
	a, p := atomicFixture(t, "old")
	var order []string
	atomicSync = func(f *os.File) error { order = append(order, "sync"); return f.Sync() }
	atomicRename = func(o, n string) error { order = append(order, "rename"); return os.Rename(o, n) }
	err := a.WriteFile(p, "new")
	atomicSync = func(f *os.File) error { return f.Sync() }
	atomicRename = os.Rename
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "sync,rename" {
		t.Errorf("order = %v, want sync then rename", order)
	}
}

// A link inside the workspace that points at a file inside it: the file it points at is replaced and the link stays a link (the same as before).
func TestWriteFileThroughALinkReplacesTheTargetAndKeepsTheLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	a, target := atomicFixture(t, "target old")
	link := filepath.Join(filepath.Dir(target), "link.probe")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := a.WriteFile(link, "through the link"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link is gone or no longer a link: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "through the link" {
		t.Errorf("target = %q", b)
	}
}

// The results export uses the same writer: a failed write leaves the report that was there.
func TestTheReportWriterIsTheSameAtomicWriter(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report.json")
	if err := os.WriteFile(p, []byte("old report"), 0o644); err != nil {
		t.Fatal(err)
	}
	atomicWrite = func(f *os.File, b []byte) (int, error) { return 0, errors.New("disk full") }
	err := writeReportFile(p, []byte("new report"))
	atomicWrite = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	if err == nil || !strings.Contains(err.Error(), "writing the report") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "old report" {
		t.Errorf("report = %q", b)
	}
	onlyFile(t, dir, "report.json")
}

// A read-only file is not replaced: the plain write it replaces failed with "permission denied", and the rename would have replaced it.
func TestWriteFileRefusesAReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not describe this on Windows")
	}
	a, p := atomicFixture(t, "locked")
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	err := a.WriteFile(p, "new")
	if err == nil || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v, want permission denied", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "locked" {
		t.Errorf("the read-only file changed: %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o444 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	onlyFile(t, filepath.Dir(p), "keep.probe")
}

// A writable file in a folder that is not writable cannot be replaced (a temporary file is needed): it fails, says why, and changes nothing.
func TestWriteFileInAFolderThatIsNotWritableSaysSo(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("folder modes do not stop this here")
	}
	a, p := atomicFixture(t, "old")
	dir := filepath.Dir(p)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	err := a.WriteFile(p, "new")
	if err == nil || !strings.Contains(err.Error(), "cannot create a temporary file next to keep.probe: the folder is not writable") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Errorf("content = %q", b)
	}
}

// The temporary file has a random name made exclusively: two writes use two names, and a link planted at a name one could guess is not followed.
func TestTheTemporaryFileNameIsRandomAndALinkAtAGuessableNameIsNotFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need a privilege on Windows")
	}
	a, p := atomicFixture(t, "old")
	dir := filepath.Dir(p)
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("victim"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, guess := range []string{".probe-save", ".probe-save-0", ".probe-save-1", ".probe-save-fixed", ".probe-save.tmp", "keep.probe.tmp", ".keep.probe.tmp"} {
		if err := os.Symlink(victim, filepath.Join(dir, guess)); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	atomicCreateTemp = func(d, pattern string) (*os.File, error) {
		f, err := os.CreateTemp(d, pattern)
		if err == nil {
			names = append(names, filepath.Base(f.Name()))
		}
		return f, err
	}
	defer func() { atomicCreateTemp = os.CreateTemp }()
	for _, c := range []string{"one", "two"} {
		if err := a.WriteFile(p, c); err != nil {
			t.Fatal(err)
		}
	}
	if len(names) != 2 || names[0] == names[1] || !strings.HasPrefix(names[0], ".probe-save-") || names[0] == ".probe-save-fixed" {
		t.Errorf("temporary names = %v, want two different random ones", names)
	}
	if b, _ := os.ReadFile(victim); string(b) != "victim" {
		t.Errorf("a planted link was followed: %q", b)
	}
}

// A panic between the creation of the temporary file and the rename does not leave it behind.
func TestAPanicDuringTheWriteLeavesNoTemporaryFile(t *testing.T) {
	a, p := atomicFixture(t, "old")
	atomicWrite = func(*os.File, []byte) (int, error) { panic("boom") }
	defer func() { atomicWrite = func(f *os.File, b []byte) (int, error) { return f.Write(b) } }()
	func() {
		defer func() { _ = recover() }()
		_ = a.WriteFile(p, "new")
	}()
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Errorf("content = %q", b)
	}
	onlyFile(t, filepath.Dir(p), "keep.probe")
}
