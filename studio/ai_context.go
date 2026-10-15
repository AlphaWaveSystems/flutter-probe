package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Limits on what a chat turn sends to the model. Each piece of context is cut to its limit, and the cut is said in the
// text, so the model never reads a silently partial file or tree.
const (
	maxContextFileBytes = 64 * 1024
	maxContextRunBytes  = 8 * 1024
	maxContextTreeBytes = 32 * 1024
	maxRationaleBytes   = 400
)

// clipContext cuts s to at most limit bytes, on a character boundary, and says how much was left out.
func clipContext(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n[truncated: %d more bytes not sent]", len(s)-cut)
}

// summarizeRun describes the last run for the model: the test and step counts, the tests that failed with their errors,
// and the steps that failed with their lines. It returns "" when there is no run to describe.
func summarizeRun(st RunState) string {
	if st.File == "" && len(st.Results) == 0 && len(st.Steps) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "file: %s\n", st.File)
	passed, failed := 0, 0
	for _, r := range st.Results {
		switch {
		case r.Skipped:
		case r.Passed:
			passed++
		default:
			failed++
		}
	}
	fmt.Fprintf(&b, "tests: %d passed, %d failed\n", passed, failed)
	for _, r := range st.Results {
		if !r.Skipped && !r.Passed {
			fmt.Fprintf(&b, "FAILED test %q: %s\n", r.Name, r.Error)
		}
	}
	for _, s := range st.Steps {
		if s.Status == "failed" {
			fmt.Fprintf(&b, "FAILED step, line %d of test %q: %s (%s)\n", s.Line, s.Test, s.Description, s.Error)
		}
	}
	return clipContext(b.String(), maxContextRunBytes)
}

// lastFailedStep returns the most recent step that failed in the last run.
func lastFailedStep(st RunState) (RunStep, bool) {
	for i := len(st.Steps) - 1; i >= 0; i-- {
		if st.Steps[i].Status == "failed" {
			return st.Steps[i], true
		}
	}
	return RunStep{}, false
}

// firstProbeBlock finds the first fenced code block in a model reply. It returns the block's text and the reply with that
// block removed, so the rest of the reply can be read as the rationale.
func firstProbeBlock(reply string) (block, rest string, ok bool) {
	start := strings.Index(reply, "```")
	if start < 0 {
		return "", reply, false
	}
	body := reply[start+3:]
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:] // the info string after the opening fence, for example "probe"
	}
	end := strings.Index(body, "```")
	if end < 0 {
		return "", reply, false
	}
	block = strings.TrimSpace(body[:end])
	rest = reply[:start] + body[end+3:]
	return block, strings.TrimSpace(rest), true
}

// parseSelectorFix reads a selector-fix reply: exactly one ProbeScript line in a fenced block, and any text outside the
// block as the rationale. A reply with no block, or with more than one line in it, is refused.
func parseSelectorFix(reply string) (line, rationale string, ok bool) {
	block, rest, found := firstProbeBlock(reply)
	if !found {
		return "", "", false
	}
	var lines []string
	for _, l := range strings.Split(block, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if len(lines) != 1 {
		return "", "", false
	}
	return lines[0], clipContext(rest, maxRationaleBytes), true
}
