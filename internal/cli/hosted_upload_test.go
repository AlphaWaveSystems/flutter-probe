package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/license"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

func uploadCmd(upload, dry bool) *cobra.Command {
	c := &cobra.Command{}
	c.Flags().Bool("upload-report", upload, "")
	c.Flags().Bool("upload-dry-run", dry, "")
	var out bytes.Buffer
	c.SetOut(&out)
	return c
}

// ciEnv makes the process look like a GitHub Actions push run, which a real upload requires.
func ciEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "o/r", "GITHUB_REF_NAME": "main",
		"GITHUB_SHA": "0123456789abcdef0123456789abcdef01234567", "GITHUB_RUN_ID": "5", "GITHUB_RUN_ATTEMPT": "1"} {
		t.Setenv(k, v)
	}
}

func hostedSampleResults() []runner.TestResult {
	return []runner.TestResult{{TestName: "login", File: "tests/login.probe", Passed: true, Attempts: 2}}
}

func TestHostedUploadDoesNothingUnlessAskedFor(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(false, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator")
	if hit || w.Len() != 0 {
		t.Fatalf("not asked for, yet hit=%v out=%q", hit, w.String())
	}
}

func TestHostedUploadWithoutKeySaysSoAndSendsNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator")
	if hit || !strings.Contains(w.String(), "hosted feature") || !strings.Contains(w.String(), "unaffected") {
		t.Fatalf("hit=%v out=%q", hit, w.String())
	}
}

func TestHostedUploadWithKeySendsOnceAndRetriesAreInTheEnvelope(t *testing.T) {
	ciEnv(t)
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanProbeCloud}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	var auth string
	var body []byte
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		auth = r.Header.Get("Authorization")
		body, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{ProbeVersion: "x", Platform: "ios"}, "simulator")
	if calls != 1 || auth != "Bearer "+key || !strings.Contains(string(body), `"attempts": 2`) || !strings.Contains(string(body), `"device_class": "simulator"`) {
		t.Fatalf("calls=%d auth ok=%v body=%s out=%q", calls, auth == "Bearer "+key, body, w.String())
	}
	if !strings.Contains(w.String(), "Report uploaded") {
		t.Fatalf("out=%q", w.String())
	}
}

func TestHostedUploadFailureIsAWarningOnly(t *testing.T) {
	ciEnv(t)
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanProbeCloud}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", http.StatusServiceUnavailable) }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL, Timeout: time.Second}}, hostedSampleResults(), runner.RunMetadata{Platform: "android"}, "emulator")
	if !strings.Contains(w.String(), "unaffected") || !strings.Contains(w.String(), "503") {
		t.Fatalf("out=%q", w.String())
	}
}

func TestHostedUploadRequiresTheProbeCloudPlan(t *testing.T) {
	ciEnv(t)
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanTeam}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{Platform: "android"}, "emulator")
	if hit || !strings.Contains(w.String(), "probe_cloud") {
		t.Fatalf("hit=%v out=%q", hit, w.String())
	}
}

func TestUploadDryRunNeedsNoKeyAndPrintsThePayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	c := uploadCmd(false, true)
	var out bytes.Buffer
	c.SetOut(&out)
	var w bytes.Buffer
	hostedUpload(c, &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{AppID: "x"}, "physical")
	if hit || !strings.Contains(out.String(), `"envelope"`) || !strings.Contains(out.String(), "tests/login.probe") || !strings.Contains(out.String(), "dry run") {
		t.Fatalf("hit=%v out=%q", hit, out.String())
	}
}

func TestDeviceClass(t *testing.T) {
	cases := []struct {
		dc       *runner.DeviceContext
		platform device.Platform
		cloud    bool
		want     string
	}{
		{&runner.DeviceContext{IsPhysical: true}, device.PlatformIOS, false, "physical"},
		{&runner.DeviceContext{}, device.PlatformIOS, false, "simulator"},
		{&runner.DeviceContext{}, device.PlatformAndroid, false, "emulator"},
		{nil, device.PlatformAndroid, true, "cloud"},
		{nil, device.PlatformAndroid, false, "unknown"},
	}
	for _, c := range cases {
		if got := deviceClass(c.dc, c.platform, c.cloud); got != c.want {
			t.Errorf("%+v = %q, want %q", c, got, c.want)
		}
	}
}

// A CI job supplies the key through PROBE_LICENSE_KEY: nothing is written to the runner, and the key
// never shows up in any output, including a dry run, a failure and a server that echoes it back.
func TestKeyFromEnvIsUsedAndNeverPrinted(t *testing.T) {
	ciEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanProbeCloud}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	t.Setenv(license.KeyValueEnv, key)

	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		http.Error(w, "rejected, your key was "+key, http.StatusUnauthorized) // a hostile or buggy server echoing it
	}))
	defer srv.Close()
	cfg := &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}

	var status, cmdOut bytes.Buffer
	c := uploadCmd(true, false)
	c.SetOut(&cmdOut)
	hostedUpload(c, &status, cfg, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator")
	if auth != "Bearer "+key {
		t.Fatalf("the key from %s was not used", license.KeyValueEnv)
	}

	var dryStatus, dryOut bytes.Buffer
	d := uploadCmd(false, true)
	d.SetOut(&dryOut)
	hostedUpload(d, &dryStatus, cfg, hostedSampleResults(), runner.RunMetadata{Platform: "ios"}, "simulator")

	for name, stream := range map[string]string{"status": status.String(), "command output": cmdOut.String(), "dry-run status": dryStatus.String(), "dry-run output": dryOut.String()} {
		if strings.Contains(stream, key) || strings.Contains(stream, "FP2.") {
			t.Errorf("the license key appears in the %s:\n%s", name, stream)
		}
	}
	if _, err := LoadLicenseKey(); err != nil {
		t.Fatal(err)
	}
	// Nothing was written to the home directory by the env path.
	if matches, _ := filepath.Glob(filepath.Join(home, ".flutterprobe", "*")); len(matches) != 0 {
		t.Errorf("the env key must not create files: %v", matches)
	}
}
