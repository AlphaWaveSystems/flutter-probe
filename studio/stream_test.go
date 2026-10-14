package main

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// frameFake answers the two calls captureFrame makes. The nil embedded interface makes any other call panic.
type frameFake struct {
	probelink.ProbeClient
	mu      sync.Mutex
	shotErr []error // one entry per call; nil means success; exhausted means success
	calls   int
	shot    []byte
	tree    string
	treeErr error
}

func (f *frameFake) ScreenshotBytes(context.Context, string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= len(f.shotErr) && f.shotErr[f.calls-1] != nil {
		return nil, f.shotErr[f.calls-1]
	}
	return f.shot, nil
}
func (f *frameFake) DumpWidgetTree(context.Context) (string, error) { return f.tree, f.treeErr }
func (f *frameFake) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestCaptureFrameBundlesScreenshotAndTree(t *testing.T) {
	f := &frameFake{shot: []byte("png-bytes"), tree: "Scaffold(...)"}
	frame, err := captureFrame(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := base64.StdEncoding.DecodeString(frame.Screenshot)
	if string(got) != "png-bytes" || frame.WidgetTree != "Scaffold(...)" || frame.TimestampMs == 0 {
		t.Errorf("frame = %+v", frame)
	}
}

func TestCaptureFrameKeepsTheImageWhenOnlyTheTreeFails(t *testing.T) {
	f := &frameFake{shot: []byte("png"), treeErr: errors.New("tree unavailable")}
	frame, err := captureFrame(context.Background(), f)
	if err != nil || frame.Screenshot == "" || frame.WidgetTree != "" {
		t.Errorf("a missing tree must not drop the frame: frame %+v, err %v", frame, err)
	}
}

func TestCaptureFrameFailsWhenTheScreenshotFails(t *testing.T) {
	f := &frameFake{shotErr: []error{errors.New("boom")}, tree: "x"}
	if _, err := captureFrame(context.Background(), f); err == nil {
		t.Fatal("want an error when there is no image")
	}
}

// The live loop reports an error, waits, and carries on until a capture works; cancelling stops it.
func TestStreamLoopBacksOffAfterAnErrorAndRecovers(t *testing.T) {
	old := streamErrorBackoff
	streamErrorBackoff = 5 * time.Millisecond
	defer func() { streamErrorBackoff = old }()

	hub := newEventHub()
	events, unsub := hub.subscribe()
	defer unsub()
	a := &App{ctx: context.Background(), hub: hub}
	f := &frameFake{shotErr: []error{errors.New("a"), errors.New("b")}, shot: []byte("png"), tree: "t"}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go a.streamLoop(ctx, done, f)

	var errs, frames int
	deadline := time.After(3 * time.Second)
	for frames < 2 {
		select {
		case ev := <-events:
			switch ev.Name {
			case "device:stream-error":
				errs++
				if frames > 0 {
					t.Error("an error event after the first good frame")
				}
			case "device:frame":
				frames++
			}
		case <-deadline:
			t.Fatalf("no recovery: %d errors, %d frames", errs, frames)
		}
	}
	if errs != 2 {
		t.Errorf("stream-error events = %d, want 2 (one per failed capture)", errs)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the loop must stop when its context is cancelled")
	}
	n := f.callCount()
	time.Sleep(30 * time.Millisecond)
	if f.callCount() != n {
		t.Error("the loop kept capturing after it stopped")
	}
}

func TestStreamLoopStopsDuringTheBackoff(t *testing.T) {
	old := streamErrorBackoff
	streamErrorBackoff = time.Hour // would hang the test if cancellation did not interrupt the wait
	defer func() { streamErrorBackoff = old }()
	a := &App{ctx: context.Background(), hub: newEventHub()}
	f := &frameFake{shotErr: []error{errors.New("a")}, shot: []byte("png")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go a.streamLoop(ctx, done, f)
	for f.callCount() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel must interrupt the backoff wait")
	}
}
