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

// signedTestKey mirrors the private issuing tool: the public CLI only verifies.
func signedTestKey(t *testing.T, c license.Claims) (pub, key string) {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	payload, _ := json.Marshal(c)
	sig := ed25519.Sign(sk, payload)
	return enc.EncodeToString(pk), strings.Join([]string{license.Prefix, enc.EncodeToString(payload), enc.EncodeToString(sig)}, ".")
}

func TestLicenseActivateAndStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{
		ID: "lic", Customer: "acme", Plans: []license.Plan{license.PlanTeam}, Seats: 5,
		Expires: time.Now().Add(24 * time.Hour),
	})
	t.Setenv(license.PublicKeyEnv, pub)

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
	pub, _ := signedTestKey(t, license.Claims{ID: "x", Customer: "c", Plans: []license.Plan{license.PlanTeam}})
	t.Setenv(license.PublicKeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{"FP1.bad.key"}); err == nil {
		t.Fatal("expected error")
	}
}
