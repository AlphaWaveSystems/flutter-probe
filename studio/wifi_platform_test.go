package main

import (
	"context"
	"testing"
)

// A WiFi connection reports no platform the agent or its mDNS record did not
// give. It must not be reported as iOS, which is what an Android phone would
// otherwise show.
func TestConnectWiFiDoesNotReportIOS(t *testing.T) {
	agent := newFakeAgent(t)
	a := NewApp()
	a.ctx = context.Background()
	t.Cleanup(func() { a.shutdown(context.Background()) })

	if _, err := a.ConnectWiFi("127.0.0.1", agent.port, "abcdefgh1234"); err != nil {
		t.Fatalf("ConnectWiFi: %v", err)
	}
	if got := a.Status().Platform; got != "unknown" {
		t.Fatalf("platform %q, want %q: a WiFi connection must not be reported as iOS", got, "unknown")
	}
}
