package hosted

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultTimeout bounds one upload; there are no retries, so a slow or dead endpoint costs a few seconds at most.
const DefaultTimeout = 5 * time.Second

// HTTPTransport posts the payload to <BaseURL>/v1/reports with the license key as a bearer token.
// The contract is a preview and may change.
type HTTPTransport struct {
	BaseURL string
	Client  *http.Client
}

// ValidateBaseURL refuses a URL that would send the key in clear text: https is required except for
// a loopback host (tests, local development).
func ValidateBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("hosted.url %q is not a valid URL", raw)
	}
	if u.Scheme == "https" {
		return u, nil
	}
	if u.Scheme == "http" {
		host, _, herr := net.SplitHostPort(u.Host)
		if herr != nil {
			host = u.Host
		}
		if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
			return u, nil
		}
	}
	return nil, fmt.Errorf("hosted.url %q must use https (plain http is allowed only for localhost)", raw)
}

// Send implements Transport.
func (t HTTPTransport) Send(ctx context.Context, licenseKey string, body []byte) error {
	u, err := ValidateBaseURL(t.BaseURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(u.String(), "/")+"/v1/reports", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+licenseKey)
	resp, err := noRedirects(t.Client).Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %s", Sanitize(err.Error(), licenseKey))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 3 {
		return fmt.Errorf("the hosted service answered with a redirect (HTTP %d); redirects are never followed so the license key cannot be forwarded", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		snippet := []rune(Sanitize(string(raw), licenseKey))
		if len(snippet) > 200 {
			snippet = append(snippet[:200], '…')
		}
		return fmt.Errorf("the hosted service answered HTTP %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}

// noRedirects returns a copy of c (or a new client) that never follows a redirect: Go would forward the
// Authorization header to the redirect target on the same host, for example from https to http.
func noRedirects(c *http.Client) *http.Client {
	var out http.Client
	if c != nil {
		out = *c
	}
	out.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &out
}

var (
	licenseLike = regexp.MustCompile(`FP2\.[A-Za-z0-9._-]*`)
	ansiCSI     = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	ansiOSC     = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)?")
)

// Sanitize makes text from a remote server safe to print in a terminal: ANSI escape sequences, control
// characters and invalid UTF-8 are removed (newlines become spaces), the license key is redacted if
// the server echoes it, and the result is trimmed.
func Sanitize(s, licenseKey string) string {
	if licenseKey != "" {
		s = strings.ReplaceAll(s, licenseKey, "[redacted]")
	}
	s = licenseLike.ReplaceAllString(s, "[redacted]") // a key, whole or cut off, in any case
	s = strings.ToValidUTF8(s, "")
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiCSI.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Options is one upload.
type Options struct {
	Report     []byte // the run's JSON report, unchanged
	Envelope   Envelope
	LicenseKey string // empty with DryRun is fine
	BaseURL    string // default DefaultBaseURL
	Timeout    time.Duration
	DryRun     bool
	Out        io.Writer // dry-run output and the notes
	Transport  Transport // default HTTPTransport{BaseURL}
	Scrubber   *Scrubber // default NewScrubber()
}

// BuildPayload scrubs the report and wraps it with the envelope, within MaxPayloadBytes.
func BuildPayload(o Options) ([]byte, Stats, error) {
	sc := o.Scrubber
	if sc == nil {
		sc = NewScrubber()
	}
	report, st, err := sc.Scrub(o.Report)
	if err != nil {
		return nil, st, err
	}
	encode := func(rep []byte) ([]byte, error) {
		return json.MarshalIndent(Payload{Schema: SchemaVersion, Envelope: o.Envelope, Report: rep}, "", "  ")
	}
	body, err := encode(report)
	if err != nil {
		return nil, st, err
	}
	if len(body) > MaxPayloadBytes {
		if short, serr := ShortenErrors(report, 300); serr == nil {
			if b2, e2 := encode(short); e2 == nil {
				body = b2
			}
		}
	}
	if len(body) > MaxPayloadBytes {
		return nil, st, fmt.Errorf("the report is %d KB, over the %d KB upload limit: nothing was sent", len(body)/1024, MaxPayloadBytes/1024)
	}
	return body, st, nil
}

// Upload builds the payload and either prints it (DryRun) or sends it once with a short timeout.
// The returned error is meant to be shown as a warning; it never has to fail a run.
func Upload(ctx context.Context, o Options) error {
	body, st, err := BuildPayload(o)
	if err != nil {
		return err
	}
	if o.Out != nil {
		fmt.Fprintf(o.Out, "report scrubbed: %d field(s) dropped, %d string(s) redacted or made relative, %d shortened\n", st.DroppedFields, st.Redactions+st.PathsMadeRel, st.Truncated)
	}
	if o.DryRun {
		if o.Out != nil {
			fmt.Fprintf(o.Out, "dry run: nothing is sent. The request would be POST %s/v1/reports with the license key as a bearer token and this body:\n%s\n", baseOrDefault(o.BaseURL), body)
		}
		return nil
	}
	if o.LicenseKey == "" {
		return fmt.Errorf("no license key")
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tr := o.Transport
	if tr == nil {
		tr = HTTPTransport{BaseURL: baseOrDefault(o.BaseURL), Client: noRedirects(&http.Client{Timeout: timeout})}
	}
	return tr.Send(cctx, o.LicenseKey, body)
}

func baseOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(s, "/")
}
