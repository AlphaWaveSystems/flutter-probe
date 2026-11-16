package main

import (
	"github.com/alphawavesystems/flutter-probe/internal/scriptgen"
)

// eventToProbeScriptLine converts a single recorded agent event into a ProbeScript step string (with leading 2-space indent), or "" to skip.
// The mapping is the engine's (internal/scriptgen), the same one `probe record` uses.
func eventToProbeScriptLine(params map[string]interface{}) string {
	return scriptgen.Step(params)
}

// sanitizeRecordedText makes recorded text safe for one ProbeScript line.
func sanitizeRecordedText(s string) string { return scriptgen.Sanitize(s) }
