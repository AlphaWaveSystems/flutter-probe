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
	"net"
	"net/url"
	"os"
	"time"
)

// sign returns the public key (as PROBE_LICENSE_PUBKEY wants it) and a license that verifies with it, valid from now for a day.
func sign(now time.Time) (pubEnv, key string) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	b := base64.RawURLEncoding
	now = now.UTC().Truncate(time.Second)
	payload, _ := json.Marshal(map[string]any{
		"v": 2, "kid": "localtest", "id": "lic-local-test", "customer": "LOCAL-TEST-ONLY-NOT-A-REAL-LICENSE",
		"plans": []string{"probe_cloud"}, "seats": 1, "iat": now, "exp": now.Add(24 * time.Hour),
	})
	signed := "FP2.localtest." + b.EncodeToString(payload)
	return "localtest=" + b.EncodeToString(pub), signed + "." + b.EncodeToString(ed25519.Sign(priv, []byte(signed)))
}

// checkLoopback refuses a service address that is not on this machine. The license made here is only meant for the fake service of this folder; if
// PROBE_CLOUD_URL already points somewhere else, the tool prints nothing, so a test license is never set up next to a real endpoint.
func checkLoopback(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("PROBE_CLOUD_URL is not a valid URL")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("PROBE_CLOUD_URL names %q, which is not this machine: refusing to make a test license (unset it, or point it at http://127.0.0.1:48899)", host)
}

func main() {
	if err := checkLoopback(os.Getenv("PROBE_CLOUD_URL")); err != nil {
		fmt.Fprintln(os.Stderr, "sign-test-license:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "TEST ONLY: a throwaway key pair and a one-day license for the fake service in scripts/fake-cloud. It is not a real license and no real service accepts it.")
	pub, key := sign(time.Now())
	fmt.Printf("export PROBE_LICENSE_PUBKEY=%s\nexport PROBE_LICENSE_KEY=%s\n", pub, key)
}
