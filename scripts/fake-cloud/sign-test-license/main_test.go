package main

import (
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
