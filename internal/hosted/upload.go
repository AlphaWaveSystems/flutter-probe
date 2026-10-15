package hosted

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds one upload attempt, so a slow or dead endpoint costs a few seconds per try.
	DefaultTimeout = 5 * time.Second
	// DefaultRetryWindow is how long a busy service (HTTP 429 or 503) is retried before the upload gives up.
	// Giving up is a warning, never a failed run.
	DefaultRetryWindow = 5 * time.Minute

	retryFirstDelay = 2 * time.Second
	retryMaxDelay   = 30 * time.Second
	retryAfterCap   = 60 * time.Second
)

// HTTPTransport posts the payload to <BaseURL>/v1/reports with the license key as a bearer token and,
// when Attestation is set, the identity token in the X-Probe-Attestation header. The token is sent only to
// BaseURL and a redirect is never followed. The contract is a preview and may change.
type HTTPTransport struct {
	BaseURL     string
	Client      *http.Client
	Attestation string
	// Note, when set, receives a warning about a stored report (for example a rejected identity token).
	Note func(string)
	// Reply, when set, receives the body of a successful answer (a local upload's receipt).
	Reply func([]byte)
}

// StatusError is a non-2xx answer from the hosted service. Upload retries the busy ones (429, 503).
type StatusError struct {
	Code       int
	Message    string
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("the hosted service answered HTTP %d: %s", e.Code, e.Message)
}

// Retryable reports whether the answer means "try again later".
func (e *StatusError) Retryable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code == http.StatusServiceUnavailable
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
	if t.Attestation != "" && validAttestation(t.Attestation) {
		req.Header.Set(AttestationHeader, t.Attestation)
	}
	resp, err := noRedirects(t.Client).Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %s", Sanitize(err.Error(), licenseKey, t.Attestation))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 3 {
		return fmt.Errorf("the hosted service answered with a redirect (HTTP %d); redirects are never followed so the license key and identity token cannot be forwarded", resp.StatusCode)
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode/100 != 2 {
		snippet := []rune(Sanitize(string(raw), licenseKey, t.Attestation))
		if len(snippet) > 200 {
			snippet = append(snippet[:200], '…')
		}
		return &StatusError{Code: resp.StatusCode, Message: string(snippet), RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if t.Reply != nil {
		t.Reply(raw)
	}
	if note := attestationNote(raw); note != "" && t.Note != nil {
		t.Note(note)
	}
	return nil
}

// parseRetryAfter reads a Retry-After header in seconds; anything else is 0 (use the backoff).
func parseRetryAfter(v string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
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
	tokenLike   = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]*){0,2}`)
	ansiCSI     = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")
	ansiOSC     = regexp.MustCompile("\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)?")
)

// Sanitize makes text from a remote server safe to print in a terminal: ANSI escape sequences, control
// characters and invalid UTF-8 are removed (newlines become spaces), the license key is redacted if
// the server echoes it, and the result is trimmed.
func Sanitize(s string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	s = tokenLike.ReplaceAllString(s, "[redacted]")   // an identity token (JWT), whole or cut off
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

	// Attestation is the job's identity token, sent as a header to BaseURL only. It is never printed.
	// A local upload never sends it.
	Attestation string
	// DryAttestation makes a dry run say "attestation: present" without a token being requested.
	DryAttestation bool
	// RetryWindow is how long a busy service (429, 503) is retried; 0 uses DefaultRetryWindow, negative disables retries.
	RetryWindow time.Duration
	// Warn receives progress and warnings that are not the final error (a retry, a rejected token).
	Warn func(string)
	// Sleep waits between retries; tests replace it. Default: a timer that honours ctx.
	Sleep func(context.Context, time.Duration) error
	// Now is the clock the retry window is measured on; tests replace it. Default time.Now.
	Now func() time.Time
	// Receipt, when set, receives the server's answer to a local upload: the run key, and whether the server already had
	// the same upload.
	Receipt *Receipt
}

// Receipt is the server's answer to an accepted local upload.
type Receipt struct {
	RunKey    string `json:"run_key"`
	Duplicate bool   `json:"duplicate"`
	Results   int    `json:"results"`
	Stored    bool   `json:"stored"`
	Source    string `json:"source"`
}

// BuildPayload scrubs the report and wraps it with the envelope, within MaxPayloadBytes.
func BuildPayload(o Options) ([]byte, Stats, error) {
	return buildPayload(o, MaxPayloadBytes)
}

// buildPayload is BuildPayload with the body limit made explicit: a local upload has a smaller one.
func buildPayload(o Options, limit int) ([]byte, Stats, error) {
	sc := o.Scrubber
	if sc == nil {
		sc = NewScrubber()
	}
	report, st, err := sc.Scrub(o.Report)
	if err != nil {
		return nil, st, err
	}
	// The envelope is scrubbed like the report: a retried test's file is a relative path, never a local one,
	// and its name goes through the same secret patterns.
	env := o.Envelope
	if len(env.Retries) > 0 {
		env.Retries = make([]RetryOutcome, len(o.Envelope.Retries))
		for i, r := range o.Envelope.Retries {
			r.File = sc.relativePath(sc.scrubString(r.File))
			r.Test = sc.scrubString(r.Test)
			env.Retries[i] = r
		}
	}
	encode := func(rep []byte) ([]byte, error) {
		return json.MarshalIndent(Payload{Schema: SchemaVersion, Envelope: env, Report: rep}, "", "  ")
	}
	body, err := encode(report)
	if err != nil {
		return nil, st, err
	}
	if len(body) > limit {
		if short, serr := ShortenErrors(report, 300); serr == nil {
			if b2, e2 := encode(short); e2 == nil {
				body = b2
			}
		}
	}
	if len(body) > limit {
		return nil, st, fmt.Errorf("the report is %d KB, over the %d KB upload limit: nothing was sent", len(body)/1024, limit/1024)
	}
	return body, st, nil
}

// Upload builds the payload and either prints it (DryRun) or sends it once with a short timeout, retrying a busy
// service within the retry window. A local upload (an envelope with a local block) is checked against the local rules,
// carries no CI facts, retry outcomes or identity token, and reports the server's answer through Options.Receipt.
// The returned error is meant to be shown as a warning; it never has to fail a run.
func Upload(ctx context.Context, o Options) error {
	local := o.Envelope.Local != nil
	var problems []string
	limit := MaxPayloadBytes
	if local {
		o.Envelope.CI, o.Envelope.Retries = nil, nil
		o.Attestation, o.DryAttestation = "", false
		problems = PreflightLocal(o.Envelope, o.Report)
		limit = MaxLocalPayloadBytes
	} else {
		problems = Preflight(o.Envelope, o.Report)
	}
	if len(problems) > 0 && !o.DryRun {
		return fmt.Errorf("nothing was uploaded: %s", strings.Join(problems, "; "))
	}
	body, st, err := buildPayload(o, limit)
	if err != nil {
		return err
	}
	if o.Out != nil {
		fmt.Fprintf(o.Out, "report scrubbed: %d field(s) dropped, %d string(s) redacted or made relative, %d shortened\n", st.DroppedFields, st.Redactions+st.PathsMadeRel, st.Truncated)
	}
	if o.DryRun {
		if o.Out != nil && len(problems) > 0 {
			fmt.Fprintf(o.Out, "note: a real upload would be refused before sending: %s\n", strings.Join(problems, "; "))
		}
		if o.Out != nil && !local {
			state := "absent"
			if o.Attestation != "" || o.DryAttestation {
				state = "present"
			}
			fmt.Fprintf(o.Out, "attestation: %s\n", state)
		}
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
	var reply []byte
	tr := o.Transport
	if tr == nil {
		h := HTTPTransport{BaseURL: baseOrDefault(o.BaseURL), Client: noRedirects(&http.Client{Timeout: timeout}), Attestation: o.Attestation, Note: o.Warn}
		if local {
			h.Reply = func(b []byte) { reply = b }
		}
		tr = h
	}
	window := o.RetryWindow
	if window == 0 {
		window = DefaultRetryWindow
	}
	sleep := o.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	if err := sendWithRetries(ctx, tr, o.LicenseKey, body, timeout, window, sleep, now, o.Warn); err != nil {
		if local {
			return localFailure(err)
		}
		return err
	}
	if local && o.Receipt != nil {
		var r Receipt
		if json.Unmarshal(reply, &r) == nil {
			*o.Receipt = r
		}
	}
	return nil
}

// sendWithRetries sends body once, and again while the service is busy (429, 503) and the retry window allows. The body
// is the same on every try: a local upload's sequence number is reserved once, before the first try.
func sendWithRetries(ctx context.Context, tr Transport, licenseKey string, body []byte, timeout, window time.Duration,
	sleep func(context.Context, time.Duration) error, now func() time.Time, warn func(string)) error {
	started := now()
	delay := retryFirstDelay
	for attempt := 1; ; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		err := tr.Send(cctx, licenseKey, body)
		cancel()
		var se *StatusError
		if err == nil || !errors.As(err, &se) || !se.Retryable() || window < 0 {
			return err
		}
		wait := delay
		if se.RetryAfter > 0 {
			wait = min(se.RetryAfter, retryAfterCap)
		}
		if now().Sub(started)+wait > window {
			return fmt.Errorf("%w (still busy after %s of retries; nothing was stored)", err, window.Round(time.Second))
		}
		if attempt == 1 || attempt%4 == 0 {
			warnf(warn, "the hosted service is busy (HTTP %d), retrying (attempt %d, waiting %s, giving up after %s)", se.Code, attempt+1, wait.Round(time.Second), window.Round(time.Second))
		}
		if serr := sleep(ctx, wait); serr != nil {
			return fmt.Errorf("%w (retries stopped: %v)", err, serr)
		}
		delay = min(delay*2, retryMaxDelay)
	}
}

// refusalCodes are the error codes the service answers local uploads with. Longer codes come first, so that bad_local
// is matched only when no more specific bad_local_* code is in the answer.
var refusalCodes = []string{
	"local_repo_not_allowed", "local_not_enabled", "no_entitlement", "too_many_results",
	"bad_local_repository", "bad_local_branch", "bad_local_commit", "bad_local_signer", "bad_local_seq", "bad_local_tree",
	"bad_local", "ci_and_local", "ci_required",
}

// refusalCode finds the first known error code in a service answer, or "" when there is none.
func refusalCode(message string) string {
	for _, c := range refusalCodes {
		if strings.Contains(message, c) {
			return c
		}
	}
	return ""
}

// localFailure says in plain words why the service refused a local upload. The reasons the contract names get a
// sentence; anything else keeps the service's own text.
func localFailure(err error) error {
	var se *StatusError
	if !errors.As(err, &se) {
		return err
	}
	switch code := refusalCode(se.Message); {
	case se.Code == http.StatusForbidden && code == "local_not_enabled":
		return errors.New("local uploads are not enabled for this team; ask the team owner")
	case se.Code == http.StatusForbidden && code == "local_repo_not_allowed":
		return errors.New("this repository is not registered or allowed for local uploads by your team; ask the team owner")
	case se.Code == http.StatusConflict:
		return errors.New("the service already has a different report under this sequence number; this is a client bug, so the upload is not retried")
	case se.Code == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("the report is over the local upload limit of %d KB; nothing was stored", MaxLocalPayloadBytes/1024)
	case code == "too_many_results":
		return fmt.Errorf("the report has more than %d results, the local upload limit; nothing was stored", MaxLocalResults)
	case strings.HasPrefix(code, "bad_local"):
		return fmt.Errorf("the service refused a local field (%s); this is a client bug, so the upload is not retried", code)
	}
	return err
}

// sleepCtx waits for d or until ctx is done.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func baseOrDefault(s string) string {
	if strings.TrimSpace(s) == "" {
		return DefaultBaseURL
	}
	return strings.TrimRight(s, "/")
}
