//go:build !windows

package main

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A named pipe in the workspace is refused with the content-free note and does not block startup: opening it would wait for a writer.
func TestSeededResultsRefusesANamedPipeWithoutBlocking(t *testing.T) {
	ws, _ := seedIn(t, "[]")
	fifo := filepath.Join(ws, "pipe.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("named pipes are not available here")
	}
	done := make(chan string, 1)
	go func() {
		_, note := seededResults(true, fifo, ws)
		done <- note
	}()
	select {
	case note := <-done:
		if !strings.Contains(note, "PROBE_STUDIO_SEED_RUN was ignored") || !strings.Contains(note, "not a regular file") || strings.Contains(note, fifo) {
			t.Fatalf("note = %q", note)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("seededResults blocked on a named pipe")
	}
}
