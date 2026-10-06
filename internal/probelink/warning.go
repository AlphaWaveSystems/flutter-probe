package probelink

import (
	"encoding/json"
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

// reportWarning forwards a {"warning": "..."} field from an RPC result, if any.
func reportWarning(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var r struct {
		Warning string `json:"warning"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Warning == "" {
		return
	}
	warnMu.RLock()
	f := warningSink
	warnMu.RUnlock()
	if f != nil {
		f(r.Warning)
	}
}
