// Package hosted is the opt-in client of the hosted report service (Probe Cloud, preview): it
// builds a payload from the run's JSON report plus a small envelope, scrubs it, and sends it.
// Nothing leaves the machine unless the user asked for it (--upload-report or hosted.upload_report),
// a license with the probe_cloud plan is present, and the scrubber has run. Failures are warnings:
// an upload never fails or noticeably slows a test run.
package hosted

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// DefaultBaseURL is a PLACEHOLDER: the hosted service is not live, so nothing answers there yet.
// Set hosted.url in probe.yaml to point at a real endpoint when one exists.
const DefaultBaseURL = "https://api.flutterprobe.dev"

// SchemaVersion is the version of the payload wrapper (the report inside keeps its own shape).
const SchemaVersion = 1

// Envelope is the small set of facts sent next to the report.
type Envelope struct {
	ProbeVersion string         `json:"probe_version"`
	DeviceClass  string         `json:"device_class"` // emulator | simulator | physical | cloud | unknown
	CI           *CIInfo        `json:"ci,omitempty"`
	Retries      []RetryOutcome `json:"retries,omitempty"`
}

// CIInfo is read from the GitHub Actions default environment variables.
type CIInfo struct {
	Provider    string `json:"provider"`
	Repository  string `json:"repository,omitempty"`
	Branch      string `json:"branch,omitempty"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	PullRequest int    `json:"pull_request,omitempty"`
	RunID       string `json:"run_id,omitempty"`
	RunAttempt  string `json:"run_attempt,omitempty"`
}

// RetryOutcome is what happened to a test that was re-run (retry_failed_tests).
type RetryOutcome struct {
	File     string `json:"file"`
	Test     string `json:"test"`
	Attempts int    `json:"attempts"`
	Passed   bool   `json:"passed"`
}

// Payload is the body that is sent: the unchanged report next to the envelope.
type Payload struct {
	Schema   int             `json:"schema"`
	Envelope Envelope        `json:"envelope"`
	Report   json.RawMessage `json:"report"`
}

var prRef = regexp.MustCompile(`^(\d+)/merge$`)

// CIFromEnv reads the GitHub Actions default variables (GITHUB_ACTIONS, GITHUB_REPOSITORY,
// GITHUB_REF_NAME, GITHUB_HEAD_REF, GITHUB_SHA, GITHUB_EVENT_NAME, GITHUB_RUN_ID,
// GITHUB_RUN_ATTEMPT; see docs.github.com/en/actions/reference/workflows-and-actions/variables).
// It returns nil outside GitHub Actions. For a pull request GITHUB_REF_NAME is "<number>/merge" and
// GITHUB_HEAD_REF is the source branch. Only these variables are read, never the rest of the environment.
func CIFromEnv(getenv func(string) string) *CIInfo {
	if getenv == nil {
		getenv = os.Getenv
	}
	if getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}
	ci := &CIInfo{
		Provider:   "github-actions",
		Repository: getenv("GITHUB_REPOSITORY"),
		Branch:     getenv("GITHUB_REF_NAME"),
		CommitSHA:  getenv("GITHUB_SHA"),
		RunID:      getenv("GITHUB_RUN_ID"),
		RunAttempt: getenv("GITHUB_RUN_ATTEMPT"),
	}
	if strings.HasPrefix(getenv("GITHUB_EVENT_NAME"), "pull_request") {
		if m := prRef.FindStringSubmatch(getenv("GITHUB_REF_NAME")); m != nil {
			n := 0
			for _, c := range m[1] {
				n = n*10 + int(c-'0')
			}
			ci.PullRequest = n
		}
		if head := getenv("GITHUB_HEAD_REF"); head != "" {
			ci.Branch = head
		}
	}
	return ci
}

// Transport sends a payload. It is an interface so the wire contract can change without touching
// the callers.
type Transport interface {
	Send(ctx context.Context, licenseKey string, body []byte) error
}
