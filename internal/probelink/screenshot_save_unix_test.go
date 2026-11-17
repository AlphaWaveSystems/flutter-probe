//go:build darwin || linux

package probelink

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO named like a screenshot neither blocks the run nor is read: only a regular file is.
func TestScreenshotWithoutDataRefusesAFIFO(t *testing.T) {
	t.Chdir(t.TempDir())
	root := fakeSimulators(t)
	fifo := filepath.Join(root, simUUID, "data", "shot.png")
	if err := os.MkdirAll(filepath.Dir(fifo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := savedScreenshot(ScreenshotResult{Path: fifo}); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a FIFO was accepted as a screenshot")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reading a FIFO blocked the run")
	}
}
