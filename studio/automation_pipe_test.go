//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A pipe planted at the discovery file's name makes the write refuse, and Studio's start-up (which closes the server after a refused write) must
// not wait on it: a start that cannot proceed says so and returns.
func TestAPipeAtTheDiscoveryFileDoesNotHangTheStart(t *testing.T) {
	dir := discoveryDir(t)
	file := filepath.Join(dir, "automation.json")
	if err := syscall.Mkfifo(file, 0o600); err != nil {
		t.Skipf("no pipes here: %v", err)
	}
	t.Setenv("PROBE_STUDIO_AUTOMATION_FILE", file)
	done := make(chan error, 1)
	go func() {
		s, err := startAutomation(&fakeBackend{})
		if s != nil {
			s.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the server started over a pipe")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the start hung on a pipe at the discovery file's name")
	}
	if fi, err := os.Lstat(file); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		t.Error("the pipe was replaced or removed")
	}
}
