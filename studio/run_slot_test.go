package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// The run state machine, tested directly: the run slot (claimRun, releaseRun, CancelRun, runActive) and the run state a run leaves behind
// (RunState). Every guard has a test that fails when the guard is removed.

// A slot is free, claimed or being cancelled. Cancelling stops the run's context and keeps the slot until the run gives it back.
func TestTheRunSlotMovesFreeClaimedCancelledReleased(t *testing.T) {
	a := &App{ctx: context.Background()}
	if a.runActive() {
		t.Fatal("a new app has a run")
	}
	a.CancelRun() // no run: nothing happens
	ctx, cancel, err := a.claimRun()
	if err != nil || ctx == nil || !a.runActive() {
		t.Fatalf("claim on a free slot: %v active=%v", err, a.runActive())
	}
	if ctx.Err() != nil {
		t.Error("a new run starts cancelled")
	}
	// a second claim is refused at once, and it does not disturb the run that holds the slot
	if c2, cancel2, err := a.claimRun(); err != errRunInProgress || c2 != nil || cancel2 != nil {
		t.Errorf("second claim: %v %v %v, want errRunInProgress and nothing", err, c2, cancel2)
	}
	if ctx.Err() != nil || !a.runActive() {
		t.Error("a refused claim changed the run that holds the slot")
	}
	a.CancelRun()
	a.CancelRun() // twice is the same as once
	if ctx.Err() == nil {
		t.Error("CancelRun did not cancel the run's context")
	}
	if !a.runActive() {
		t.Error("CancelRun gave the slot back: the run has not finished yet")
	}
	if _, _, err := a.claimRun(); err != errRunInProgress {
		t.Errorf("a cancelled run that has not ended still holds the slot, got %v", err)
	}
	a.releaseRun(cancel)
	if a.runActive() {
		t.Error("releaseRun left the slot taken")
	}
	if ctx.Err() == nil {
		t.Error("releaseRun left the run's context alive")
	}
	free, freeCancel, _ := a.claimRun() // a run that ends normally also stops its context
	a.releaseRun(freeCancel)
	if free.Err() == nil {
		t.Error("a run that ended normally left its context alive")
	}
	// free again: a new run is not born cancelled by the old one
	ctx2, cancel2, err := a.claimRun()
	if err != nil || ctx2.Err() != nil {
		t.Fatalf("claim after release: %v err=%v", err, ctx2.Err())
	}
	a.releaseRun(cancel2)
	a.CancelRun() // after the end: nothing to cancel, and no panic
}

// The slot is held by one claimant, whoever races for it.
func TestOnlyOneOfManyConcurrentStartsGetsTheSlot(t *testing.T) {
	a := &App{ctx: context.Background()}
	var won, refused atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	var cancels sync.Map
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, cancel, err := a.claimRun()
			switch err {
			case nil:
				won.Add(1)
				cancels.Store(i, cancel)
			case errRunInProgress:
				refused.Add(1)
			default:
				t.Errorf("claim: %v", err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if won.Load() != 1 || refused.Load() != 63 {
		t.Fatalf("%d claims won, %d refused; want 1 and 63", won.Load(), refused.Load())
	}
	cancels.Range(func(_, c any) bool { a.releaseRun(c.(context.CancelFunc)); return true })
	if a.runActive() {
		t.Error("the slot is still taken")
	}
}

// A run that cannot start (a scope that does not resolve) gives its slot back before the call returns: the next run is not refused for it.
func TestARunThatCannotStartDoesNotKeepTheSlot(t *testing.T) {
	a, file, fake := blockedApp(t)
	defer close(fake.gate)
	if err := a.runFileAsync(file, RunScope{Tag: "no-such-tag"}); err == nil {
		t.Fatal("a scope that matches nothing started a run")
	}
	if a.runActive() {
		t.Error("a run that never started holds the slot")
	}
	if err := a.runFileAsync(filepath.Join(t.TempDir(), "outside.probe"), RunScope{}); err == nil {
		t.Fatal("a file outside the folders started a run")
	}
	if a.runActive() {
		t.Error("a refused path holds the slot")
	}
	// not connected: refused before the slot is taken
	b := &App{ctx: context.Background()}
	if err := b.runFileAsync(file, RunScope{}); err != errNotConnected || b.runActive() {
		t.Errorf("no connection: %v active=%v", err, b.runActive())
	}
}

// blockedApp is an app whose device does not answer a tap until the test opens the gate, with a workspace that holds one test file.
func blockedApp(t *testing.T) (*App, string, *blockFake) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "a.probe")
	if err := os.WriteFile(file, []byte("test \"t\"\n  tap \"Go\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), hub: newEventHub()}
	a.SetWorkspace(dir)
	cfg := &config.Config{}
	cfg.Defaults.Timeout = 5 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	fake := &blockFake{started: make(chan struct{}), gate: make(chan struct{})}
	a.conn = &connection{client: fake, cfg: cfg, streamCancel: cancel, streamDone: done}
	go a.streamLoop(ctx, done, &closeFake{frameFake: frameFake{shot: []byte("s")}})
	t.Cleanup(func() { cancel(); <-done })
	return a, file, fake
}

func waitStarted(t *testing.T, fake *blockFake) {
	t.Helper()
	select {
	case <-fake.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the run never reached the device")
	}
}

func waitFree(t *testing.T, a *App) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for a.runActive() {
		select {
		case <-deadline:
			t.Fatal("the run never gave its slot back")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// A run that was cancelled ends as finished, with the reason, no step in flight and its slot back; the next run starts from a clean state.
func TestAFinishAfterACancelLeavesAFinishedStateAndAFreeSlot(t *testing.T) {
	a, file, fake := blockedApp(t)
	if err := a.RunFileAsync(file); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, fake)
	st := a.RunState()
	if !st.Running || st.Finished || st.Current == nil {
		t.Fatalf("while it runs: %+v", st)
	}
	if err := a.RunFileAsync(file); err != errRunInProgress {
		t.Errorf("a second start while it runs: %v", err)
	}
	a.CancelRun()
	waitFree(t, a)
	st = a.RunState()
	if st.Running || !st.Finished || st.Current != nil || !strings.Contains(st.Error, "cancelled") {
		t.Fatalf("after cancel: running=%v finished=%v current=%v error=%q", st.Running, st.Finished, st.Current, st.Error)
	}
	a.CancelRun() // after the end: nothing, and the state is the same
	if again := a.RunState(); again.Running || !again.Finished || again.Error != st.Error {
		t.Errorf("a late CancelRun changed the state: %+v", again)
	}
	// a new run is not born finished, cancelled or carrying the old error
	fake2 := &blockFake{started: make(chan struct{}), gate: make(chan struct{})}
	a.mu.Lock()
	a.conn.client = fake2
	a.mu.Unlock()
	if err := a.RunFileAsync(file); err != nil {
		t.Fatalf("the slot was not free for the next run: %v", err)
	}
	waitStarted(t, fake2)
	if st := a.RunState(); !st.Running || st.Finished || st.Error != "" || len(st.Results) != 0 {
		t.Errorf("a new run starts with the state of the old one: %+v", st)
	}
	close(fake2.gate)
	waitFree(t, a)
	if st := a.RunState(); st.Running || !st.Finished || st.Error != "" || len(st.Results) != 1 || !st.Results[0].Passed {
		t.Errorf("after a clean finish: %+v", st)
	}
}

// A run is cancelled while it is still starting (its slot is taken, nothing has run): the context is cancelled, and the run ends as cancelled,
// not as a pass.
func TestACancelWhileStartingStopsTheRunBeforeItsSteps(t *testing.T) {
	a, file, fake := blockedApp(t)
	defer close(fake.gate)
	ctx, cancel, err := a.claimRun()
	if err != nil {
		t.Fatal(err)
	}
	a.CancelRun() // the run has the slot and has not begun
	if ctx.Err() == nil {
		t.Fatal("the cancel did not reach a run that was starting")
	}
	path, err := probeFileWithinRoots(a.automationRoots(), file)
	if err != nil {
		t.Fatal(err)
	}
	sel, plan, err := a.resolveRunScope(path, RunScope{})
	if err != nil {
		t.Fatal(err)
	}
	_, runErr := a.executeRun(ctx, path, sel, plan)
	a.releaseRun(cancel)
	if runErr == nil || !strings.Contains(runErr.Error(), "cancelled") {
		t.Errorf("a run cancelled before it began returned %v", runErr)
	}
	if st := a.RunState(); st.Running || !st.Finished || st.Error == "" {
		t.Errorf("state after: %+v", st)
	}
	if a.CanExportRunResults() {
		t.Error("a run that was cancelled left results to export") // a stopped run is partial (this fake device answers a tap with a nil error when it is cancelled, as a pass: the run state's error is what says it was cancelled)
	}
	if a.runActive() {
		t.Error("the slot is still held")
	}
}

// Another workspace opened while a run goes: the run finishes in the folder it started in, its results are not offered for export in the new
// workspace (their names would be relative to the wrong folder), and the slot and the state are as for any finish.
func TestAWorkspaceChangedDuringARunLeavesNothingToExportThere(t *testing.T) {
	a, file, fake := blockedApp(t)
	if err := a.RunFileAsync(file); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, fake)
	other := t.TempDir()
	a.SetWorkspace(other)
	close(fake.gate)
	waitFree(t, a)
	st := a.RunState()
	if st.Running || !st.Finished || st.Error != "" || len(st.Results) != 1 {
		t.Fatalf("the run did not finish cleanly: %+v", st)
	}
	if a.CanExportRunResults() {
		results, _, ws := a.export.get()
		t.Errorf("a run of another workspace is offered for export: %d results attributed to %q (open workspace %q)", len(results), ws, other)
	}
}

// A workspace that is the same one again (re-opened) during a run does not drop the results: only another folder does.
func TestTheSameWorkspaceReopenedDuringARunKeepsItsResults(t *testing.T) {
	a, file, fake := blockedApp(t)
	if err := a.RunFileAsync(file); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, fake)
	a.mu.Lock()
	same := a.workspace
	a.mu.Unlock()
	a.useWorkspace(same)
	close(fake.gate)
	waitFree(t, a)
	if !a.CanExportRunResults() {
		t.Error("the results of a run in the workspace that is still open are not exportable")
	}
}
