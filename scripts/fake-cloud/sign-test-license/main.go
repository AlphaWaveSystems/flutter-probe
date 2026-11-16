// Command sign-test-license makes a throwaway Ed25519 key pair and signs a one-day license with it, for testing an upload against a fake server
// (scripts/fake-cloud/README.md). It prints two lines to export:
//
//	PROBE_LICENSE_PUBKEY=localtest=<public key>
//	PROBE_LICENSE_KEY=<license>
//
// The key pair exists only for this run and the private half is never printed or stored. A released probe trusts none of this: the public key
// is accepted only because PROBE_LICENSE_PUBKEY names it, and a real service signs with its own keys.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// sign returns the public key (as PROBE_LICENSE_PUBKEY wants it) and a license that verifies with it, valid from now for a day.
func sign(now time.Time) (pubEnv, key string) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	b := base64.RawURLEncoding
	now = now.UTC().Truncate(time.Second)
	payload, _ := json.Marshal(map[string]any{
		"v": 2, "kid": "localtest", "id": "lic-local-test", "customer": "local-test",
		"plans": []string{"probe_cloud"}, "seats": 1, "iat": now, "exp": now.Add(24 * time.Hour),
	})
	signed := "FP2.localtest." + b.EncodeToString(payload)
	return "localtest=" + b.EncodeToString(pub), signed + "." + b.EncodeToString(ed25519.Sign(priv, []byte(signed)))
}

func main() {
	pub, key := sign(time.Now())
	fmt.Printf("export PROBE_LICENSE_PUBKEY=%s\nexport PROBE_LICENSE_KEY=%s\n", pub, key)
}
