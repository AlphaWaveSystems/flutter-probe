//go:build darwin || linux

package confined

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO is refused at once: the open does not wait for a writer.
func TestReadFileRefusesAFIFO(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.png")
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := ReadFile(p, under(t, dir)); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a FIFO was read")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ReadFile blocked on a FIFO")
	}
}
