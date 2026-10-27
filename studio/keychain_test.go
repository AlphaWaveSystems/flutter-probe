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

	// GetAPIKey reports presence only, so the test checks what it reports, not a key value.
	if has, err := a.GetAPIKey(); err != nil || has {
		t.Fatalf("no key stored: has %v, err %v; want false and no error", has, err)
	}
	if err := a.SetAPIKey("test-key-value"); err != nil {
		t.Fatal(err)
	}
	if has, _ := a.GetAPIKey(); !has {
		t.Error("key stored but GetAPIKey reports none")
	}
	// An empty key means "forget it".
	if err := a.SetAPIKey(""); err != nil {
		t.Fatal(err)
	}
	if has, _ := a.GetAPIKey(); has {
		t.Error("after clearing, GetAPIKey still reports a key")
	}
	if err := a.DeleteAPIKey(); err == nil {
		t.Error("deleting a key that is not there should report it")
	}
}
