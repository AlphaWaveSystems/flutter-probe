package cli

import (
	"fmt"
	"io"

	"github.com/alphawavesystems/flutter-probe/internal/sysdialog"
)

// ANSI formatting prefixes for status messages.
const (
	prefixOK   = "  \033[32m✓\033[0m  "
	prefixWarn = "  \033[33m⚠\033[0m  "
	prefixFail = "  \033[31m✗\033[0m  "
	prefixNav  = "  \033[36m→\033[0m  "
)

// Status message constants used across CLI commands.
const (
	msgNoProbeFiles            = "No .probe files found."
	msgCreatingRelaySession    = "  Creating relay session..."
	msgWaitingForAgentRelay    = "  Waiting for agent to connect to relay..."
	msgWaitingForTokenIOS      = "  Waiting for ProbeAgent token (iOS)..."
	msgWaitingForToken         = "  Waiting for ProbeAgent token..."
	msgCollectingArtifacts     = "  Collecting cloud session artifacts..."
	msgCloudSessionStopped     = "Cloud session stopped"
	msgAppInstalledAndLaunched = "App installed and launched"
	msgUploadingToCloud        = "\n  Uploading results to FlutterProbe Cloud..."
	msgCloudTokenMissing       = "\n  \033[33m⚠  --cloud-token not set and cloud.token not found in probe.yaml. Skipping cloud upload.\033[0m"
)

// statusOK writes a success status line (green ✓) to w.
func statusOK(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, prefixOK+format+"\n", a...)
}

// statusWarn writes a warning status line (yellow ⚠) to w.
func statusWarn(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, prefixWarn+format+"\n", a...)
}

// printAgentWarning writes an agent or CLI warning (yellow ⚠) to w with resolved environment values masked, as errors
// are. A warning can name the widget a step acted on, and that label can be a value the script took from the environment.
func printAgentWarning(w io.Writer, msg string) {
	statusWarn(w, "%s", sysdialog.ScrubKnown(msg))
}

// statusFail writes a failure status line (red ✗) to w.
func statusFail(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, prefixFail+format+"\n", a...)
}

// statusNav writes a navigation link line (cyan →) to w.
func statusNav(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, prefixNav+format+"\n", a...)
}

// statusInfo writes a plain indented status line to w.
func statusInfo(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, "  "+format+"\n", a...)
}
