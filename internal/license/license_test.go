package license

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The shared test vector (signer and verifier use the same bytes).
const (
	vectorPub  = "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg"
	vectorKey  = "FP2.k1.eyJ2IjoyLCJraWQiOiJrMSIsImlkIjoic3ViX1RFU1RWRUNUT1IiLCJjdXN0b21lciI6ImN1c19URVNUVkVDVE9SIiwicGxhbnMiOlsidGVhbSJdLCJzZWF0cyI6MTAsImlhdCI6IjIwMjYtMTEtMDFUMDA6MDA6MDBaIiwiZXhwIjoiMjAyNi0xMi0wNFQwMDowMDowMFoifQ.IgbwiDzLQBGQZBUeViBUV2xCMzdFspHbGVs488OFnAxbQsst2b-5wvogE06GKwt7yAdgwl5JYoXhnOozyZNWAA"
	vectorJSON = `{"v":2,"kid":"k1","id":"sub_TESTVECTOR","customer":"cus_TESTVECTOR","plans":["team"],"seats":10,"iat":"2026-11-01T00:00:00Z","exp":"2026-12-04T00:00:00Z"}`
)

var vectorNow = time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

func vectorKeys() KeySet { return KeySet{Keys: []TrustedKey{{KID: "k1", PublicKey: vectorPub}}} }

func TestVectorVerifies(t *testing.T) {
	c, err := VerifyWith(vectorKey, vectorKeys(), vectorNow)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "sub_TESTVECTOR" || c.Customer != "cus_TESTVECTOR" || !c.Has(PlanTeam) || c.Seats != 10 || c.KID != "k1" || c.V != 2 {
		t.Fatalf("claims: %+v", c)
	}
	if !c.Expires.Equal(time.Date(2026, 12, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("exp = %v", c.Expires)
	}
}

func TestSeedDerivesTheVectorKeyAndSignature(t *testing.T) {
	sk, pub := testKey(testSeed())
	if pub != vectorPub {
		t.Fatalf("seed 00..1f gives public key %s, the vector says %s", pub, vectorPub)
	}
	if got := testIssueRaw(sk, "k1", []byte(vectorJSON)); got != vectorKey {
		t.Fatalf("re-signing the vector claims gives a different key:\n%s\n%s", got, vectorKey)
	}
}

func TestVerifyRejects(t *testing.T) {
	sk, pub := testKey(testSeed())
	otherSK, otherPub := testKey(append([]byte{0xff}, make([]byte, 31)...))
	ks := KeySet{Keys: []TrustedKey{{KID: "k1", PublicKey: pub}, {KID: "k2", PublicKey: otherPub}}}
	good := Claims{V: 2, KID: "k1", ID: "x", Customer: "c", Plans: []Plan{PlanTeam}, Seats: 3,
		IssuedAt: vectorNow, Expires: vectorNow.Add(24 * time.Hour)}
	withClaims := func(mod func(*Claims)) string {
		c := good
		mod(&c)
		return testIssue(t, sk, "k1", c)
	}
	parts := strings.Split(vectorKey, ".")
	flip := func(i int) string { // change one character of segment i
		p := append([]string(nil), parts...)
		b := []byte(p[i])
		if b[10] == 'A' {
			b[10] = 'B'
		} else {
			b[10] = 'A'
		}
		p[i] = string(b)
		return strings.Join(p, ".")
	}
	ksVector := KeySet{Keys: []TrustedKey{{KID: "k1", PublicKey: vectorPub}, {KID: "k2", PublicKey: otherPub}}}

	cases := []struct {
		name string
		key  string
		keys KeySet
		at   time.Time
		want error // nil: any error
	}{
		{"unknown kid", testIssue(t, sk, "k9", good), ks, vectorNow, ErrUnknownKey},
		{"kid segment changed", strings.Replace(vectorKey, "FP2.k1.", "FP2.k2.", 1), ksVector, vectorNow, nil},
		{"kid field differs from the segment", testIssue(t, sk, "k1", func() Claims { c := good; c.KID = "k2"; return c }()), ks, vectorNow, nil},
		{"v is not 2", withClaims(func(c *Claims) { c.V = 1 }), ks, vectorNow, nil},
		{"seats 0", withClaims(func(c *Claims) { c.Seats = 0 }), ks, vectorNow, nil},
		{"no plans", withClaims(func(c *Claims) { c.Plans = nil }), ks, vectorNow, nil},
		{"exp not after iat", withClaims(func(c *Claims) { c.Expires = c.IssuedAt }), ks, vectorNow, nil},
		{"revoked kid", vectorKey, KeySet{Keys: vectorKeys().Keys, Revoked: []string{"k1"}}, vectorNow, ErrRevoked},
		{"payload byte changed", flip(2), vectorKeys(), vectorNow, nil},
		{"signature byte changed", flip(3), vectorKeys(), vectorNow, nil},
		{"signed by a different key", testIssue(t, otherSK, "k1", good), ks, vectorNow, nil},
		{"FP1 key", "FP1." + strings.Join(parts[2:], "."), vectorKeys(), vectorNow, ErrOldFormat},
		{"expired", vectorKey, vectorKeys(), time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC), ErrExpired},
		{"three segments", strings.Join(append([]string{"FP2"}, parts[2:]...), "."), vectorKeys(), vectorNow, nil},
		{"five segments", vectorKey + ".x", vectorKeys(), vectorNow, nil},
		{"bad kid charset", strings.Replace(vectorKey, "FP2.k1.", "FP2.K1.", 1), vectorKeys(), vectorNow, nil},
		{"no keys at all", vectorKey, KeySet{}, vectorNow, ErrNoPublicKey},
		{"non-canonical signature spelling", nonCanonical(vectorKey), vectorKeys(), vectorNow, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := VerifyWith(tc.key, tc.keys, tc.at)
			if err == nil {
				t.Fatalf("expected an error, got claims %+v", c)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want != ErrExpired && c != nil {
				t.Fatalf("a rejected key must not return claims: %+v", c)
			}
		})
	}
}

func TestExpiredKeyStillReturnsItsClaims(t *testing.T) {
	c, err := VerifyWith(vectorKey, vectorKeys(), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrExpired) || c == nil || c.Customer != "cus_TESTVECTOR" {
		t.Fatalf("c=%+v err=%v", c, err)
	}
}

func TestReleaseKeySetEmbedsNoTestKey(t *testing.T) {
	for _, k := range trustedKeys {
		if k.PublicKey == vectorPub {
			t.Fatal("the test vector key must never be in the embedded key set")
		}
	}
}

func TestVerifyUsesEnvKeys(t *testing.T) {
	sk, pub := testKey(testSeed())
	t.Setenv(KeyEnv, "k1="+pub+", k2=bogus")
	key := testIssue(t, sk, "k1", Claims{V: 2, KID: "k1", ID: "x", Customer: "c", Plans: []Plan{Plan("hosted")}, Seats: 1,
		IssuedAt: time.Now().Add(-time.Hour), Expires: time.Now().Add(time.Hour)})
	c, err := Verify(key)
	if err != nil || !c.Has(Plan("hosted")) {
		t.Fatalf("c=%+v err=%v", c, err)
	}
	t.Setenv(KeyEnv, "")
	if _, err := Verify(key); !errors.Is(err, ErrNoPublicKey) {
		t.Fatalf("without any key: %v", err)
	}
}

// nonCanonical re-spells the signature with non-zero trailing bits in its last
// character: a lax decoder accepts it as the same bytes.
func nonCanonical(key string) string {
	parts := strings.Split(key, ".")
	sig := parts[3]
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	idx := strings.IndexByte(alphabet, sig[len(sig)-1])
	parts[3] = sig[:len(sig)-1] + string(alphabet[idx^0x01])
	return strings.Join(parts, ".")
}
