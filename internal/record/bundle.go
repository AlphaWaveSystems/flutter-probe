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

// SchemaVersion is the version of the bundle's JSON files. RingSize lives in cursor.go.
const SchemaVersion = 1

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
	Redaction   bool   `json:"redaction"`
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
	Kind   string  `json:"kind"`
	Target *Rect   `json:"target,omitempty"`
	TapX   float64 `json:"tap_x,omitempty"`
	TapY   float64 `json:"tap_y,omitempty"`
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
