package cli

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphawavesystems/flutter-probe/internal/cloud"
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
// --upload-local takes the local path instead: a local run is never a CI upload, whatever the config says.
func hostedUpload(cmd *cobra.Command, w io.Writer, cfg *config.Config, results []runner.TestResult, meta runner.RunMetadata, class string) {
	if local, _ := cmd.Flags().GetBool("upload-local"); local {
		localUpload(cmd, w, cfg, results, meta, class, "")
		return
	}
	flagOn, _ := cmd.Flags().GetBool("upload-report")
	dry, _ := cmd.Flags().GetBool("upload-dry-run")
	if !flagOn && !dry && !cfg.Hosted.UploadReport {
		return
	}

	key := ""
	if !dry {
		k, ok := uploadLicenseKey(w)
		if !ok {
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
	env := hosted.Envelope{ProbeVersion: Version, DeviceClass: class, CI: hosted.CIFromEnv(nil, func(msg string) { statusWarn(w, "%s", msg) }), RunLabel: label}
	for _, r := range results {
		if r.Attempts > 1 {
			env.Retries = append(env.Retries, hosted.RetryOutcome{File: r.File, Test: r.TestName, Attempts: min(r.Attempts, 20), Passed: r.Passed})
		}
	}
	var out io.Writer
	if dry {
		out = cmd.OutOrStdout()
	}
	baseURL := cfg.Hosted.URL
	if v := strings.TrimSpace(os.Getenv("PROBE_CLOUD_URL")); v != "" {
		baseURL = v
	}
	audience := cfg.Hosted.Audience
	if v := strings.TrimSpace(os.Getenv("PROBE_CLOUD_AUDIENCE")); v != "" {
		audience = v
	}
	warn := func(msg string) { statusWarn(w, "%s", msg) }
	opts := hosted.Options{
		Report: raw, Envelope: env, LicenseKey: key, BaseURL: baseURL, Timeout: cfg.Hosted.Timeout, DryRun: dry, Out: out,
		RetryWindow: cfg.Hosted.RetryWindow, Warn: warn,
	}
	if dry {
		opts.DryAttestation = hosted.AttestationAvailable(os.Getenv)
	} else {
		opts.Attestation = hosted.RequestAttestation(context.Background(), os.Getenv, nil, audience, warn)
	}
	err = hosted.Upload(context.Background(), opts)
	switch {
	case err != nil:
		statusWarn(w, "report upload failed (the run is unaffected): %v", err)
	case !dry:
		host := baseURL
		if strings.TrimSpace(host) == "" {
			host = hosted.DefaultBaseURL
		}
		if u, perr := url.Parse(host); perr == nil && u.Host != "" {
			host = u.Host
		}
		tokenNote := "without an identity token"
		if opts.Attestation != "" {
			tokenNote = "with an identity token"
		}
		statusOK(w, "Report uploaded to %s %s (scrubbed: relative paths, no artifacts)", host, tokenNote)
	}
}

// uploadLicenseKey checks what a real upload needs: an activated license with the probe_cloud plan, and its key. When
// something is missing it says why and returns false. The run is never affected.
func uploadLicenseKey(w io.Writer) (string, bool) {
	claims, err := LoadLicense()
	switch {
	case errors.Is(err, os.ErrNotExist):
		statusInfo(w, "Report upload is a hosted feature (Probe Cloud, preview) and needs a license key: none is activated, so nothing was uploaded. The run is unaffected. See `probe license activate`.")
		return "", false
	case err != nil && !errors.Is(err, license.ErrExpired):
		statusWarn(w, "report upload skipped: %v", err)
		return "", false
	case errors.Is(err, license.ErrExpired):
		statusWarn(w, "report upload skipped: the license has expired")
		return "", false
	case !claims.Has(license.PlanProbeCloud):
		statusInfo(w, "Report upload needs the probe_cloud plan: this license does not include it, so nothing was uploaded. The run is unaffected.")
		return "", false
	}
	k, kerr := LoadLicenseKey()
	if kerr != nil {
		statusWarn(w, "report upload skipped: %v", kerr)
		return "", false
	}
	return k, true
}

// uploadFlagsError refuses the flag combination that cannot be honoured: --upload-local and --upload-report are different
// uploads, and the run must not guess which one the user meant.
func uploadFlagsError(cmd *cobra.Command) error {
	localUp, _ := cmd.Flags().GetBool("upload-local")
	reportUp, _ := cmd.Flags().GetBool("upload-report")
	if localUp && reportUp {
		return errors.New("--upload-local and --upload-report are different uploads: use one of them")
	}
	return nil
}

// localUpload sends this run as a local run (--upload-local): the same license and scrubbed report as a CI upload, with
// the git facts of the repository in the envelope instead of CI facts, and no identity token. Nothing is sent without
// the flag, and every failure is a warning that never changes the run's exit status. dir is the directory the run
// started in; empty means the working directory.
func localUpload(cmd *cobra.Command, w io.Writer, cfg *config.Config, results []runner.TestResult, meta runner.RunMetadata, class, dir string) {
	dry, _ := cmd.Flags().GetBool("upload-dry-run")
	key := ""
	if !dry {
		k, ok := uploadLicenseKey(w)
		if !ok {
			return
		}
		key = k
	}
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			statusWarn(w, "local upload skipped: %v", err)
			return
		}
		dir = wd
	}
	facts, err := hosted.ReadGitFacts(dir)
	if err != nil {
		statusWarn(w, "local upload skipped (the run is unaffected): %v", err)
		return
	}
	stateDir, err := localStateDir()
	if err != nil {
		statusWarn(w, "local upload skipped: %v", err)
		return
	}
	signer, err := hosted.Signer(stateDir)
	if err != nil {
		statusWarn(w, "local upload skipped: %v", err)
		return
	}
	// The sequence number is reserved before the body is built and written down before anything is sent, so a retry
	// resends this same body. A dry run reserves nothing.
	var seq int64
	if dry {
		seq, err = hosted.PeekSeq(stateDir, signer, time.Now())
	} else {
		seq, err = hosted.NextSeq(stateDir, signer, time.Now())
	}
	if err != nil {
		statusWarn(w, "local upload skipped: %v", err)
		return
	}
	rep := runner.NewReporter(runner.FormatJSON, io.Discard, false)
	rep.SetMetadata(meta)
	raw, err := rep.JSONBytes(results)
	if err != nil {
		statusWarn(w, "local upload skipped: %v", err)
		return
	}
	label, _ := cmd.Flags().GetString("upload-label")
	if label == "" {
		label = cfg.Hosted.Label
	}
	env := hosted.Envelope{
		ProbeVersion: Version,
		DeviceClass:  class,
		RunLabel:     hosted.SanitizeLabel(label),
		Local: &hosted.LocalInfo{
			Repository: facts.Repository,
			Branch:     facts.Branch,
			CommitSHA:  facts.CommitSHA,
			Dirty:      facts.Dirty,
			Tree:       facts.Tree,
			Signer:     signer,
			Seq:        seq,
		},
	}
	var out io.Writer
	if dry {
		out = cmd.OutOrStdout()
		statusInfo(w, "local run: sequence number %d would be used; a dry run reserves nothing and sends nothing", seq)
	}
	baseURL := cfg.Hosted.URL
	if v := strings.TrimSpace(os.Getenv("PROBE_CLOUD_URL")); v != "" {
		baseURL = v
	}
	var receipt hosted.Receipt
	err = hosted.Upload(context.Background(), hosted.Options{
		Report: raw, Envelope: env, LicenseKey: key, BaseURL: baseURL, Timeout: cfg.Hosted.Timeout, DryRun: dry, Out: out,
		RetryWindow: cfg.Hosted.RetryWindow, Warn: func(msg string) { statusWarn(w, "%s", msg) }, Receipt: &receipt,
	})
	switch {
	case err != nil:
		statusWarn(w, "local upload failed (the run is unaffected): %v", err)
	case dry:
		// Upload has printed the payload.
	case receipt.Duplicate:
		statusOK(w, "Local run already stored on %s (run %s): this upload repeats an earlier one and changed nothing.", displayHost(baseURL), runRef(receipt))
	default:
		statusOK(w, "Local run uploaded to %s (run %s). Local evidence is unverified: it is shown beside the CI result and never counts toward a pull request's verdict.", displayHost(baseURL), runRef(receipt))
	}
}

// localStateDir is where the local-run signer and sequence number live: ~/.flutterprobe/local-upload, beside the license.
func localStateDir() (string, error) {
	base, err := cloud.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "local-upload"), nil
}

// displayHost is the service an upload goes to, for the status line: the host without the scheme or the path.
func displayHost(baseURL string) string {
	host := strings.TrimSpace(baseURL)
	if host == "" {
		host = hosted.DefaultBaseURL
	}
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		return u.Host
	}
	return host
}

// runRef is the run key the service answered with, for printing, or a note when the answer carried none.
func runRef(r hosted.Receipt) string {
	if k := hosted.Sanitize(r.RunKey); k != "" {
		return k
	}
	return "no run key in the answer"
}
