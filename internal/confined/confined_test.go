package confined

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// under returns an allowed func for dir (resolved, as the real path of an open file is).
func under(t *testing.T, dir string) func(string) bool {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return func(real string) bool { return Under(r, real) }
}

func TestReadFileReadsAnAllowedRegularFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	write(t, p, "DATA")
	data, real, err := ReadFile(p, under(t, dir))
	if err != nil || string(data) != "DATA" {
		t.Fatalf("ReadFile = %q, %v", data, err)
	}
	if want, _ := filepath.EvalSymlinks(p); real != want {
		t.Errorf("real path = %q, want %q", real, want)
	}
	if got, err := Check(p, under(t, dir)); err != nil || got != real {
		t.Errorf("Check = %q, %v", got, err)
	}
}

func TestReadFileRefuses(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.png")
	write(t, outside, "SECRET")
	inside := filepath.Join(dir, "a.png")
	write(t, inside, "DATA")
	cases := map[string]string{"a file outside": outside, "a folder": dir, "nothing": filepath.Join(dir, "missing.png")}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.png")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		cases["a link out"] = link
		inLink := filepath.Join(dir, "inlink.png")
		if err := os.Symlink(inside, inLink); err != nil {
			t.Fatal(err)
		}
		cases["a link even to an allowed file"] = inLink
		hard := filepath.Join(dir, "hard.png")
		if err := os.Link(outside, hard); err == nil {
			cases["a hard link"] = hard
		}
	}
	for name, p := range cases {
		if data, _, err := ReadFile(p, under(t, dir)); err == nil || !errors.Is(err, ErrRefused) {
			t.Errorf("%s: ReadFile = %q, %v; want ErrRefused", name, data, err)
		}
		if _, err := Check(p, under(t, dir)); err == nil {
			t.Errorf("%s: Check accepted it", name)
		}
	}
	if _, _, err := ReadFile(inside, func(string) bool { return false }); !errors.Is(err, ErrRefused) {
		t.Errorf("a file allowed returns false for must be refused: %v", err)
	}
}

// ReadFile reads at most 64 MiB: a larger file is refused with ErrTooLarge (and ErrRefused); a file of exactly 64 MiB is read. Check has no
// limit. The files are sparse, so the test writes almost nothing.
func TestReadFileSizeCap(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, size int64) string {
		p := filepath.Join(dir, name)
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		return p
	}
	exact, over := mk("exact.png", maxSize), mk("over.png", maxSize+1)
	if data, _, err := ReadFile(exact, under(t, dir)); err != nil || len(data) != maxSize {
		t.Errorf("a file of exactly %d bytes must be read: %d, %v", maxSize, len(data), err)
	}
	_, _, err := ReadFile(over, under(t, dir))
	if !errors.Is(err, ErrTooLarge) || !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), "64 MiB") {
		t.Errorf("a larger file must be refused as too large: %v", err)
	}
	if _, err := Check(over, under(t, dir)); err != nil {
		t.Errorf("Check has no size limit: %v", err)
	}
}

func TestUnder(t *testing.T) {
	sep := string(filepath.Separator)
	base := sep + filepath.Join("a", "b")
	for p, want := range map[string]bool{
		base:                            true,
		filepath.Join(base, "c"):        true,
		sep + filepath.Join("a", "bc"):  false,
		sep + "a":                       false,
		filepath.Join(base, "..", "x"):  false,
		filepath.Join(base, "..c", "x"): true,
	} {
		if got := Under(base, filepath.Clean(p)); got != want {
			t.Errorf("Under(%q, %q) = %v, want %v", base, p, got, want)
		}
	}
}
