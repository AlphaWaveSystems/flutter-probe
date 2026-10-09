package runner

import "sync/atomic"

var looseMatching atomic.Bool

// SetLooseMatching turns loose text matching on for this process: text selectors then fold case,
// accents, typographic apostrophes/dashes, full-width forms and whitespace on both sides
// (`--match-loose`, `defaults.match: loose`). Ids (#key) are never folded.
func SetLooseMatching(on bool) { looseMatching.Store(on) }

// LooseMatching reports whether loose text matching is on.
func LooseMatching() bool { return looseMatching.Load() }
