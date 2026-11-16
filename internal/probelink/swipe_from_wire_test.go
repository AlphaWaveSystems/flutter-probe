package probelink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// checkSwipeStartParams decodes the params of a swipe to the target "Menu" and checks the start point and the distance: each
// field is left out when it is 0, and sent with its value otherwise.
func checkSwipeStartParams(t *testing.T, raw json.RawMessage, start, percent int) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("params %s: %v", raw, err)
	}
	if got["direction"] != "right" {
		t.Errorf("direction = %v, want right (params %s)", got["direction"], raw)
	}
	if _, ok := got["selector"]; !ok {
		t.Errorf("selector left out (params %s), want the target sent", raw)
	}
	if v, ok := got["start_percent"]; start == 0 && ok {
		t.Errorf("start_percent %v was sent, want it left out (params %s)", v, raw)
	} else if start != 0 && (!ok || v != float64(start)) {
		t.Errorf("start_percent = %v (present %v), want %d (params %s)", v, ok, start, raw)
	}
	if v, ok := got["percent"]; percent == 0 && ok {
		t.Errorf("percent %v was sent, want it left out (params %s)", v, raw)
	} else if percent != 0 && (!ok || v != float64(percent)) {
		t.Errorf("percent = %v (present %v), want %d (params %s)", v, ok, percent, raw)
	}
}

// TestSwipeFromCapabilityName pins the name the agent reports: probe_agent lists 'swipe_from' in probeAgentCapabilities. The
// wire tests use the constant on both sides, so only this test catches a rename that would make every agent look too old.
func TestSwipeFromCapabilityName(t *testing.T) {
	if CapabilitySwipeFrom != "swipe_from" {
		t.Errorf("CapabilitySwipeFrom = %q; want %q, the name probe_agent reports", CapabilitySwipeFrom, "swipe_from")
	}
}

// TestSwipeFromOnTheWire checks the start point of a swipe on both clients. An agent that reports the swipe_from capability
// gets start_percent with the target. An agent without it, or no handshake at all, is refused before anything is sent. A swipe
// without a start is sent as it was, to an agent without the capability too.
func TestSwipeFromOnTheWire(t *testing.T) {
	menu := &SelectorParam{Kind: "text", Text: "Menu"}
	for _, wc := range wireClients {
		t.Run(wc.name+" capable agent gets the start and the distance", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilitySwipeFrom}, actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			if err := client.Swipe(context.Background(), "right", menu, 2, 40); err != nil {
				t.Fatalf("Swipe from 2 percent over 40 percent: %v", err)
			}
			checkSwipeStartParams(t, <-agent.actions, 2, 40)
		})

		t.Run(wc.name+" agent without the capability is refused before sending", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			err := client.Swipe(context.Background(), "right", menu, 2, 0)
			if err == nil || !strings.Contains(err.Error(), "swipe from needs a probe_agent") || !strings.Contains(err.Error(), "v0.27.0") {
				t.Errorf("Swipe from: %v; want a refusal that names the agent version", err)
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" swipe from without a handshake is refused", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilitySwipeFrom}, actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if err := client.Swipe(context.Background(), "right", menu, 2, 0); err == nil {
				t.Error("Swipe from without a handshake: want a refusal")
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" swipe without a start goes to an agent without the capability", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			if err := client.Swipe(context.Background(), "right", menu, 0, 0); err != nil {
				t.Fatalf("Swipe without a start: %v", err)
			}
			checkSwipeStartParams(t, <-agent.actions, 0, 0)
		})
	}
}
