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
// tested directly. Machine output, CI, a pipe on stderr, the MCP stdio server and development builds never get extra text.
// The `probe prefs set` rule is in setsPrefs, which reads the command the way cobra does.
func updateNoticeSkip(args []string, getenv func(string) string, stderrIsTerminal bool, version string) string {
	switch {
	case version == "" || version == "dev":
		return "a development build"
	case getenv("CI") != "" && getenv("CI") != "false":
		return "CI"
	case !stderrIsTerminal:
		return "stderr is not a terminal"
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

// stderrIsTerminal reports whether stderr is a terminal, where a notice is useful to a person. A device such as /dev/null is not one.
func stderrIsTerminal() bool {
	return fdIsTerminal(os.Stderr.Fd())
}

// runUpdateCheck is this run's update check, set by Execute. It is nil when the run makes no check.
var runUpdateCheck *updatecheck.Check

// startUpdateCheck begins the check for this run. It returns nil when the run must not check or print anything.
func startUpdateCheck(args []string) *updatecheck.Check {
	if updateNoticeSkip(args, os.Getenv, stderrIsTerminal(), Version) != "" || setsPrefs(args) {
		return nil
	}
	return updatecheck.Start(Version, os.Getenv, time.Now())
}

// setsPrefs reports whether the command is `probe prefs set`. It finds the command the way cobra does, so a global flag before it
// (`probe --config x prefs set ...`) does not hide it: the setting it changes may be the one that turns the check off.
func setsPrefs(args []string) bool {
	cmd, _, err := rootCmd.Find(args)
	return err == nil && cmd.Name() == "set" && cmd.Parent() != nil && cmd.Parent().Name() == "prefs"
}

// printUpdateNotice writes the update line on stderr when this run knows of a newer release. A run that refreshes the stored answer
// waits for GitHub's reply for up to one budget, after the command's output, so the answer is kept for the next run; any other run
// does not wait. When nothing is known (offline, a failed request, a timeout) it writes nothing at all: a failed check is silent.
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

// updateDoctorCheck is the doctor line about updates. It uses the check this run already made, so a run that made none (CI, machine
// output, piped or development builds) gets no update line. The line is added only when a release answer is known.
func updateDoctorCheck() (doctorCheck, bool) {
	if runUpdateCheck == nil {
		return doctorCheck{}, false
	}
	st := runUpdateCheck.Status(updatecheck.Budget)
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
