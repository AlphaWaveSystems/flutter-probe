package probelink

import (
	"encoding/json"
	"testing"
)

// view_ready_ms is read only as a plain integer from 0 to maxViewReadyMS; anything else, and its absence (an older agent), is -1.
func TestViewReadyMS(t *testing.T) {
	for raw, want := range map[string]int{
		``:         -1,
		`0`:        0,
		`1180`:     1180,
		`60000`:    60000,
		`60001`:    -1,
		`-1`:       -1,
		`12.5`:     -1,
		`1e3`:      -1,
		`"1180"`:   -1,
		`null`:     -1,
		`{"ms":1}`: -1,
		`true`:     -1,
	} {
		if got := viewReadyMS(json.RawMessage(raw)); got != want {
			t.Errorf("viewReadyMS(%s) = %d, want %d", raw, got, want)
		}
	}
}

// A ping answer with or without the field decodes, and the other fields are unchanged.
func TestPingResultWithViewReadyMS(t *testing.T) {
	for raw, want := range map[string]int{
		`{"ok":true,"agent_version":"0.28.0","capabilities":["press_key","waits_for_view_size"],"view_ready_ms":1180}`: 1180,
		`{"ok":true,"agent_version":"0.27.0","capabilities":["press_key"]}`:                                            -1,
		`{"ok":true,"agent_version":"0.28.0","view_ready_ms":"slow"}`:                                                  -1,
	} {
		var res PingResult
		if err := json.Unmarshal([]byte(raw), &res); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		h := newHandshakeResult(res)
		if h.ViewReadyMS != want || h.AgentVersion == "" {
			t.Errorf("%s: ViewReadyMS %d, want %d (agent %q)", raw, h.ViewReadyMS, want, h.AgentVersion)
		}
	}
}

// The capability names the agent reports.
func TestCapabilityWaitsForViewSizeName(t *testing.T) {
	if CapabilityWaitsForViewSize != "waits_for_view_size" {
		t.Errorf("CapabilityWaitsForViewSize = %q; want the name probe_agent reports", CapabilityWaitsForViewSize)
	}
}
