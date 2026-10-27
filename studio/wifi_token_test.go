package main

import (
	"context"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// A WiFi token is kept for each device in the platform keychain. keyring.MockInit keeps the test off the login keychain.
func TestWiFiTokenIsSavedPerDeviceAndForgottenOneAtATime(t *testing.T) {
	keyring.MockInit()
	a := &App{}

	if saved, err := a.WiFiTokenSaved("Pat's phone"); err != nil || saved {
		t.Fatalf("nothing saved yet: saved %v, err %v; want false and no error", saved, err)
	}
	if err := a.SaveWiFiToken("Pat's phone", goodToken); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveWiFiToken("Desk phone", goodToken); err != nil {
		t.Fatal(err)
	}
	if saved, _ := a.WiFiTokenSaved("Pat's phone"); !saved {
		t.Error("a token was saved for Pat's phone, but WiFiTokenSaved reports none")
	}
	if saved, _ := a.WiFiTokenSaved("Kitchen tablet"); saved {
		t.Error("a device with no saved token reports one")
	}
	if err := a.ForgetWiFiToken("Pat's phone"); err != nil {
		t.Fatal(err)
	}
	if saved, _ := a.WiFiTokenSaved("Pat's phone"); saved {
		t.Error("after forgetting, Pat's phone still reports a saved token")
	}
	if saved, _ := a.WiFiTokenSaved("Desk phone"); !saved {
		t.Error("forgetting one device removed the token saved for another")
	}
}

func TestSaveWiFiTokenRefusesAnEmptyDeviceOrToken(t *testing.T) {
	keyring.MockInit()
	a := &App{}
	if err := a.SaveWiFiToken("", goodToken); err == nil {
		t.Error("a token with no device name was saved")
	}
	if err := a.SaveWiFiToken("Desk phone", "   "); err == nil {
		t.Error("a blank token was saved")
	}
}

// ConnectWiFiSaved needs a saved token, and it runs the same target checks as ConnectWiFi: a public address is refused
// before anything is dialled, and the refusal does not repeat the token.
func TestConnectWiFiSavedUsesTheSavedTokenAndTheSameChecks(t *testing.T) {
	keyring.MockInit()
	t.Setenv(allowPublicEnv, "")
	a := &App{ctx: context.Background(), lookupHost: stubResolver(nil)}

	if _, err := a.ConnectWiFiSaved("8.8.8.8", 48686, "Desk phone"); err == nil || !strings.Contains(err.Error(), "no token is saved") {
		t.Fatalf("no saved token: err %v; want a message that no token is saved", err)
	}
	if err := a.SaveWiFiToken("Desk phone", goodToken); err != nil {
		t.Fatal(err)
	}
	_, err := a.ConnectWiFiSaved("8.8.8.8", 48686, "Desk phone")
	if err == nil || strings.Contains(err.Error(), goodToken) {
		t.Fatalf("public target with a saved token: err %v; want a refusal that does not repeat the token", err)
	}
}
