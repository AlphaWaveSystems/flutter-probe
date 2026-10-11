// Package license verifies offline-verifiable FlutterProbe license keys.
//
// A key is `FP2.<kid>.<payload>.<signature>`:
//
//   - kid: the id of the signing key, [a-z0-9-]{1,16} (for example k1)
//   - payload: base64url (no padding) of the JSON claims
//   - signature: base64url of the Ed25519 signature over the exact ASCII bytes
//     "FP2." + kid + "." + payload, as received. Nothing is re-encoded before
//     verifying, and the format tag and the kid are covered by the signature.
//
// Claims: v (must be 2), kid (must equal the kid segment), id, customer, plans
// (non-empty), seats (at least 1; 0 is rejected), iat and exp (RFC 3339, exp after iat).
//
// The CLI embeds a trusted key set and a list of revoked key ids; keys are
// issued elsewhere and this package only verifies. The free product never needs
// a key: a missing or invalid key only means the hosted entitlements are off.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Prefix identifies the key format.
const Prefix = "FP2"

// KeyEnv adds trusted keys for development and tests: a comma-separated list of
// kid=publicKey pairs (public keys are base64url), for example
// "k1=A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg".
const KeyEnv = "PROBE_LICENSE_PUBKEY"

// TrustedKey is a public key keys may be signed with.
type TrustedKey struct {
	KID       string
	PublicKey string // base64url, no padding
}

// trustedKeys are the production verification keys. The set is filled in with
// the live public keys of the issuer; until then only KeyEnv adds keys.
var trustedKeys = []TrustedKey{}

// revokedKIDs lists key ids whose keys must no longer be accepted.
var revokedKIDs = []string{}

// KeySet is the trusted keys and the revoked key ids a verification uses.
type KeySet struct {
	Keys    []TrustedKey
	Revoked []string
}

// Plan names an entitlement bundle.
type Plan string

const (
	PlanTeam Plan = "team"
)

// Claims is the signed payload.
type Claims struct {
	V        int       `json:"v"`
	KID      string    `json:"kid"`
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

// b64 rejects non-canonical encodings (non-zero trailing bits), so a key has
// exactly one valid spelling and cannot be re-spelled into an alias of itself.
var b64 = base64.RawURLEncoding.Strict()

var kidPattern = regexp.MustCompile(`^[a-z0-9-]{1,16}$`)

var (
	// ErrNoPublicKey means no verification key is configured.
	ErrNoPublicKey = errors.New("license: no verification key configured")
	// ErrExpired means the key was valid but is past its expiry.
	ErrExpired = errors.New("license: expired")
	// ErrOldFormat means the key uses the retired FP1 format.
	ErrOldFormat = errors.New("license: this key is in an old format; request a new key")
	// ErrUnknownKey means the key was signed with a key id this CLI does not know.
	ErrUnknownKey = errors.New("license: signed with an unknown key id; update the CLI")
	// ErrRevoked means the key id was revoked.
	ErrRevoked = errors.New("license: the signing key was revoked; request a new key")
)

// Verify checks a key against the CLI's trusted keys (plus KeyEnv) and returns its claims.
func Verify(key string) (*Claims, error) {
	return VerifyWith(key, currentKeySet(), time.Now())
}

// VerifyWith verifies against an explicit key set at a given time. Checks run in
// this order and stop at the first failure: shape, key lookup, signature,
// claims, expiry. A key that is valid but expired returns its claims with ErrExpired.
func VerifyWith(key string, ks KeySet, now time.Time) (*Claims, error) {
	key = strings.TrimSpace(key)
	parts := strings.Split(key, ".")
	if parts[0] == "FP1" {
		return nil, ErrOldFormat
	}
	if len(parts) != 4 || parts[0] != Prefix || !kidPattern.MatchString(parts[1]) {
		return nil, errors.New("license: malformed key")
	}
	kid, payloadSeg, sigSeg := parts[1], parts[2], parts[3]

	for _, r := range ks.Revoked {
		if r == kid {
			return nil, ErrRevoked
		}
	}
	if len(ks.Keys) == 0 {
		return nil, ErrNoPublicKey
	}
	var pub string
	for _, k := range ks.Keys {
		if k.KID == kid {
			pub = k.PublicKey
			break
		}
	}
	if pub == "" {
		return nil, ErrUnknownKey
	}
	pk, err := b64.DecodeString(pub)
	if err != nil || len(pk) != ed25519.PublicKeySize {
		return nil, errors.New("license: invalid public key")
	}
	sig, err := b64.DecodeString(sigSeg)
	if err != nil {
		return nil, errors.New("license: malformed signature")
	}
	signed := Prefix + "." + kid + "." + payloadSeg
	if !ed25519.Verify(ed25519.PublicKey(pk), []byte(signed), sig) {
		return nil, errors.New("license: signature does not verify")
	}

	payload, err := b64.DecodeString(payloadSeg)
	if err != nil {
		return nil, errors.New("license: malformed payload")
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("license: payload: %w", err)
	}
	switch {
	case c.V != 2:
		return nil, fmt.Errorf("license: unsupported claims version %d", c.V)
	case c.KID != kid:
		return nil, errors.New("license: key id in the payload does not match the key")
	case c.Seats < 1:
		return nil, errors.New("license: seats must be at least 1")
	case len(c.Plans) == 0:
		return nil, errors.New("license: no plans")
	case c.Expires.IsZero() || c.IssuedAt.IsZero() || !c.Expires.After(c.IssuedAt):
		return nil, errors.New("license: expiry must be after the issue time")
	}
	if now.After(c.Expires) {
		return &c, ErrExpired
	}
	return &c, nil
}

func currentKeySet() KeySet {
	ks := KeySet{Keys: append([]TrustedKey(nil), trustedKeys...), Revoked: append([]string(nil), revokedKIDs...)}
	for _, pair := range strings.Split(os.Getenv(KeyEnv), ",") {
		kid, pub, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && kid != "" && pub != "" {
			ks.Keys = append(ks.Keys, TrustedKey{KID: kid, PublicKey: pub})
		}
	}
	return ks
}
