package probelink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestCapabilityGateIgnoresTheVersionNumber pins that a step is allowed by the capabilities an agent reports, not by its version.
// The release version and the build's version can disagree: a build labelled 0.27.0 may report press_key, and a build with a
// high number may not report it. Both clients are checked.
func TestCapabilityGateIgnoresTheVersionNumber(t *testing.T) {
	for _, wc := range wireClients {
		t.Run(wc.name+" a high version without the capability is refused", func(t *testing.T) {
			agent := keyAgent{version: "99.0.0", actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			err := client.DeviceAction(context.Background(), "press_key", "tab")
			if err == nil || !strings.Contains(err.Error(), "press <key> needs a probe_agent") || !strings.Contains(err.Error(), "v99.0.0") {
				t.Errorf("DeviceAction press_key from an agent v99.0.0 without the capability: %v; want a refusal that names the version", err)
			}
			select {
			case raw := <-agent.actions:
				t.Errorf("the agent received %s; want nothing sent", raw)
			default:
			}
		})

		t.Run(wc.name+" the capability is accepted whatever the version", func(t *testing.T) {
			agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)}
			client, done := wc.connect(t, agent)
			defer done()
			if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
				t.Fatalf("Handshake: %v", err)
			}
			if err := client.DeviceAction(context.Background(), "press_key", "tab"); err != nil {
				t.Fatalf("DeviceAction press_key from an agent v0.27.0 with the capability: %v", err)
			}
			checkDeviceActionParams(t, <-agent.actions, "press_key", "tab")
		})
	}
}

// TestSwipeFromEveryDirectionOnTheWire checks that a swipe with a start carries its direction and start_percent for each of the
// four directions, on both clients.
func TestSwipeFromEveryDirectionOnTheWire(t *testing.T) {
	menu := &SelectorParam{Kind: "text", Text: "Menu"}
	for _, wc := range wireClients {
		for _, direction := range []string{"up", "down", "left", "right"} {
			t.Run(wc.name+" "+direction, func(t *testing.T) {
				agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilitySwipeFrom}, actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
					t.Fatalf("Handshake: %v", err)
				}
				if err := client.Swipe(context.Background(), direction, menu, 2, 0); err != nil {
					t.Fatalf("Swipe %s from 2 percent: %v", direction, err)
				}
				var got map[string]any
				if err := json.Unmarshal(<-agent.actions, &got); err != nil {
					t.Fatalf("params: %v", err)
				}
				if got["direction"] != direction {
					t.Errorf("direction = %v, want %s", got["direction"], direction)
				}
				if got["start_percent"] != float64(2) {
					t.Errorf("start_percent = %v, want 2 (params %v)", got["start_percent"], got)
				}
			})
		}
	}
}
