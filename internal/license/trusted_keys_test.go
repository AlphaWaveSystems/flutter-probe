package license

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"reflect"
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

// checkKeys returns one error for each malformed key in a list, and one for each key id that is listed a second time.
func checkKeys(keys []TrustedKey) []error {
	var errs []error
	seen := map[string]bool{}
	for _, k := range keys {
		if err := checkKey(k); err != nil {
			errs = append(errs, err)
		}
		if seen[k.KID] {
			errs = append(errs, fmt.Errorf("key id %s is listed twice", k.KID))
		}
		seen[k.KID] = true
	}
	return errs
}

// TestEmbeddedTrustedKeysAreWellFormed checks the list a release embeds. The list is empty in this release, so the test
// passes; once a production key is added it fails on a typo in that key's id or public key, or on a key id listed twice.
func TestEmbeddedTrustedKeysAreWellFormed(t *testing.T) {
	for _, err := range checkKeys(trustedKeys) {
		t.Errorf("embedded key: %v", err)
	}
}

func TestCheckKeysAcceptsAGoodList(t *testing.T) {
	_, pub := testKey(testSeed())
	_, pub2 := testKey(append([]byte{0xff}, make([]byte, 31)...))
	if errs := checkKeys([]TrustedKey{{KID: "k1", PublicKey: pub}, {KID: "k2", PublicKey: pub2}}); len(errs) != 0 {
		t.Fatalf("a good list gave %v", errs)
	}
	if errs := checkKeys(nil); len(errs) != 0 {
		t.Fatalf("the empty list gave %v", errs)
	}
}

func TestCheckKeysReportsADuplicateKeyID(t *testing.T) {
	_, pub := testKey(testSeed())
	_, pub2 := testKey(append([]byte{0xff}, make([]byte, 31)...))
	errs := checkKeys([]TrustedKey{{KID: "k1", PublicKey: pub}, {KID: "k1", PublicKey: pub2}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "listed twice") {
		t.Fatalf("errs = %v, want one duplicate key id error", errs)
	}
}

func TestCheckKeysReportsAMalformedKey(t *testing.T) {
	_, pub := testKey(testSeed())
	errs := checkKeys([]TrustedKey{{KID: "k1", PublicKey: pub}, {KID: "k2", PublicKey: "not a key!"}})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "key k2") {
		t.Fatalf("errs = %v, want one error for key k2", errs)
	}
}

func TestCheckKeysReportsEveryProblem(t *testing.T) {
	_, pub := testKey(testSeed())
	// The second entry repeats k1 and has a malformed public key: a duplicate and a malformed key, so two errors.
	errs := checkKeys([]TrustedKey{{KID: "k1", PublicKey: pub}, {KID: "k1", PublicKey: "bad"}})
	if len(errs) != 2 {
		t.Fatalf("errs = %v, want the duplicate and the malformed key", errs)
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

// TestEmbedKeysRestoresTheList runs embedKeys in a subtest, whose cleanup finishes before the subtest returns. The list must
// be back to what it was whatever order the tests run in, so no test sees another test's keys.
func TestEmbedKeysRestoresTheList(t *testing.T) {
	before := trustedKeys
	t.Run("sets a key", func(t *testing.T) {
		embedKeys(t, TrustedKey{KID: "k1", PublicKey: "unused"})
		if len(trustedKeys) != 1 {
			t.Fatalf("embedKeys did not set the list: %v", trustedKeys)
		}
	})
	if !reflect.DeepEqual(trustedKeys, before) {
		t.Fatalf("trustedKeys = %v after embedKeys, want the original %v", trustedKeys, before)
	}
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
