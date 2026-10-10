package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
)

// testKeypair and testIssue mirror the private issuing tool so the verifier
// can be tested without shipping signing code in the public CLI.
func testKeypair(t *testing.T) (pub, priv string) {
	t.Helper()
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return b64.EncodeToString(pk), b64.EncodeToString(sk)
}

func testIssue(t *testing.T, priv string, c Claims) string {
	t.Helper()
	sk, _ := b64.DecodeString(priv)
	payload, _ := json.Marshal(c)
	sig := ed25519.Sign(ed25519.PrivateKey(sk), payload)
	return strings.Join([]string{Prefix, b64.EncodeToString(payload), b64.EncodeToString(sig)}, ".")
}
