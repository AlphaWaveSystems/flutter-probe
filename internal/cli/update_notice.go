package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/updatecheck"
)

// updateNoticeSkip names why a run makes no update check and prints no notice, or "" when it may. It is pure, so the rules are
// tested directly. Machine output, CI, a pipe on stderr, the MCP stdio server and development builds never get extra text. A
// `probe prefs set` run makes no check either: the setting it changes may be the one that turns the check off.
func updateNoticeSkip(args []string, getenv func(string) string, stderrIsTerminal bool, version string) string {
	switch {
	case version == "" || version == "dev":
		return "a development build"
	case getenv("CI") != "" && getenv("CI") != "false":
		return "CI"
	case !stderrIsTerminal:
		return "stderr is not a terminal"
	case len(args) >= 2 && args[0] == "prefs" && args[1] == "set":
		return "prefs set"
	}
	for i, a := range args {
		switch {
		case a == "mcp-server":
			return "the MCP server"
		case a == "--format" && i+1 < len(args) && machineFormat(args[i+1]):
			return "--format " + args[i+1]
		case strings.HasPrefix(a, "--format=") && machineFormat(strings.TrimPrefix(a, "--format=")):
			return a
		}
	}
	return ""
}

// machineFormat reports whether a --format value asks for output that scripts read.
func machineFormat(f string) bool {
	switch strings.ToLower(f) {
	case "json", "junit":
		return true
	}
	return false
}

// stderrIsTerminal reports whether stderr is a terminal, where a notice is useful to a person.
func stderrIsTerminal() bool {
	info, err := os.Stderr.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// startUpdateCheck begins the check for this run. It returns nil when the run must not check or print anything.
func startUpdateCheck(args []string) *updatecheck.Check {
	if updateNoticeSkip(args, os.Getenv, stderrIsTerminal(), Version) != "" {
		return nil
	}
	return updatecheck.Start(Version, os.Getenv, time.Now())
}

// printUpdateNotice writes the update line on stderr when this run knows of a newer release. It never waits for the network, and
// when nothing is known (offline, a failed request, a timeout) it writes nothing at all: a failed check is silent.
func printUpdateNotice(w io.Writer, h *updatecheck.Check) {
	if h == nil {
		return
	}
	var wait time.Duration
	if h.Pending() {
		wait = updatecheck.Budget
	}
	st := h.Status(wait)
	if !st.UpdateAvailable {
		return
	}
	fmt.Fprintln(w, updatecheck.Notice(st.Current, st.Latest, st.URL, homebrewFormula()))
}

// homebrewFormula names the Homebrew formula this binary came from, or "" when it did not come from Homebrew.
func homebrewFormula() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return updatecheck.HomebrewFormula(exe)
}

// updateDoctorCheck is the doctor line about updates. It is added only when a release answer is known: an offline machine, a
// disabled check and a development build see nothing about updates in the report.
func updateDoctorCheck() (doctorCheck, bool) {
	if Version == "dev" {
		return doctorCheck{}, false
	}
	st := updatecheck.Start(Version, os.Getenv, time.Now()).Status(updatecheck.Budget)
	if st.Source == "disabled" || st.Latest == "" {
		return doctorCheck{}, false
	}
	c := doctorCheck{name: "update", status: "ok"}
	if st.UpdateAvailable {
		c.detail = fmt.Sprintf("%s is available (you have %s)", st.Latest, st.Current)
	} else {
		c.detail = fmt.Sprintf("%s is the latest stable release", st.Latest)
	}
	return c, true
}
