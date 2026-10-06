package probelink

import (
	"encoding/json"
	"testing"
)

func TestReportWarning(t *testing.T) {
	var got []string
	SetWarningHandler(func(w string) { got = append(got, w) })
	defer SetWarningHandler(nil)

	reportWarning(json.RawMessage(`{"ok":true}`))
	reportWarning(json.RawMessage(`{"ok":true,"warning":"covered"}`))
	reportWarning(json.RawMessage(`not json`))
	reportWarning(nil)

	if len(got) != 1 || got[0] != "covered" {
		t.Errorf("only a real warning field is forwarded, got %v", got)
	}
}

func TestReportWarningWithoutAHandlerIsHarmless(t *testing.T) {
	SetWarningHandler(nil)
	reportWarning(json.RawMessage(`{"warning":"x"}`)) // must not panic
}
