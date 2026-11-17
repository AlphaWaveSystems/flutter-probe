package probelink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSeeFilledOnTheWire checks `see ... is filled` and `is empty` on both clients. An agent that does not know the two state words
// answers ok for any element, so it would pass `is filled` on an empty field. The CLI therefore sends the check only to an agent that
// reports CapabilitySeeFilled, and refuses it before sending to any other, whether the agent predates the capability, predates the
// handshake, or was never handshaken. The other state words and a plain see are not gated.
func TestSeeFilledOnTheWire(t *testing.T) {
	selector := SelectorParam{Kind: "id", Text: "pw"}
	for _, wc := range wireClients {
		for _, check := range []string{"filled", "empty"} {
			t.Run(wc.name+" "+check+": a capable agent gets the step", func(t *testing.T) {
				agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilitySeeFilled}, actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
					t.Fatalf("Handshake: %v", err)
				}
				if err := client.See(context.Background(), SeeParams{Selector: selector, Check: check}); err != nil {
					t.Fatalf("See: %v", err)
				}
				var got map[string]any
				if err := json.Unmarshal(<-agent.actions, &got); err != nil {
					t.Fatal(err)
				}
				if got["check"] != check {
					t.Errorf("check on the wire = %v; want %q", got["check"], check)
				}
			})

			t.Run(wc.name+" "+check+": an agent without the capability is refused before sending", func(t *testing.T) {
				agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilityPressKey}, actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
					t.Fatalf("Handshake: %v", err)
				}
				err := client.See(context.Background(), SeeParams{Selector: selector, Check: check})
				if err == nil || !strings.Contains(err.Error(), "see ... is filled / is empty needs a probe_agent") || !strings.Contains(err.Error(), "v0.27.0") {
					t.Errorf("See: %v; want a refusal that names the step and the agent version", err)
				}
				select {
				case raw := <-agent.actions:
					t.Errorf("the agent received %s; want nothing sent", raw)
				default:
				}
			})

			t.Run(wc.name+" "+check+": an agent that predates the handshake is refused", func(t *testing.T) {
				agent := keyAgent{actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
					t.Fatalf("Handshake: %v", err)
				}
				err := client.See(context.Background(), SeeParams{Selector: selector, Check: check})
				if err == nil || !strings.Contains(err.Error(), "predates the version handshake") {
					t.Errorf("See: %v; want a refusal that says the agent predates the handshake", err)
				}
				select {
				case raw := <-agent.actions:
					t.Errorf("the agent received %s; want nothing sent", raw)
				default:
				}
			})

			t.Run(wc.name+" "+check+": without a handshake it is refused", func(t *testing.T) {
				agent := keyAgent{version: "0.27.0", capabilities: []string{CapabilitySeeFilled}, actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if err := client.See(context.Background(), SeeParams{Selector: selector, Check: check}); err == nil {
					t.Error("See without a handshake: want a refusal")
				}
				select {
				case raw := <-agent.actions:
					t.Errorf("the agent received %s; want nothing sent", raw)
				default:
				}
			})
		}

		for _, check := range []string{"", "enabled", "disabled", "checked", "focused"} {
			t.Run(wc.name+" the check "+check+" is not gated", func(t *testing.T) {
				agent := keyAgent{version: "0.27.0", actions: make(chan json.RawMessage, 1)}
				client, done := wc.connect(t, agent)
				defer done()
				if _, err := client.Handshake(context.Background(), "0.27.0"); err != nil {
					t.Fatalf("Handshake: %v", err)
				}
				if err := client.See(context.Background(), SeeParams{Selector: selector, Check: check}); err != nil {
					t.Fatalf("See with check %q: %v", check, err)
				}
				select {
				case <-agent.actions:
				default:
					t.Errorf("the agent received nothing for check %q; want the step sent", check)
				}
			})
		}
	}
}
