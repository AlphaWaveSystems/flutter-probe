package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The review workflow skips the review, with a notice and without failing, when the oauth provider has no CLAUDE_CODE_OAUTH_TOKEN. The other providers
// without their credential still fail closed. These tests read the workflow file and run the script of its credential step.

type reviewWorkflowStep struct {
	Name string `yaml:"name"`
	ID   string `yaml:"id"`
	If   string `yaml:"if"`
	Uses string `yaml:"uses"`
	Run  string `yaml:"run"`
}

func reviewSteps(t *testing.T) []reviewWorkflowStep {
	t.Helper()
	raw, err := os.ReadFile("../.github/workflows/review-agent.yml")
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []reviewWorkflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("review-agent.yml is not valid YAML: %v", err)
	}
	return wf.Jobs["review"].Steps
}

const skipCondition = "steps.creds.outputs.skip != 'true'"

// Every step after the credential step is conditional on it, so a skip skips the review, the freshness check and the submission.
func TestReviewStepsAfterTheCredentialCheckAreSkippedWhenItSkips(t *testing.T) {
	steps := reviewSteps(t)
	creds := -1
	for i, s := range steps {
		if s.ID == "creds" {
			creds = i
		}
	}
	if creds < 0 {
		t.Fatal("the review job has no step with id creds")
	}
	if steps[creds].Name != "Validate inputs and credentials" {
		t.Errorf("the creds step is %q, want \"Validate inputs and credentials\"", steps[creds].Name)
	}
	if len(steps)-creds-1 < 6 {
		t.Fatalf("only %d step(s) after the credential check, want the six of the review", len(steps)-creds-1)
	}
	for _, s := range steps[creds+1:] {
		if !strings.Contains(s.If, skipCondition) {
			t.Errorf("step %q (%s) does not have `%s` in its condition (is %q): it would run after a skip", s.Name, s.Uses, skipCondition, s.If)
		}
	}
	// The checkout before it stays unconditional: the credential step needs nothing from it, but the run keeps its shape.
	for _, s := range steps[:creds] {
		if strings.Contains(s.If, "creds") {
			t.Errorf("step %q before the credential step refers to it", s.Name)
		}
	}
}

// runCredentialStep runs the script of the credential step with the given provider and credentials, with the tools it checks for stubbed, and returns the
// exit code, the notice lines of its output and the GITHUB_OUTPUT file.
func runCredentialStep(t *testing.T, provider, oauth, apiKey, localURL string) (code int, out, ghOutput string) {
	t.Helper()
	var script string
	for _, s := range reviewSteps(t) {
		if s.ID == "creds" {
			script = s.Run
		}
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"git", "gh", "jq", "curl", "openssl"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	outFile := filepath.Join(dir, "github_output")
	if err := os.WriteFile(outFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-eo", "pipefail", "-c", script)
	cmd.Env = []string{
		"PATH=" + bin + ":/usr/bin:/bin", "GITHUB_OUTPUT=" + outFile,
		"PR=891", "EVENT_HEAD_SHA=" + strings.Repeat("a", 40), "BASE_REF=main", "PROVIDER=" + provider,
		"OAUTH_TOKEN=" + oauth, "API_KEY=" + apiKey, "LOCAL_URL=" + localURL,
	}
	b, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	g, _ := os.ReadFile(outFile)
	return code, string(b), string(g)
}

func TestCredentialStepSkipsOnlyForOauthWithoutItsToken(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	cases := []struct {
		name                          string
		provider, oauth, key, local   string
		wantCode                      int
		wantSkip                      bool
		wantNotice, wantErrorContains string
	}{
		{"oauth without the token skips, with a notice, and succeeds", "oauth", "", "", "", 0, true, "::notice::The review was skipped", ""},
		{"oauth with the token reviews", "oauth", "token", "", "", 0, false, "", ""},
		{"anthropic without its key fails closed", "anthropic", "", "", "", 1, false, "", "ANTHROPIC_API_KEY is not set"},
		{"anthropic with its key reviews", "anthropic", "", "key", "", 0, false, "", ""},
		{"anthropic without its key but with an oauth token still fails closed", "anthropic", "token", "", "", 1, false, "", "ANTHROPIC_API_KEY is not set"},
		{"local without its URL fails closed", "local", "", "", "", 1, false, "", "REVIEW_LOCAL_URL is not set"},
		{"local with its URL reviews", "local", "", "", "http://127.0.0.1:1234", 0, false, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out, gh := runCredentialStep(t, c.provider, c.oauth, c.key, c.local)
			if code != c.wantCode {
				t.Errorf("exit code %d, want %d; output: %s", code, c.wantCode, out)
			}
			if skip := strings.Contains(gh, "skip=true"); skip != c.wantSkip {
				t.Errorf("skip output = %v, want %v (GITHUB_OUTPUT: %q)", skip, c.wantSkip, gh)
			}
			if c.wantNotice != "" && !strings.Contains(out, c.wantNotice) {
				t.Errorf("no %q in the output: %s", c.wantNotice, out)
			}
			if c.wantErrorContains != "" && !strings.Contains(out, c.wantErrorContains) {
				t.Errorf("no %q in the output: %s", c.wantErrorContains, out)
			}
			if c.wantSkip && strings.Contains(out, "::error::") {
				t.Errorf("a skip must not print an error: %s", out)
			}
		})
	}
	// Bad inputs still fail before any credential is looked at.
}

// The trust boundary is untouched: the gate job still fails for an untrusted author instead of skipping.
func TestReviewGateStillFailsForUntrustedAuthors(t *testing.T) {
	raw, err := os.ReadFile("../.github/workflows/review-agent.yml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"Pull requests from forks are not reviewed automatically", "is not trusted for automated review", "exit 1"} {
		if !strings.Contains(s, want) {
			t.Errorf("the gate job no longer contains %q", want)
		}
	}
}
