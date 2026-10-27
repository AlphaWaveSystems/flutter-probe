// Package record writes recording bundles for test runs.
package record

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SchemaVersion is the version of the bundle's JSON files. RingSize lives in cursor.go. Version 2 changes capture.redaction
// from a boolean to the redacted and not_redacted lists.
const SchemaVersion = 2

type Manifest struct {
	SchemaVersion int        `json:"schema_version"`
	RunID         string     `json:"run_id"`
	TestName      string     `json:"test_name"`
	File          string     `json:"file"`
	FileSHA256    string     `json:"file_sha256"`
	Attempt       int        `json:"attempt"`
	DeviceName    string     `json:"device_name"`
	Platform      string     `json:"platform"`
	OSVersion     string     `json:"os_version"`
	AgentVersion  string     `json:"agent_version"`
	CLIVersion    string     `json:"cli_version"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       time.Time  `json:"ended_at"`
	Status        string     `json:"status"`
	Capture       Capture    `json:"capture"`
	Truncated     Truncation `json:"truncated"`
}

type Capture struct {
	Screenshots string `json:"screenshots"`
	Native      string `json:"native"`
	// Redacted lists what the recording removed or masked, and NotRedacted what it keeps as the app and the test produced
	// it. A reader sharing a recording needs both: screenshots and step descriptions are not redacted.
	Redacted    []string `json:"redacted"`
	NotRedacted []string `json:"not_redacted"`
}

// redactedKinds lists what a recording removes or masks, for the manifest. Each entry says what its rule covers, and
// notRedactedKinds says what the rules do not reach.
func redactedKinds() []string {
	return []string{
		"http headers: the values of headers whose name holds a credential word by the token rule, such as X-Api-Token, Private-Token or X-Amz-Signature, and of the fixed names (authorization, cookie, api-key and the other fixed names)",
		"http urls: the password in a userinfo, absolute or scheme-relative, and the values of query, fragment and path parameters whose names are credentials, such as X-Amz-Signature and X-Goog-Signature",
		"http bodies: the values of JSON members, XML elements and multipart parts whose names are credentials (a multipart name quoted or not, and percent-decoded), of any value type; the value member (value, val, data, content, text or newValue, in any case) of a JSON object whose name, key, field, id or label member names a credential, where a name member may be a string or an array of strings; a name that holds a credential word is masked whatever else it says, counters such as Token-Count included; name=value assignments in form and plain text whose name is a credential and whose value has four or more characters; the whole value of authorization, proxy-authorization, cookie and set-cookie lines in text; and credential-shaped strings (API keys, bearer tokens, JWTs, private keys, long hex and base64 strings)",
		"errors and warnings: the body rules applied to plain text (credential-shaped strings, URL userinfo passwords and name=value assignments), and resolved environment values",
		"selectors and widget trees: resolved environment values only",
	}
}

// notRedactedKinds lists what a recording keeps as the app and the test produced it, and what the redaction rules do not
// reach, for the manifest.
func notRedactedKinds() []string {
	return []string{
		"screenshots: pixels as rendered, so an on-screen value can appear",
		"step descriptions: the test's own text, including literal text it types",
		"free text with no assignment, such as a sentence that says a password is a value: it is not redacted",
		"names that use lookalike letters, such as a Cyrillic letter in place of a Latin one: they are not matched as credentials",
		"names outside the credential token, pair and compound lists, such as a bare code, card name, newpassword, secretkey or authtoken, or a plural such as cookies",
		"base64 and hex credentials below the shape thresholds (40 base64 characters, 32 hex characters), and credentials in any other encoding",
		"names and separators that are percent-encoded twice",
		"form and plain-text values of under four characters, including in a JSON body that does not parse, such as a truncated one",
		"environment values of under three characters, and of under eight characters when the variable's name has no secret word as a whole token (PASS, PASSWORD, PASSWD, PWD, PW, SECRET, TOKEN, OTP, CRED, PIN, KEY and the other listed words)",
		"HTML attributes, such as the value of an input, and other formats that are not JSON, form, XML, multipart or plain text",
		"selectors and widget trees: values the run did not resolve from the environment, such as a literal typed into a step",
		"http bodies in other formats, such as HTML or XML, where a value is an attribute, as in an input's value",
		"http bodies: values whose names are outside the credential token, pair and compound lists, such as a bare code or key",
		"http urls: parameter names outside the credential query, token, pair and compound lists, such as a bare code",
		"http bodies: a name/value pair whose name member is not name, key, field, id or label, or whose value member is not value, val, data, content, text or newValue, such as item and value or name and amount, and a form pair whose value is in a separate field, such as name=password&value=X",
		"http urls: a userinfo password with an unencoded @, /, ?, # or whitespace, which keeps the whole password or its tail",
		"http urls: a parameter whose value holds its own ?, such as next=/cb?token=X, so the nested parameter is kept",
		"http bodies: a URL in a JSON string written with escaped slashes (\\/), whose userinfo password is kept",
		"http bodies: a form or plain-text value that holds a space, quote, comma, semicolon, ampersand or angle bracket: only the part before it is masked",
		"http bodies: a JSON member name, which is kept as written even when it holds a credential-shaped pair",
		"http bodies: XML text that is CDATA, and a credential element with child elements, which is masked only through its children's own names",
		"http bodies: an authorization, cookie or set-cookie header that does not start its line, such as one in a log line",
	}
}

type Truncation struct {
	Exchanges int   `json:"exchanges"`
	Bytes     int64 `json:"bytes"`
}

type Rect struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Interaction struct {
	Kind     string  `json:"kind"`
	Selector string  `json:"selector,omitempty"`
	Target   *Rect   `json:"target,omitempty"`
	TapX     float64 `json:"tap_x,omitempty"`
	TapY     float64 `json:"tap_y,omitempty"`
}

type Step struct {
	Index            int          `json:"index"`
	Depth            int          `json:"depth"`
	File             string       `json:"file"`
	Line             int          `json:"line"`
	Description      string       `json:"description"`
	Status           string       `json:"status"`
	Error            string       `json:"error,omitempty"`
	StartedAt        time.Time    `json:"started_at"`
	DurationMs       int64        `json:"duration_ms"`
	ScreenshotBefore string       `json:"screenshot_before,omitempty"`
	ScreenshotAfter  string       `json:"screenshot_after,omitempty"`
	Tree             string       `json:"tree,omitempty"`
	Interaction      *Interaction `json:"interaction,omitempty"`
	Exchanges        []string     `json:"exchanges,omitempty"`
	AppRestarted     bool         `json:"app_restarted,omitempty"`
}

type Timeline struct {
	Steps []Step `json:"steps"`
}

type Exchange struct {
	ID              string            `json:"id"`
	Seq             int               `json:"seq"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Status          int               `json:"status"`
	DurationMs      int               `json:"duration_ms"`
	Mocked          bool              `json:"mocked"`
	Error           string            `json:"error,omitempty"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	RequestBody     string            `json:"request_body"`
	ResponseBody    string            `json:"response_body"`
	Source          string            `json:"source"`
	Duplicate       bool              `json:"duplicate,omitempty"`
	TLSFailed       bool              `json:"tls_failed,omitempty"`
}

// Bundle is a directory containing a recording and its associated artifacts.
type Bundle struct{ dir string }

// Create creates a private bundle directory and its artifact subdirectories.
func Create(dir string) (*Bundle, error) {
	for _, path := range []string{dir, filepath.Join(dir, "http"), filepath.Join(dir, "tree"), filepath.Join(dir, "screenshots")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, 0700); err != nil {
			return nil, err
		}
	}
	return &Bundle{dir: dir}, nil
}

func (b *Bundle) Dir() string                    { return b.dir }
func (b *Bundle) WriteManifest(m Manifest) error { return b.writeJSON("manifest.json", m) }
func (b *Bundle) WriteTimeline(t Timeline) error { return b.writeJSON("timeline.json", t) }

var bundleID = regexp.MustCompile(`^[0-9]{4,}$`)
var screenshotName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (b *Bundle) WriteExchange(e Exchange) error {
	if !bundleID.MatchString(e.ID) {
		return fmt.Errorf("invalid exchange ID %q", e.ID)
	}
	return b.writeJSON(filepath.Join("http", e.ID+".json"), e)
}

func (b *Bundle) WriteTree(id, text string) error {
	if !bundleID.MatchString(id) {
		return fmt.Errorf("invalid tree ID %q", id)
	}
	return b.writeFile(filepath.Join("tree", id+".txt"), []byte(text))
}

func (b *Bundle) WriteScreenshot(name string, png []byte) error {
	if !screenshotName.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid screenshot name %q", name)
	}
	return b.writeFile(filepath.Join("screenshots", name), png)
}

func (b *Bundle) writeJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return b.writeFile(name, data)
}

// writeFile only receives fixed or validated relative names. Renaming a closed
// temporary file in the destination directory publishes the complete contents.
func (b *Bundle) writeFile(name string, data []byte) error {
	path := filepath.Join(b.dir, name)
	f, err := os.CreateTemp(filepath.Dir(path), ".record-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
