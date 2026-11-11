package main

import "testing"

// provider: none is an explicit off in the form too, and it survives a save and a reload, so the keychain key is not used.
func TestProviderNoneRoundTripsThroughTheForm(t *testing.T) {
	dir := writeProbeYAML(t, "agent:\n  port: 1\n")
	if err := writeAISettings(dir, AISettings{Provider: "none", KeySource: "keychain", Vision: true}); err != nil {
		t.Fatal(err)
	}
	got, err := readAISettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "none" {
		t.Fatalf("provider = %q, want none", got.Provider)
	}
}
