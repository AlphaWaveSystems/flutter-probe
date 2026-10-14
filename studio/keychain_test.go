package main

import (
	"testing"

	"github.com/zalando/go-keyring"
)

// The API key lives in the platform keychain. keyring.MockInit swaps the real keychain for memory, so the test
// never touches the developer's login keychain.
func TestAPIKeyRoundTripThroughTheKeychain(t *testing.T) {
	keyring.MockInit()
	a := &App{}

	if got, err := a.GetAPIKey(); err != nil || got != "" {
		t.Fatalf("no key stored: got %q, err %v; want an empty string and no error", got, err)
	}
	if err := a.SetAPIKey("test-key-value"); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.GetAPIKey(); got != "test-key-value" {
		t.Errorf("stored key = %q", got)
	}
	// An empty key means "forget it".
	if err := a.SetAPIKey(""); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.GetAPIKey(); got != "" {
		t.Errorf("after clearing, key = %q", got)
	}
	if err := a.DeleteAPIKey(); err == nil {
		t.Error("deleting a key that is not there should report it")
	}
}
