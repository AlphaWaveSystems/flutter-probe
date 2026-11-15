package device

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The system WebView of an app opens an abstract Unix socket named webview_devtools_remote_<pid> when WebView debugging is on, where <pid> is the process id of the app that owns the WebView.
// /proc/net/unix lists abstract sockets with a leading "@". See docs/proposals/webview-content-automation.md.
const webViewSocketPrefix = "webview_devtools_remote_"

// maxPID bounds what the parser accepts as a process id (Linux allows up to 2^22); anything longer or larger is not a pid.
const maxPID = 1 << 22

// WebViewSocket is one DevTools socket found in the table.
type WebViewSocket struct {
	PID  int
	Name string // without the leading "@", as adb's localabstract: wants it
}

// ParseWebViewSockets reads the output of `cat /proc/net/unix` and returns every webview_devtools_remote_<pid> socket in it, one entry per distinct name, ordered by pid.
// It is a pure function. A line is taken only when its last field (the path) is exactly "@webview_devtools_remote_" followed by decimal digits that make a pid: a file-system path with that name, a name
// with other characters, a zero or oversized pid and every other line are ignored, and so are the header line and blank lines.
func ParseWebViewSockets(table string) []WebViewSocket {
	seen := map[int]bool{}
	var out []WebViewSocket
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		// Num RefCount Protocol Flags Type St [Inode [Path]]: a socket with a path has 8 fields
		if len(fields) != 8 {
			continue
		}
		path := fields[7]
		if !strings.HasPrefix(path, "@"+webViewSocketPrefix) {
			continue
		}
		digits := strings.TrimPrefix(path, "@"+webViewSocketPrefix)
		if digits == "" || len(digits) > 8 || digits[0] == '0' {
			continue
		}
		pid, err := strconv.Atoi(digits)
		if err != nil || pid <= 0 || pid > maxPID || strconv.Itoa(pid) != digits {
			continue
		}
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, WebViewSocket{PID: pid, Name: webViewSocketPrefix + digits})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// OwnWebViewSocket returns the DevTools socket that belongs to the process pid, and no other. A socket of another process is never returned, whatever else is in the table.
// An error says why there is none: pid is not a valid process id, or the table has no socket for it.
func OwnWebViewSocket(table string, pid int) (WebViewSocket, error) {
	if pid <= 0 || pid > maxPID {
		return WebViewSocket{}, fmt.Errorf("not a process id: %d", pid)
	}
	for _, s := range ParseWebViewSockets(table) {
		if s.PID == pid {
			return s, nil
		}
	}
	return WebViewSocket{}, fmt.Errorf("no WebView debugging socket was found for the app (process %d): the app may have no WebView on this screen, or WebView debugging is off in this build", pid)
}
