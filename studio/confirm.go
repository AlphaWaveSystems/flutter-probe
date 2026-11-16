package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// A step that wipes the app's data (`clear app data`) asks first. In `probe test` it asks on the terminal or needs --yes; in Studio it asks in
// the window, and the answer is a button press by the person. Nothing here lets a script approve it: the id of a request is random, bound to the
// pending request and to the run that raised it, and used once; the automation endpoint has no method that reaches AnswerConfirm; and without an
// answer (no window, no run, a timeout, a cancel) the step is not run.

// confirmTimeout is how long the window has to answer. A variable so tests do not wait a minute.
var confirmTimeout = 60 * time.Second

const (
	kindClearData = "clear-app-data"
	maxConfirmTxt = 80
)

// ConfirmRequest is what the window shows. Every field is made by Studio from its own state (the app id of the workspace and the name of the
// connected device, cleaned), never from text the engine or a provider wrote.
type ConfirmRequest struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	AppID      string `json:"appId"`
	DeviceName string `json:"deviceName"`
}

type pendingConfirm struct {
	id   string
	run  context.Context
	done chan bool
}

type confirmState struct {
	mu      sync.Mutex
	pending *pendingConfirm
	slot    chan struct{} // one request at a time: a second step waits for the first answer
}

func (c *confirmState) init() {
	c.mu.Lock()
	if c.slot == nil {
		c.slot = make(chan struct{}, 1)
	}
	c.mu.Unlock()
}

// setConfirmRun records the run a confirmation would belong to; clearConfirmRun forgets it when the run is over.
func (a *App) setConfirmRun(run context.Context) {
	a.mu.Lock()
	a.confirmRun = run
	a.mu.Unlock()
}

func (a *App) clearConfirmRun() {
	a.mu.Lock()
	a.confirmRun = nil
	a.mu.Unlock()
}

// windowReady says whether something is listening for a request: the desktop window, or a page of the headless build.
func (a *App) windowReady() bool {
	if a.runtimeUp.Load() {
		return true
	}
	if a.hub != nil {
		a.hub.mu.Lock()
		defer a.hub.mu.Unlock()
		return len(a.hub.subs) > 0
	}
	return false
}

// confirmClearData is the runner.ConfirmFunc of a Studio connection. The text the engine passes is not used: the window shows Studio's own.
func (a *App) confirmClearData() runner.ConfirmFunc {
	return func(string) bool {
		a.mu.Lock()
		run := a.confirmRun
		conn := a.conn
		a.mu.Unlock()
		if run == nil || run.Err() != nil || conn == nil {
			return false // no run to ask for
		}
		if !a.windowReady() {
			return false // nobody to ask: the step is not run
		}
		a.confirm.init()
		select { // one request at a time; a run that ends while it waits is a no
		case a.confirm.slot <- struct{}{}:
		case <-run.Done():
			return false
		}
		defer func() { <-a.confirm.slot }()

		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return false
		}
		p := &pendingConfirm{id: hex.EncodeToString(raw), run: run, done: make(chan bool, 1)}
		a.confirm.mu.Lock()
		a.confirm.pending = p
		a.confirm.mu.Unlock()
		appID := ""
		if conn.deviceCtx != nil {
			appID = conn.deviceCtx.AppID
		}
		a.emit("confirm:request", ConfirmRequest{ID: p.id, Kind: kindClearData, AppID: dialogText(appID), DeviceName: dialogText(conn.deviceName)})

		timer := time.NewTimer(confirmTimeout)
		defer timer.Stop()
		var allow, answered bool
		var note string
		select {
		case allow = <-p.done:
			answered = true
		case <-timer.C:
			note = "Clear app data was not run: nothing answered in the window"
		case <-run.Done():
			note = "Clear app data was not run: the run was cancelled"
		}
		a.confirm.mu.Lock()
		if a.confirm.pending == p {
			a.confirm.pending = nil
		}
		a.confirm.mu.Unlock()
		a.emit("confirm:close", p.id)
		if !answered {
			a.emit("confirm:note", note)
			return false
		}
		return allow
	}
}

// AnswerConfirm is the window's answer to the request it showed. It counts only for the pending request with this id, raised by the run that is
// still going, and only once: any other call (a second window, a repeat, an old id) changes nothing and returns false.
func (a *App) AnswerConfirm(id string, allow bool) bool {
	a.confirm.mu.Lock()
	p := a.confirm.pending
	if p == nil || subtle.ConstantTimeCompare([]byte(p.id), []byte(id)) != 1 || p.run.Err() != nil {
		a.confirm.mu.Unlock()
		return false
	}
	a.confirm.pending = nil
	a.confirm.mu.Unlock()
	p.done <- allow // buffered: the waiting step reads it, or has already given up and nobody does
	return true
}

// shortText makes a name from the device list or the workspace safe to show in the dialog: one line, no invisible characters, bounded.
func dialogText(s string) string {
	t := oneLine(s)
	r := []rune(t)
	if len(r) > maxConfirmTxt {
		return string(r[:maxConfirmTxt]) + "…"
	}
	return t
}
