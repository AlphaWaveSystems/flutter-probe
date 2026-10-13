package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	// DefaultAudience is the audience requested for the GitHub Actions identity token unless
	// hosted.audience (or PROBE_CLOUD_AUDIENCE) names another one.
	DefaultAudience = "probe-cloud"
	// MaxAttestationLength is the longest token that is sent; a longer one is left out.
	MaxAttestationLength = 4096
	// AttestationHeader carries the token on the upload request.
	AttestationHeader = "X-Probe-Attestation"

	attestationTimeout = 5 * time.Second
	attestationMaxBody = 16 << 10
)

// AttestationAvailable reports whether the job can request a GitHub Actions identity token: both variables
// that GitHub sets for a job with `permissions: id-token: write` are present.
func AttestationAvailable(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return getenv("ACTIONS_ID_TOKEN_REQUEST_URL") != "" && getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN") != ""
}

// RequestAttestation asks GitHub for the job's identity token (OIDC) for the given audience, so that Probe
// Cloud can tell which workflow made an upload. It returns "" when no token can be had. Every failure is a
// warning: the upload goes on without the token. Neither the token nor the request credential is ever
// written to warn, an error or a log; it lives only in the returned string.
//
// The request goes only to ACTIONS_ID_TOKEN_REQUEST_URL (https, or loopback http in tests) and a redirect
// is never followed, so the request credential cannot be forwarded to another host.
func RequestAttestation(ctx context.Context, getenv func(string) string, client *http.Client, audience string, warn func(string)) string {
	if getenv == nil {
		return ""
	}
	reqURL, reqToken := getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" || reqToken == "" {
		if getenv("GITHUB_ACTIONS") == "true" {
			warnf(warn, "no identity token is available (the job needs `permissions: id-token: write`; pull requests from forks get none), so the upload goes without one")
		}
		return ""
	}
	if audience == "" {
		audience = DefaultAudience
	}
	u, err := ValidateBaseURL(reqURL)
	if err != nil {
		warnf(warn, "the identity token request URL is not usable, so the upload goes without a token")
		return ""
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()

	cctx, cancel := context.WithTimeout(ctx, attestationTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, u.String(), nil)
	if err != nil {
		warnf(warn, "the identity token request could not be built, so the upload goes without a token")
		return ""
	}
	req.Header.Set("Authorization", "bearer "+reqToken)
	req.Header.Set("Accept", "application/json")
	resp, err := noRedirects(client).Do(req)
	if err != nil {
		warnf(warn, "the identity token request failed, so the upload goes without a token: %s", Sanitize(err.Error(), reqToken))
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		warnf(warn, "the identity token request answered HTTP %d, so the upload goes without a token", resp.StatusCode)
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, attestationMaxBody))
	if err != nil {
		warnf(warn, "the identity token answer could not be read, so the upload goes without a token")
		return ""
	}
	var out struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &out) != nil || out.Value == "" {
		warnf(warn, "the identity token answer had no token, so the upload goes without one")
		return ""
	}
	if !validAttestation(out.Value) {
		warnf(warn, "the identity token is not usable (empty, longer than %d characters or not a plain token), so the upload goes without one", MaxAttestationLength)
		return ""
	}
	return out.Value
}

// validAttestation accepts a token of 1 to MaxAttestationLength characters with no whitespace or control
// characters, which is what a header value must be.
func validAttestation(t string) bool {
	if t == "" || len(t) > MaxAttestationLength {
		return false
	}
	for _, r := range t {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// attestationNote turns the fields of a 2xx answer into a warning for the run's output. Only the machine
// reason code is shown (and only if it is a short code), never the token. It returns "" when there is
// nothing to say.
func attestationNote(body []byte) string {
	var a struct {
		Attestation string `json:"attestation"`
		Reason      string `json:"attestation_reason"`
	}
	if json.Unmarshal(body, &a) != nil || a.Attestation != "rejected" {
		return ""
	}
	reason := a.Reason
	if len(reason) == 0 || len(reason) > 64 || strings.IndexFunc(reason, func(r rune) bool {
		return !(r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) >= 0 {
		return "the hosted service stored the report but did not accept its identity token"
	}
	return fmt.Sprintf("the hosted service stored the report but did not accept its identity token (reason: %s)", reason)
}
