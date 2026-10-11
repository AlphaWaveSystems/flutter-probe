package cli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/license"
)

// signedTestKey mirrors the issuing side: the public CLI only verifies. It returns the
// value for the PROBE_LICENSE_PUBKEY override and a key signed with a throwaway keypair.
func signedTestKey(t *testing.T, c license.Claims) (keyEnv, key string) {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	c.V, c.KID = 2, "t1"
	if c.IssuedAt.IsZero() {
		c.IssuedAt = time.Now().Add(-time.Hour)
	}
	payload, _ := json.Marshal(c)
	seg := enc.EncodeToString(payload)
	sig := ed25519.Sign(sk, []byte(license.Prefix+".t1."+seg))
	return "t1=" + enc.EncodeToString(pk), strings.Join([]string{license.Prefix, "t1", seg, enc.EncodeToString(sig)}, ".")
}

func TestLicenseActivateAndStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{
		ID: "lic", Customer: "acme", Plans: []license.Plan{license.PlanTeam}, Seats: 5,
		Expires: time.Now().Add(24 * time.Hour),
	})
	t.Setenv(license.KeyEnv, pub)

	var out bytes.Buffer
	licenseActivateCmd.SetOut(&out)
	if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "acme") || !strings.Contains(out.String(), "5 seats") {
		t.Fatalf("activate output: %s", out.String())
	}

	out.Reset()
	licenseStatusCmd.SetOut(&out)
	if err := runLicenseStatus(licenseStatusCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Licensed to acme") {
		t.Fatalf("status output: %s", out.String())
	}
}

func TestLicenseStatusWithoutKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	licenseStatusCmd.SetOut(&out)
	if err := runLicenseStatus(licenseStatusCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No license") {
		t.Fatalf("status output: %s", out.String())
	}
}

func TestLicenseActivateRejectsBadKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pub, _ := signedTestKey(t, license.Claims{ID: "x", Customer: "c", Plans: []license.Plan{license.PlanTeam}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{"FP1.bad.key"}); err == nil {
		t.Fatal("expected error")
	}
}
