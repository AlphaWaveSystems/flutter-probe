//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A file that is not a regular file is not turned into one (probeFileForWriteWithinRoots refuses only a folder); a plain write to a FIFO would not have
// replaced it with a file.
func TestWriteFileRefusesATargetThatIsNotARegularFile(t *testing.T) {
	_, p := atomicFixture(t, "x")
	fifo := filepath.Join(filepath.Dir(p), "pipe.probe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("no FIFO here: %v", err)
	}
	if err := replaceFileAtomically(fifo, []byte("x"), ".probe-save-*"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a FIFO: err = %v", err)
	}
	if fi, _ := os.Lstat(fifo); fi.Mode()&os.ModeNamedPipe == 0 {
		t.Error("the FIFO was replaced by something else")
	}
}
