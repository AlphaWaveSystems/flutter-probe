package license

import (
	"errors"
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
		"wrong key":  {key, otherPub, now, nil},
		"tampered":   {key[:len(key)-2] + "AA", pub, now, nil},
		"malformed":  {"FP1.abc", pub, now, nil},
		"bad pub":    {key, "nope", now, nil},
		"expired":    {key, pub, now.Add(2 * time.Hour), ErrExpired},
		"no pub key": {key, "", now, ErrNoPublicKey},
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
