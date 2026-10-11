package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/config"
	"github.com/alphawavesystems/flutter-probe/internal/device"
	"github.com/alphawavesystems/flutter-probe/internal/hosted"
	"github.com/alphawavesystems/flutter-probe/internal/license"
	"github.com/alphawavesystems/flutter-probe/internal/runner"
)

// deviceClass names what kind of device a run used, for the upload envelope.
func deviceClass(devCtx *runner.DeviceContext, platform device.Platform, cloudRun bool) string {
	switch {
	case devCtx != nil && devCtx.IsPhysical:
		return "physical"
	case devCtx != nil && platform == device.PlatformIOS:
		return "simulator"
	case devCtx != nil && platform == device.PlatformAndroid:
		return "emulator"
	case cloudRun:
		return "cloud"
	}
	return "unknown"
}

// hostedUpload sends the scrubbed run report to the hosted service when the user asked for it
// (--upload-report, hosted.upload_report, or --upload-dry-run). Every failure is a warning: it never
// changes the exit status of the run, and the upload is one attempt with a short timeout.
func hostedUpload(cmd *cobra.Command, w io.Writer, cfg *config.Config, results []runner.TestResult, meta runner.RunMetadata, class string) {
	flagOn, _ := cmd.Flags().GetBool("upload-report")
	dry, _ := cmd.Flags().GetBool("upload-dry-run")
	if !flagOn && !dry && !cfg.Hosted.UploadReport {
		return
	}

	key := ""
	if !dry {
		claims, err := LoadLicense()
		switch {
		case errors.Is(err, os.ErrNotExist):
			statusInfo(w, "Report upload is a hosted feature (Probe Cloud, preview) and needs a license key: none is activated, so nothing was uploaded. The run is unaffected. See `probe license activate`.")
			return
		case err != nil && !errors.Is(err, license.ErrExpired):
			statusWarn(w, "report upload skipped: %v", err)
			return
		case errors.Is(err, license.ErrExpired):
			statusWarn(w, "report upload skipped: the license has expired")
			return
		case !claims.Has(license.PlanProbeCloud):
			statusInfo(w, "Report upload needs the probe_cloud plan: this license does not include it, so nothing was uploaded. The run is unaffected.")
			return
		}
		k, kerr := LoadLicenseKey()
		if kerr != nil {
			statusWarn(w, "report upload skipped: %v", kerr)
			return
		}
		key = k
	}

	rep := runner.NewReporter(runner.FormatJSON, io.Discard, false)
	rep.SetMetadata(meta)
	raw, err := rep.JSONBytes(results)
	if err != nil {
		statusWarn(w, "report upload skipped: %v", err)
		return
	}
	label, _ := cmd.Flags().GetString("upload-label")
	if label == "" {
		label = cfg.Hosted.Label
	}
	label = hosted.SanitizeLabel(label)
	env := hosted.Envelope{ProbeVersion: Version, DeviceClass: class, CI: hosted.CIFromEnv(nil), RunLabel: label}
	for _, r := range results {
		if r.Attempts > 1 {
			env.Retries = append(env.Retries, hosted.RetryOutcome{File: r.File, Test: r.TestName, Attempts: min(r.Attempts, 20), Passed: r.Passed})
		}
	}
	var out io.Writer
	if dry {
		out = cmd.OutOrStdout()
	}
	err = hosted.Upload(context.Background(), hosted.Options{
		Report: raw, Envelope: env, LicenseKey: key, BaseURL: cfg.Hosted.URL, Timeout: cfg.Hosted.Timeout, DryRun: dry, Out: out,
	})
	switch {
	case err != nil:
		statusWarn(w, "report upload failed (the run is unaffected): %v", err)
	case !dry:
		statusOK(w, "Report uploaded (scrubbed: relative paths, no artifacts)")
	}
}
