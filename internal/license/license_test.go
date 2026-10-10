package license

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	pub, priv := testKeypair(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	key := testIssue(t, priv, Claims{
		ID: "lic_1", Customer: "acme", Plans: []Plan{PlanTeam}, Seats: 10,
		IssuedAt: now, Expires: now.Add(30 * 24 * time.Hour),
	})
	c, err := VerifyWith(key, pub, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if c.Customer != "acme" || !c.Has(PlanTeam) || c.Has(Plan("other")) || c.Seats != 10 {
		t.Fatalf("claims: %+v", c)
	}
}

func TestVerifyRejects(t *testing.T) {
	pub, priv := testKeypair(t)
	otherPub, _ := testKeypair(t)
	now := time.Now()
	key := testIssue(t, priv, Claims{ID: "x", Customer: "c", Plans: []Plan{PlanTeam}, Expires: now.Add(time.Hour)})

	cases := map[string]struct {
		key, pub string
		at       time.Time
		want     error
	}{
		"wrong key":          {key, otherPub, now, nil},
		"tampered payload":   {flipPart(t, key, 1), pub, now, nil},
		"tampered signature": {flipPart(t, key, 2), pub, now, nil},
		"non-canonical sig":  {nonCanonicalSig(t, key), pub, now, nil},
		"malformed":          {"FP1.abc", pub, now, nil},
		"bad pub":            {key, "nope", now, nil},
		"expired":            {key, pub, now.Add(2 * time.Hour), ErrExpired},
		"no pub key":         {key, "", now, ErrNoPublicKey},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := VerifyWith(tc.key, tc.pub, tc.at)
			if err == nil {
				t.Fatal("expected error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestVerifyUsesEnvKey(t *testing.T) {
	pub, priv := testKeypair(t)
	t.Setenv(PublicKeyEnv, pub)
	key := testIssue(t, priv, Claims{ID: "x", Customer: "c", Plans: []Plan{Plan("hosted")}})
	c, err := Verify(key)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Has(Plan("hosted")) {
		t.Fatal("plan missing")
	}
}

// flipPart returns key with one bit of the decoded part (1 = payload, 2 =
// signature) inverted, so the result always differs from the original.
func flipPart(t *testing.T, key string, part int) string {
	t.Helper()
	parts := strings.Split(key, ".")
	raw, err := base64.RawURLEncoding.DecodeString(parts[part])
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 0x01
	parts[part] = base64.RawURLEncoding.EncodeToString(raw)
	return strings.Join(parts, ".")
}

// nonCanonicalSig re-spells the signature with non-zero trailing bits in its
// last character; it decodes to the same bytes under a lax decoder.
func nonCanonicalSig(t *testing.T, key string) string {
	t.Helper()
	parts := strings.Split(key, ".")
	sig := parts[2]
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	idx := strings.IndexByte(alphabet, sig[len(sig)-1])
	parts[2] = sig[:len(sig)-1] + string(alphabet[idx^0x01])
	return strings.Join(parts, ".")
}
