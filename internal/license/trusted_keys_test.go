package license

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// checkKey reports whether a trusted key is well formed: a key id that matches kidPattern and a public key that decodes
// strictly, without padding, to an Ed25519 public key. It is a release gate only: Verify accepts what it is given.
func checkKey(k TrustedKey) error {
	if !kidPattern.MatchString(k.KID) {
		return fmt.Errorf("key id %q does not match %s", k.KID, kidPattern)
	}
	pk, err := b64.DecodeString(k.PublicKey)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return fmt.Errorf("key %s: the public key is not unpadded base64url of %d bytes", k.KID, ed25519.PublicKeySize)
	}
	return nil
}

func TestCheckKeyAcceptsAWellFormedKey(t *testing.T) {
	_, pub := testKey(testSeed())
	if err := checkKey(TrustedKey{KID: "k1", PublicKey: pub}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckKeyRejectsAMalformedKey(t *testing.T) {
	_, pub := testKey(testSeed())
	cases := []struct {
		name string
		key  TrustedKey
	}{
		{"empty key id", TrustedKey{KID: "", PublicKey: pub}},
		{"upper case key id", TrustedKey{KID: "K1", PublicKey: pub}},
		{"key id over 16 characters", TrustedKey{KID: strings.Repeat("a", 17), PublicKey: pub}},
		{"key id with a dot", TrustedKey{KID: "k.1", PublicKey: pub}},
		{"padded public key", TrustedKey{KID: "k1", PublicKey: pub + "="}},
		{"standard base64 alphabet", TrustedKey{KID: "k1", PublicKey: strings.NewReplacer("-", "+", "_", "/").Replace(pub)}},
		{"public key of 31 bytes", TrustedKey{KID: "k1", PublicKey: b64.EncodeToString(make([]byte, 31))}},
		{"public key of 33 bytes", TrustedKey{KID: "k1", PublicKey: b64.EncodeToString(make([]byte, 33))}},
		{"public key not base64", TrustedKey{KID: "k1", PublicKey: "not a key!"}},
		{"empty public key", TrustedKey{KID: "k1", PublicKey: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkKey(tc.key); err == nil {
				t.Fatalf("checkKey(%+v) accepted a malformed key", tc.key)
			}
		})
	}
}

// TestEmbeddedTrustedKeysAreWellFormed checks the list a release embeds. It passes on an empty list. Once a production key
// is added, a typo in its key id or public key, or a key id listed twice, fails here before a release is cut.
func TestEmbeddedTrustedKeysAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range trustedKeys {
		if err := checkKey(k); err != nil {
			t.Errorf("embedded key: %v", err)
		}
		if seen[k.KID] {
			t.Errorf("key id %s is embedded twice", k.KID)
		}
		seen[k.KID] = true
	}
}

// embedKeys sets the embedded key list for one test and restores it afterwards. KeyEnv is cleared, so only the list counts.
func embedKeys(t *testing.T, keys ...TrustedKey) {
	t.Helper()
	t.Setenv(KeyEnv, "")
	saved := trustedKeys
	trustedKeys = keys
	t.Cleanup(func() { trustedKeys = saved })
}

// liveClaims are claims that are valid now, for a key signed at the current time.
func liveClaims() Claims {
	now := time.Now()
	return Claims{V: 2, KID: "k1", ID: "sub_EMBEDTEST", Customer: "cus_EMBEDTEST", Plans: []Plan{PlanProbeCloud}, Seats: 1,
		IssuedAt: now.Add(-time.Hour), Expires: now.Add(time.Hour)}
}

func TestLicenseVerifiesOnceItsKeyIsEmbedded(t *testing.T) {
	sk, pub := testKey(testSeed())
	key := testIssue(t, sk, "k1", liveClaims())

	embedKeys(t, TrustedKey{KID: "k1", PublicKey: pub})
	c, err := Verify(key)
	if err != nil {
		t.Fatalf("verify with the embedded key: %v", err)
	}
	if !c.Has(PlanProbeCloud) || c.Customer != "cus_EMBEDTEST" {
		t.Fatalf("claims = %+v", c)
	}
}

func TestLicenseIsRefusedWithoutAnEmbeddedKey(t *testing.T) {
	sk, _ := testKey(testSeed())
	key := testIssue(t, sk, "k1", liveClaims())

	embedKeys(t)
	if _, err := Verify(key); !errors.Is(err, ErrNoPublicKey) {
		t.Fatalf("with an empty list: %v, want ErrNoPublicKey", err)
	}
}

func TestLicenseSignedByAnotherKeyIsRefused(t *testing.T) {
	_, pub := testKey(testSeed())
	otherSK, _ := testKey(append([]byte{0xff}, make([]byte, 31)...))
	key := testIssue(t, otherSK, "k1", liveClaims())

	embedKeys(t, TrustedKey{KID: "k1", PublicKey: pub})
	c, err := Verify(key)
	if err == nil || c != nil {
		t.Fatalf("a key signed by another private key verified: claims %+v, err %v", c, err)
	}
	if !strings.Contains(err.Error(), "signature does not verify") {
		t.Fatalf("err = %v, want a signature failure", err)
	}
}
