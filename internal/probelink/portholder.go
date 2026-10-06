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

// Holder is a process listening on the agent port.
type Holder struct {
	PID     string
	Name    string // short command name, e.g. "adb", "Runner"
	Command string // full command line when it could be read (used to spot simulator apps)
}

// holders returns the processes listening on a loopback TCP port. Best effort.
func holders(port int) []Holder {
	if runtime.GOOS == "windows" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc").Output()
	if err != nil {
		return nil
	}
	var hs []Holder
	pid := ""
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "p"):
			pid = line[1:]
		case strings.HasPrefix(line, "c") && pid != "":
			h := Holder{PID: pid, Name: line[1:]}
			if cmd, err := exec.CommandContext(ctx, "ps", "-o", "command=", "-p", pid).Output(); err == nil {
				h.Command = strings.TrimSpace(string(cmd))
			}
			hs = append(hs, h)
			pid = ""
		}
	}
	return hs
}

// simulatorUDID extracts the simulator device id from a process path such as
// .../CoreSimulator/Devices/<UDID>/data/Containers/Bundle/Application/.../Runner.
func simulatorUDID(command string) string {
	const marker = "/CoreSimulator/Devices/"
	i := strings.Index(command, marker)
	if i < 0 {
		return ""
	}
	rest := command[i+len(marker):]
	if j := strings.Index(rest, "/"); j > 0 {
		return rest[:j]
	}
	return ""
}

// describeHolders turns the holders of port into an actionable one-liner: an
// adb forward and an iOS simulator app need different fixes, and the old
// message said the same thing for both (FP-19, reported when an iOS Runner held
// the port while an Android run started).
func describeHolders(port int, hs []Holder) string {
	if len(hs) == 0 {
		return ""
	}
	var parts []string
	for _, h := range hs {
		switch {
		case strings.EqualFold(h.Name, "adb"):
			parts = append(parts, fmt.Sprintf("an adb port forward (pid %s) — left over from an earlier run, or another Android device is forwarded to this port: remove it with `adb forward --remove tcp:%d` (list them with `adb forward --list`)", h.PID, port))
		case simulatorUDID(h.Command) != "":
			udid := simulatorUDID(h.Command)
			parts = append(parts, fmt.Sprintf("an iOS simulator app %q (pid %s, simulator %s) — probably left over from an iOS run: stop it with `xcrun simctl terminate %s <bundle-id>` or shut the simulator down", h.Name, h.PID, udid, udid))
		default:
			parts = append(parts, fmt.Sprintf("pid %s (%s)", h.PID, h.Name))
		}
	}
	return fmt.Sprintf(" — agent port %d is held by %s; if that is not the app under test, stop it or pick another port with --agent-port", port, strings.Join(parts, "; "))
}

// portHolderHint is appended to dial errors: what already owns the agent
// port on this machine, and how to move off it. Empty for remote hosts and
// when the port is free (then the failure is something else).
func portHolderHint(host string, port int) string {
	if !isLoopback(host) {
		return ""
	}
	return describeHolders(port, holders(port))
}
