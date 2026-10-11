package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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

func hostedSampleResults() []runner.TestResult {
	return []runner.TestResult{{TestName: "login", File: "tests/login.probe", Passed: true, Attempts: 2}}
}

func TestHostedUploadDoesNothingUnlessAskedFor(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(false, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{}, "simulator")
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
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{}, "simulator")
	if hit || !strings.Contains(w.String(), "hosted feature") || !strings.Contains(w.String(), "unaffected") {
		t.Fatalf("hit=%v out=%q", hit, w.String())
	}
}

func TestHostedUploadWithKeySendsOnceAndRetriesAreInTheEnvelope(t *testing.T) {
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
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{ProbeVersion: "x"}, "simulator")
	if calls != 1 || auth != "Bearer "+key || !strings.Contains(string(body), `"attempts": 2`) || !strings.Contains(string(body), `"device_class": "simulator"`) {
		t.Fatalf("calls=%d auth ok=%v body=%s out=%q", calls, auth == "Bearer "+key, body, w.String())
	}
	if !strings.Contains(w.String(), "Report uploaded") {
		t.Fatalf("out=%q", w.String())
	}
}

func TestHostedUploadFailureIsAWarningOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pub, key := signedTestKey(t, license.Claims{ID: "l", Customer: "acme", Plans: []license.Plan{license.PlanProbeCloud}, Seats: 1, Expires: time.Now().Add(time.Hour)})
	t.Setenv(license.KeyEnv, pub)
	if err := runLicenseActivate(licenseActivateCmd, []string{key}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 503) }))
	defer srv.Close()
	var w bytes.Buffer
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL, Timeout: time.Second}}, hostedSampleResults(), runner.RunMetadata{}, "emulator")
	if !strings.Contains(w.String(), "unaffected") || !strings.Contains(w.String(), "503") {
		t.Fatalf("out=%q", w.String())
	}
}

func TestHostedUploadRequiresTheProbeCloudPlan(t *testing.T) {
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
	hostedUpload(uploadCmd(true, false), &w, &config.Config{Hosted: config.HostedConfig{URL: srv.URL}}, hostedSampleResults(), runner.RunMetadata{}, "emulator")
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
