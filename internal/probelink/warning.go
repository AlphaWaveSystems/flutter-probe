package probelink

import (
	"encoding/json"
	"fmt"
	"sync"
)

var (
	warnMu      sync.RWMutex
	warningSink func(string)
)

// SetWarningHandler registers where agent warnings go (the CLI prints them).
// A warning is a successful call the agent still wants the user to know about,
// e.g. a tap whose target is covered by another widget (FP-19). Nil disables.
func SetWarningHandler(f func(string)) {
	warnMu.Lock()
	warningSink = f
	warnMu.Unlock()
}

var strictWarnings bool

// SetStrictWarnings makes agent warnings fail the call that produced them
// (`probe test --fail-on-warning`): a tap that provably did nothing, a `press enter`
// with no focused field, a `go back` at the root route. Off by default.
func SetStrictWarnings(on bool) {
	warnMu.Lock()
	strictWarnings = on
	warnMu.Unlock()
}

// reportWarning forwards a {"warning": "..."} field from an RPC result, if any.
// It returns an error only in strict mode (see SetStrictWarnings).
func reportWarning(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var r struct {
		Warning string `json:"warning"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Warning == "" {
		return nil
	}
	warnMu.RLock()
	f := warningSink
	strict := strictWarnings
	warnMu.RUnlock()
	if f != nil {
		f(r.Warning)
	}
	if strict {
		return fmt.Errorf("agent warning treated as an error (--fail-on-warning): %s", r.Warning)
	}
	return nil
}

// emitWarning sends a CLI-side warning to the same sink as agent warnings.
func emitWarning(msg string) {
	warnMu.RLock()
	f := warningSink
	warnMu.RUnlock()
	if f != nil {
		f(msg)
	}
}
