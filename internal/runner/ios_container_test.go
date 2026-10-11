package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWipeIOSContainerEmptiesAndRecreates(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "Documents", "hive"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "Library", "Preferences"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "Documents", "hive", "box.hive"), []byte("x"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "Library", "Preferences", "app.plist"), []byte("onboarded"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "tmp-keep.txt"), []byte("outside the three dirs"), 0o644))

	must(wipeIOSContainer(dir))

	for _, gone := range []string{"Documents/hive/box.hive", "Library/Preferences/app.plist"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s must be gone (err=%v)", gone, err)
		}
	}
	for _, want := range []string{"Documents", "tmp", "Library/Caches", "Library/Preferences", "Library/Application Support"} {
		if fi, err := os.Stat(filepath.Join(dir, want)); err != nil || !fi.IsDir() {
			t.Errorf("%s must exist as an empty directory (err=%v)", want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "tmp-keep.txt")); err != nil {
		t.Errorf("files outside Documents/Library/tmp are left alone: %v", err)
	}
	// wiping an already-empty / missing container is fine
	must(wipeIOSContainer(filepath.Join(dir, "nonexistent")))
}
