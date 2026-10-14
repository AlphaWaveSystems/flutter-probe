package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/config"
)

// releasesURL is the page every FlutterProbe CLI release is published on.
const releasesURL = "https://github.com/AlphaWaveSystems/flutter-probe/releases"

// checkProbeVersion refuses a test run when probe.yaml pins a CLI release that this CLI does not meet. It runs before
// any device is touched, so no test starts. A project without probe_version is unchanged.
func checkProbeVersion(cfg *config.Config, cliVersion string) error {
	if cfg.ProbeVersion == "" {
		return nil
	}
	pin, err := config.ParseProbeVersion(cfg.ProbeVersion)
	if err != nil {
		return fmt.Errorf("invalid probe_version in probe.yaml: %w", err)
	}
	if pin.Satisfied(cliVersion) {
		return nil
	}
	return errors.New(probeVersionRefusal(pin, cliVersion))
}

// probeVersionRefusal names the installed CLI, the version probe.yaml requires, and how to get it.
func probeVersionRefusal(pin config.ProbeVersionPin, cliVersion string) string {
	installed := strings.TrimPrefix(cliVersion, "v")
	return fmt.Sprintf("probe test: the installed CLI is %s, but probe.yaml requires probe_version %s. No test was run.\n%s",
		installed, pin, probeVersionFix(pin, installed))
}

// probeVersionFix says how to get a CLI that meets the pin, or how to pin the installed release instead. brew needs the
// tap added before it can find the formula, and it installs the latest stable release, so an exact pin comes from the
// release page.
func probeVersionFix(pin config.ProbeVersionPin, installed string) string {
	var b strings.Builder
	if pin.Line {
		fmt.Fprintf(&b, "Install a %s release: download it from %s, or run brew tap AlphaWaveSystems/tap && brew install probe for the latest stable release, then check it with probe version.", pin, releasesURL)
	} else {
		fmt.Fprintf(&b, "Install %s: download that release from %s/tag/v%s, or run brew tap AlphaWaveSystems/tap && brew install probe for the latest stable release, then check it with probe version.", pin, releasesURL, pin)
	}
	if installed != "dev" {
		fmt.Fprintf(&b, " Or change probe_version in probe.yaml to %s.", installed)
	}
	return b.String()
}

// probeLine is the one line that names the CLI and the agent a run used: "probe CLI 0.25.1 / agent 0.25.1". An agent
// that predates the version field is shown as "unknown".
func probeLine(cliVersion, agentVersion string) string {
	agent := strings.TrimPrefix(agentVersion, "v")
	if agent == "" {
		agent = "unknown"
	}
	return fmt.Sprintf("probe CLI %s / agent %s", strings.TrimPrefix(cliVersion, "v"), agent)
}
