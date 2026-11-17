package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func createFixture(t *testing.T) (*App, string) {
	t.Helper()
	t.Setenv("PROBE_STUDIO_WORKSPACE", "")
	t.Setenv("PROBE_STUDIO_AUTOMATION_ROOTS", "")
	dir := t.TempDir()
	a := NewApp()
	a.SetWorkspace(dir)
	return a, dir
}

func TestCreateFileWritesANewFile(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "new.probe")
	if err := a.CreateFile(p, "test \"x\"\n"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "test \"x\"\n" {
		t.Errorf("content = %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

// The file appears between the check (the listing the window made) and the write: it is not replaced, and the answer says so.
func TestCreateFileNeverReplacesAFileThatAppearedAfterTheCheck(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "taken.probe")
	names := func() []string {
		es, _ := os.ReadDir(dir)
		var out []string
		for _, e := range es {
			out = append(out, e.Name())
		}
		return out
	}
	if len(names()) != 0 { // the listing the window checked the name against: empty
		t.Fatal("not empty")
	}
	if err := os.WriteFile(p, []byte("someone else's file"), 0o600); err != nil { // created after that listing
		t.Fatal(err)
	}
	err := a.CreateFile(p, "mine")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want a refusal that says the file already exists", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "someone else's file" {
		t.Errorf("the file was replaced: %q", b)
	}
}

func TestCreateFileRefusesALinkAFolderAndWhatIsNotAProbeFileOrOutsideTheWorkspace(t *testing.T) {
	a, dir := createFixture(t)
	target := filepath.Join(dir, "target.probe")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.probe")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if err := a.CreateFile(link, "x"); err == nil {
			t.Error("a link was written through")
		}
		dangling := filepath.Join(dir, "dangling.probe")
		if err := os.Symlink(filepath.Join(dir, "nowhere.probe"), dangling); err != nil {
			t.Fatal(err)
		}
		if err := a.CreateFile(dangling, "x"); err == nil {
			t.Error("a dangling link was written through")
		}
		if _, err := os.Stat(filepath.Join(dir, "nowhere.probe")); err == nil {
			t.Error("a file was created at the end of a dangling link")
		}
		if b, _ := os.ReadFile(target); string(b) != "keep" {
			t.Errorf("the target of a link was changed: %q", b)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d.probe"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{
		"a folder":         filepath.Join(dir, "d.probe"),
		"not a probe file": filepath.Join(dir, "x.txt"),
		"outside":          filepath.Join(filepath.Dir(dir), "outside.probe"),
		"a missing folder": filepath.Join(dir, "no", "such", "x.probe"),
	} {
		if err := a.CreateFile(p, "x"); err == nil {
			t.Errorf("%s: written", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "no")); err == nil {
		t.Error("a folder was created for the file")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside.probe")); err == nil {
		t.Error("a file was written outside the workspace")
	}
}

// A name that differs only by case, or by Unicode normalisation, is the same name on a file system that ignores either (APFS, NTFS): it is refused by
// comparing the listing, on every file system, and the exclusive create is the check that cannot race.
func TestCreateFileRefusesTheSameNameInAnotherCaseOrNormalisation(t *testing.T) {
	a, dir := createFixture(t)
	for existing, again := range map[string]string{
		"LOGIN.probe":       "login.probe",
		"e\u0301cran.probe": "\u00e9cran.probe",  // NFD on disk, NFC asked for
		"\u00e9cole.probe":  "e\u0301cole.probe", // NFC on disk, NFD asked for
		"\u00c9TE.probe":    "e\u0301te.probe",   // both at once
	} {
		p := filepath.Join(dir, existing)
		if err := os.WriteFile(p, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := a.CreateFile(filepath.Join(dir, again), "mine"); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("%q over %q: %v, want already exists", again, existing, err)
		}
		if b, _ := os.ReadFile(p); string(b) != "keep" {
			t.Errorf("%q was replaced: %q", existing, b)
		}
	}
}

func TestCreateFileRefusesNamesAFileSystemOrWindowsCannotHold(t *testing.T) {
	a, dir := createFixture(t)
	for _, bad := range []string{"CON.probe", "con.probe", "NUL.probe", "Aux.probe", "COM1.probe", "lpt9.probe", "CON.x.probe", "a..probe", "a .probe", "..probe", "a\u200bb.probe", "a\u202eb.probe", "a:b.probe", "a*b.probe", strings.Repeat("x", 251) + ".probe", "x.PROBE", "x.txt", ".x.probe", ".hidden.probe"} {
		if err := a.CreateFile(filepath.Join(dir, bad), "x"); err == nil {
			t.Errorf("%q was created", bad)
		}
	}
	for _, good := range []string{"console.probe", "com10.probe", "com0.probe", "lpt0.probe", "lpt10.probe", "com.probe", "lpt.probe", "a.b.probe", "ok name.probe", "\u00e9.probe"} {
		if err := a.CreateFile(filepath.Join(dir, good), "x"); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
}

// Many creates of one name at once: exactly one wins and the others say the file exists, and the winner's content is what is on disk.
func TestCreateFileOnlyOneOfManyConcurrentCreatesWins(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "race.probe")
	const n = 40
	errs := make(chan error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			<-start
			errs <- a.CreateFile(p, strings.Repeat(string(rune('a'+i%26)), 50))
		}(i)
	}
	close(start)
	won := 0
	for i := 0; i < n; i++ {
		if err := <-errs; err == nil {
			won++
		} else if !strings.Contains(err.Error(), "already exists") {
			t.Errorf("an unexpected refusal: %v", err)
		}
	}
	if won != 1 {
		t.Errorf("%d creates succeeded, want exactly one", won)
	}
	if b, _ := os.ReadFile(p); len(b) != 50 || strings.Trim(string(b), string(b[:1])) != "" {
		t.Errorf("the file holds mixed content: %q", b)
	}
}

// The listing check is tested as data, because a file system that ignores case or normalisation would refuse the create by itself and hide a missing check.
func TestSameNameAmongFoldsCaseAndNormalisation(t *testing.T) {
	for _, c := range []struct {
		entries    []string
		name, want string
	}{
		{[]string{"LOGIN.probe"}, "login.probe", "LOGIN.probe"},
		{[]string{"e\u0301cran.probe"}, "\u00e9cran.probe", "e\u0301cran.probe"},
		{[]string{"\u00e9cole.probe"}, "e\u0301cole.probe", "\u00e9cole.probe"},
		{[]string{"\u00c9TE.probe"}, "e\u0301te.probe", "\u00c9TE.probe"},
		{[]string{"other.probe"}, "login.probe", ""},
		{nil, "login.probe", ""},
	} {
		if got := sameNameAmong(c.entries, c.name); got != c.want {
			t.Errorf("%q among %q = %q, want %q", c.name, c.entries, got, c.want)
		}
	}
}

// The file appears between the listing and the exclusive create (the race the listing cannot see): it is not replaced.
func TestCreateFileNeverReplacesAFileThatAppearsBetweenTheListingAndTheCreate(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "late.probe")
	createFileAfterListing = func() { _ = os.WriteFile(p, []byte("theirs"), 0o600) }
	t.Cleanup(func() { createFileAfterListing = func() {} })
	err := a.CreateFile(p, "mine")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "theirs" {
		t.Errorf("the file was replaced: %q", b)
	}
}

// A write that fails part way leaves no half-written file, and the error is the writer's.
func TestCreateFileRemovesAHalfWrittenFile(t *testing.T) {
	a, dir := createFixture(t)
	p := filepath.Join(dir, "half.probe")
	boom := errors.New("disk full")
	createFileWrite = func(f *os.File, content string) (int, error) {
		n, _ := f.WriteString(content[:2])
		return n, boom
	}
	t.Cleanup(func() {
		createFileWrite = func(f *os.File, content string) (int, error) { return f.WriteString(content) }
	})
	if err := a.CreateFile(p, "test \"x\"\n"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the writer's error", err)
	}
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Errorf("the half-written file is still there: %v", err)
	}
}

// A new file has a size cap; one byte over is refused before anything is created, the cap itself is written.
func TestCreateFileHasASizeCap(t *testing.T) {
	a, dir := createFixture(t)
	over := filepath.Join(dir, "over.probe")
	if err := a.CreateFile(over, strings.Repeat("x", maxSaveBytes+1)); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("err = %v, want a size refusal", err)
	}
	if _, err := os.Lstat(over); !os.IsNotExist(err) {
		t.Error("a refused file was created")
	}
	at := filepath.Join(dir, "at.probe")
	if err := a.CreateFile(at, strings.Repeat("x", maxSaveBytes)); err != nil {
		t.Fatalf("a file of exactly the cap was refused: %v", err)
	}
}

// On a case-sensitive file system the exclusive create cannot see a twin that differs in case or normalisation: only the folded listing does.
// APFS and NTFS (case-insensitive) answer that case themselves, so the test is skipped there.
func TestCreateFileListingFoldsOnACaseSensitiveFileSystem(t *testing.T) {
	a, dir := createFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "a.probe"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "A.probe")); err == nil {
		t.Skip("this file system ignores case: the exclusive create refuses the twin without the listing")
	}
	for _, twin := range []string{"A.probe"} {
		if err := a.CreateFile(filepath.Join(dir, twin), "x"); err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("%s: err = %v, want a refusal that the file exists", twin, err)
		}
	}
	nfc, nfd := "\u00e9.probe", "e\u0301.probe"
	if err := os.WriteFile(filepath.Join(dir, nfc), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateFile(filepath.Join(dir, nfd), "x"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("the NFD twin: err = %v, want a refusal that the file exists", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.probe")); string(b) != "mine" {
		t.Errorf("a.probe = %q", b)
	}
}
