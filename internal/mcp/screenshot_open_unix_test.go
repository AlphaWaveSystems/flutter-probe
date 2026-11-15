//go:build !windows

package mcp

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A named pipe put in place of the picture after it was checked must not make the open wait for a writer that never comes.
func TestOpenNoBlockDoesNotWaitOnANamedPipe(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "a_1.png")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("named pipes not available:", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f, err := openNoBlock(fifo)
		if err == nil {
			fi, serr := f.Stat()
			if serr != nil || fi.Mode().IsRegular() {
				t.Errorf("the pipe looks like a regular file: %v %v", fi, serr)
			}
			f.Close()
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the open of a named pipe blocked")
	}
}
