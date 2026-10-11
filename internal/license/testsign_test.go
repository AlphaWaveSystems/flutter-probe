package license

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
)

// testSeed is the throwaway seed 00 01 02 ... 1f of the shared test vector. Its key must
// never appear in a trusted key set of a release.
func testSeed() []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = byte(i)
	}
	return s
}

func testKey(seed []byte) (ed25519.PrivateKey, string) {
	sk := ed25519.NewKeyFromSeed(seed)
	return sk, b64.EncodeToString(sk.Public().(ed25519.PublicKey))
}

// testIssue signs claims the way the issuing side does: over "FP2." + kid + "." + payload.
func testIssue(t *testing.T, sk ed25519.PrivateKey, kid string, claims any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return testIssueRaw(sk, kid, payload)
}

func testIssueRaw(sk ed25519.PrivateKey, kid string, payload []byte) string {
	p := b64.EncodeToString(payload)
	sig := ed25519.Sign(sk, []byte(Prefix+"."+kid+"."+p))
	return strings.Join([]string{Prefix, kid, p, b64.EncodeToString(sig)}, ".")
}
