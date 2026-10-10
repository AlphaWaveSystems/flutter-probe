// Package license verifies offline-verifiable FlutterProbe license
// keys. A key is `FP1.<payload>.<signature>` where payload is base64url JSON and
// the signature is an ed25519 signature over the raw payload bytes.
//
// Keys are issued by the private ops tooling; this package only verifies.
// The free product never needs a key: a missing or invalid key only means the
// hosted entitlements are off.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Prefix identifies the key format version.
const Prefix = "FP1"

// PublicKeyEnv overrides the embedded public key (development and tests).
const PublicKeyEnv = "PROBE_LICENSE_PUBKEY"

// publicKeyB64 is the production verification key. It is filled in by the owner
// at setup (docs/ops/OWNER-SETUP.md); until then only PROBE_LICENSE_PUBKEY works.
var publicKeyB64 = ""

// Plan names an entitlement bundle.
type Plan string

const (
	PlanTeam Plan = "team"
)

// Claims is the signed payload.
type Claims struct {
	ID       string    `json:"id"`
	Customer string    `json:"customer"`
	Plans    []Plan    `json:"plans"`
	Seats    int       `json:"seats"`
	IssuedAt time.Time `json:"iat"`
	Expires  time.Time `json:"exp"`
}

// Has reports whether the claims include a plan.
func (c Claims) Has(p Plan) bool {
	for _, x := range c.Plans {
		if x == p {
			return true
		}
	}
	return false
}

// Strict rejects non-canonical encodings (non-zero trailing bits), so a key has
// exactly one valid spelling and cannot be "tampered" into an alias of itself.
var b64 = base64.RawURLEncoding.Strict()

// ErrNoPublicKey means no verification key is configured.
var ErrNoPublicKey = errors.New("license: no verification key configured")

// ErrExpired means the key was valid but is past its expiry.
var ErrExpired = errors.New("license: expired")

// Verify checks a key against the configured public key and returns its claims.
func Verify(key string) (*Claims, error) {
	return VerifyWith(key, publicKey(), time.Now())
}

// VerifyWith verifies against an explicit base64url public key at a given time.
func VerifyWith(key, pubB64 string, now time.Time) (*Claims, error) {
	if pubB64 == "" {
		return nil, ErrNoPublicKey
	}
	pk, err := b64.DecodeString(pubB64)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return nil, errors.New("license: invalid public key")
	}
	parts := strings.Split(strings.TrimSpace(key), ".")
	if len(parts) != 3 || parts[0] != Prefix {
		return nil, errors.New("license: malformed key")
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("license: malformed payload")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("license: malformed signature")
	}
	if !ed25519.Verify(ed25519.PublicKey(pk), payload, sig) {
		return nil, errors.New("license: signature does not verify")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("license: payload: %w", err)
	}
	if !c.Expires.IsZero() && now.After(c.Expires) {
		return &c, ErrExpired
	}
	return &c, nil
}

func publicKey() string {
	if v := os.Getenv(PublicKeyEnv); v != "" {
		return v
	}
	return publicKeyB64
}
