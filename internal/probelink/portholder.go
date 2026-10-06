package probelink

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// PortHolder describes the process listening on a loopback TCP port, e.g.
// "pid 4242 (Runner)", or "" when the port is free or the holder can't be
// determined. Best effort and read-only: shells out to lsof (macOS/Linux) with
// a short timeout and swallows every failure.
//
// FP-13: a stale simulator app (a Runner left over from a previous run) keeps
// the agent port bound, so the next run — typically on a different simulator —
// dials the wrong agent and fails with an auth rejection or a hang that names
// nothing. Naming the holder turns that into a one-line fix.
func PortHolder(port int) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc").Output()
	if err != nil {
		return ""
	}
	return parseLsofHolders(string(out))
}

// parseLsofHolders turns `lsof -Fpc` output (one "p<pid>" line followed by a
// "c<command>" line per process) into "pid N (cmd)[, pid M (cmd)]".
func parseLsofHolders(out string) string {
	var holders []string
	pid := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "p"):
			pid = line[1:]
		case strings.HasPrefix(line, "c") && pid != "":
			holders = append(holders, fmt.Sprintf("pid %s (%s)", pid, line[1:]))
			pid = ""
		}
	}
	return strings.Join(holders, ", ")
}

func isLoopback(host string) bool {
	switch host {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// portHolderHint is appended to dial errors: what already owns the agent
// port on this machine, and how to move off it. Empty for remote hosts and
// when the port is free (then the failure is something else).
func portHolderHint(host string, port int) string {
	if !isLoopback(host) {
		return ""
	}
	holder := PortHolder(port)
	if holder == "" {
		return ""
	}
	return fmt.Sprintf(" — agent port %d is held by %s; if that is not the app under test, stop it (e.g. `xcrun simctl terminate <udid> <bundle-id>` or kill the pid) or pick another port with --agent-port", port, holder)
}
