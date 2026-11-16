package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/license"
)

// The license the tool prints verifies with the public key it prints, carries the plan an upload needs, and expires after a day.
func TestTheTestLicenseVerifiesWithItsOwnKey(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	pubEnv, key := sign(now)
	kid, pub, ok := strings.Cut(pubEnv, "=")
	if !ok || kid != "localtest" {
		t.Fatalf("pubEnv = %q", pubEnv)
	}
	ks := license.KeySet{Keys: []license.TrustedKey{{KID: kid, PublicKey: pub}}}
	c, err := license.VerifyWith(key, ks, now.Add(time.Hour))
	if err != nil || !c.Has(license.Plan("probe_cloud")) || c.Seats != 1 {
		t.Fatalf("claims %+v, err %v", c, err)
	}
	if _, err := license.VerifyWith(key, ks, now.Add(25*time.Hour)); err != license.ErrExpired {
		t.Errorf("after a day: %v, want expired", err)
	}
	// a key the tool signed is not trusted by anything else: another run's public key does not verify it
	otherPub, _ := sign(now)
	_, other, _ := strings.Cut(otherPub, "=")
	if _, err := license.VerifyWith(key, license.KeySet{Keys: []license.TrustedKey{{KID: "localtest", PublicKey: other}}}, now); err == nil {
		t.Error("a different key pair verified the license")
	}
	// a released CLI trusts no such key: with no key set at all, nothing verifies
	if _, err := license.VerifyWith(key, license.KeySet{}, now); err != license.ErrNoPublicKey {
		t.Errorf("with no trusted key: %v", err)
	}
}

// The tool makes nothing when the service address is not this machine, and says why without printing a key.
func TestItRefusesAServiceThatIsNotThisMachine(t *testing.T) {
	for _, ok := range []string{"", "http://127.0.0.1:48899", "http://localhost:1", "https://127.0.0.1", "http://[::1]:8080"} {
		if err := checkLoopback(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"https://api.flutterprobe.dev", "http://example.com", "http://10.0.0.5:48899", "http://127.0.0.1.example.com", "not a url", "http://"} {
		if err := checkLoopback(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestTheLicenseIsMarkedAsATestOne(t *testing.T) {
	_, key := sign(time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(key, ".")[2])
	if !strings.Contains(string(payload), "LOCAL-TEST-ONLY-NOT-A-REAL-LICENSE") || !strings.Contains(string(payload), `"kid":"localtest"`) {
		t.Errorf("payload = %s", payload)
	}
}
