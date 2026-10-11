// Package hosted is the opt-in client of the hosted report service (Probe Cloud, preview): it
// builds a payload from the run's JSON report plus a small envelope, scrubs it, and sends it.
// Nothing leaves the machine unless the user asked for it (--upload-report or hosted.upload_report),
// a license with the probe_cloud plan is present, and the scrubber has run. Failures are warnings:
// an upload never fails or noticeably slows a test run.
package hosted

import (
	"context"
	"encoding/json"
	"fmt"
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
	RunLabel     string         `json:"run_label,omitempty"` // optional free label for a suite, shard or matrix leg; part of the server's run key, so two uploads of one CI job must differ in it
	Retries      []RetryOutcome `json:"retries,omitempty"`
}

// CIInfo is read from the GitHub Actions default environment variables.
type CIInfo struct {
	Provider    string `json:"provider"`
	Repository  string `json:"repository,omitempty"`
	Branch      string `json:"branch,omitempty"`      // push: GITHUB_REF_NAME; pull request: GITHUB_HEAD_REF (the source branch, not "N/merge")
	BaseBranch  string `json:"base_branch,omitempty"` // pull request only: GITHUB_BASE_REF (the target branch); omitted on push
	Job         string `json:"job,omitempty"`         // GITHUB_JOB, the job id (1 to 100 of [0-9A-Za-z_-]); omitted if it does not fit
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
		CommitSHA:  strings.ToLower(getenv("GITHUB_SHA")),
		RunID:      getenv("GITHUB_RUN_ID"),
		RunAttempt: getenv("GITHUB_RUN_ATTEMPT"),
	}
	if job := getenv("GITHUB_JOB"); jobRe.MatchString(job) {
		ci.Job = job
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
		ci.BaseBranch = getenv("GITHUB_BASE_REF")
	}
	return ci
}

// Transport sends a payload. It is an interface so the wire contract can change without touching
// the callers.
type Transport interface {
	Send(ctx context.Context, licenseKey string, body []byte) error
}

var (
	sha40   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	decimal = regexp.MustCompile(`^[0-9]+$`)
	labelRe = regexp.MustCompile(`^[0-9A-Za-z._:/-]{1,64}$`)
	jobRe   = regexp.MustCompile(`^[0-9A-Za-z_-]{1,100}$`)
)

// MaxResults is the most results one upload may carry.
const MaxResults = 5000

// ValidLabel reports whether s is an acceptable run label (1 to 64 of [0-9A-Za-z._:/-]) or empty.
func ValidLabel(s string) bool { return s == "" || labelRe.MatchString(s) }

// SanitizeLabel turns free text (for example matrix values) into a valid run label: characters outside
// the allowed set become '-', and the result is cut to 64 characters.
func SanitizeLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r < 0x80 && (r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("._:/-", r)) {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// Preflight lists why this upload would be refused, so the user gets a clear warning instead of a
// rejected request: the run must be a GitHub Actions run with complete CI facts, the report must name its
// platform (android or ios), carry at most MaxResults results, and the suite label must be valid.
func Preflight(env Envelope, report []byte) []string {
	var problems []string
	switch ci := env.CI; {
	case ci == nil:
		problems = append(problems, "this is not a GitHub Actions run, and the hosted service needs the CI facts (repository, branch, commit); only --upload-dry-run works outside CI")
	default:
		if ci.Provider != "github-actions" {
			problems = append(problems, "unsupported CI provider")
		}
		if ci.Repository == "" {
			problems = append(problems, "GITHUB_REPOSITORY is empty")
		}
		if ci.Branch == "" {
			problems = append(problems, "no branch (GITHUB_REF_NAME / GITHUB_HEAD_REF is empty)")
		}
		if !sha40.MatchString(ci.CommitSHA) {
			problems = append(problems, "GITHUB_SHA is not a 40-character commit hash")
		}
		if !decimal.MatchString(ci.RunID) || !decimal.MatchString(ci.RunAttempt) {
			problems = append(problems, "GITHUB_RUN_ID or GITHUB_RUN_ATTEMPT is not a number")
		}
	}
	if !ValidLabel(env.RunLabel) {
		problems = append(problems, "the run label must be 1 to 64 characters of letters, digits, '.', '_', ':', '/' or '-'")
	}
	var rep struct {
		Metadata struct {
			Platform string `json:"platform"`
		} `json:"metadata"`
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(report, &rep); err != nil {
		problems = append(problems, "the report is not valid JSON")
		return problems
	}
	if p := rep.Metadata.Platform; p != "android" && p != "ios" {
		problems = append(problems, "the report has no platform (android or ios) in its metadata, which happens when no device was connected")
	}
	if len(rep.Results) > MaxResults {
		problems = append(problems, fmt.Sprintf("the report has %d results; at most %d can be uploaded", len(rep.Results), MaxResults))
	}
	return problems
}
