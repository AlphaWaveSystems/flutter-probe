package device

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AdbRunner runs one adb command line (the arguments after the adb binary) and returns its standard output. The real one is ADB.RunArgs; tests replace it.
type AdbRunner interface {
	RunArgs(ctx context.Context, args ...string) ([]byte, error)
}

// RunArgs runs adb with the given arguments exactly as written.
func (a *ADB) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	return a.run(ctx, args...)
}

var webViewSocketName = regexp.MustCompile(`^` + webViewSocketPrefix + `[1-9][0-9]{0,7}$`)

// removeTimeout bounds the removal of a forward, which must still run after the caller's context ended.
const removeTimeout = 5 * time.Second

// WebViewForward is one adb forward from a loopback port on this machine to the DevTools socket of one app's WebView. It owns exactly that rule: Close removes the port it created and nothing else.
// Every adb call it makes names the device with -s, and none ever removes rules it did not create (no --remove-all).
type WebViewForward struct {
	runner AdbRunner
	serial string
	port   int

	mu      sync.Mutex
	removed bool // set only when the removal succeeded, so a failed removal can be tried again
}

// maxForwardAttempts is how many probe-chosen ports are tried when adb says the port is already in use.
const maxForwardAttempts = 5

// ForwardWebView creates `adb -s <serial> forward tcp:0 localabstract:<socket>`. adb prints the port it allocated (adb 1.0.41 does). When it prints nothing usable, the forward it created exists all the same,
// so the device's forward list is read before and after: if exactly one new rule points at this socket, that rule is ours and its port is used (nothing is created twice); if it cannot be told which one is ours,
// probe forwards a free loopback port chosen and checked here with --no-rebind (a port taken in between is refused by adb, never silently rebound over another tool's rule), trying another port when adb says the first is in use (adb 1.0.41 says "cannot bind listener: Address already in use"), and leaves the rules it cannot identify alone. The port is recorded either way.
// socket must be a webview_devtools_remote_<pid> name.
func ForwardWebView(ctx context.Context, runner AdbRunner, serial string, socket WebViewSocket) (*WebViewForward, error) {
	if strings.TrimSpace(serial) == "" {
		return nil, fmt.Errorf("webview forward: a device serial is required")
	}
	if !webViewSocketName.MatchString(socket.Name) {
		return nil, fmt.Errorf("webview forward: not a WebView DevTools socket name")
	}
	target := "localabstract:" + socket.Name
	before := listForwardPorts(ctx, runner, serial, socket.Name)
	out, err := runner.RunArgs(ctx, "-s", serial, "forward", "tcp:0", target)
	if err != nil {
		return nil, fmt.Errorf("adb forward for the WebView socket failed on %s: %w", CleanOneLine(serial, 80), err)
	}
	port, perr := strconv.Atoi(strings.TrimSpace(string(out)))
	if perr == nil && port > 0 && port < 65536 {
		return &WebViewForward{runner: runner, serial: serial, port: port}, nil
	}
	// adb printed no usable port: find the rule it created
	if before != nil {
		after := listForwardPorts(ctx, runner, serial, socket.Name)
		var created []int
		for p := range after {
			if !before[p] {
				created = append(created, p)
			}
		}
		if len(created) == 1 {
			return &WebViewForward{runner: runner, serial: serial, port: created[0]}, nil
		}
	}
	// the fallback: a port chosen here and checked free
	var last error
	for attempt := 0; attempt < maxForwardAttempts; attempt++ {
		free, err := freeLoopbackPort()
		if err != nil {
			return nil, fmt.Errorf("adb did not print the forwarded port and no free local port was found: %w", err)
		}
		if _, err := runner.RunArgs(ctx, "-s", serial, "forward", "--no-rebind", "tcp:"+strconv.Itoa(free), target); err != nil {
			last = err
			if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
				continue
			}
			return nil, fmt.Errorf("adb forward tcp:%d for the WebView socket failed on %s: %w", free, CleanOneLine(serial, 80), err)
		}
		return &WebViewForward{runner: runner, serial: serial, port: free}, nil
	}
	return nil, fmt.Errorf("adb forward for the WebView socket failed on %s after %d ports were in use: %w", CleanOneLine(serial, 80), maxForwardAttempts, last)
}

// ParseForwardList reads the output of `adb forward --list` ("<serial> tcp:<port> localabstract:<name>" per line) and returns the host ports of the rules of that serial that point at that socket. Other
// lines, other serials, other targets and ports that are not plain decimal numbers are ignored.
func ParseForwardList(out, serial, socketName string) []int {
	var ports []int
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != serial || f[2] != "localabstract:"+socketName || !strings.HasPrefix(f[1], "tcp:") {
			continue
		}
		digits := strings.TrimPrefix(f[1], "tcp:")
		p, err := strconv.Atoi(digits)
		if err != nil || p <= 0 || p >= 65536 || strconv.Itoa(p) != digits {
			continue
		}
		ports = append(ports, p)
	}
	return ports
}

// listForwardPorts returns the set of host ports of the rules that point at the socket on this device, or nil when the list could not be read (an empty, readable list is an empty non-nil set).
func listForwardPorts(ctx context.Context, runner AdbRunner, serial, socketName string) map[int]bool {
	out, err := runner.RunArgs(ctx, "-s", serial, "forward", "--list")
	if err != nil {
		return nil
	}
	set := map[int]bool{}
	for _, p := range ParseForwardList(string(out), serial, socketName) {
		set[p] = true
	}
	return set
}

// Port is the loopback port the forward listens on (127.0.0.1).
func (f *WebViewForward) Port() int { return f.port }

// Close removes the forward this value created. It is safe to call from several goroutines and more than once: the removal runs until it has succeeded, so a removal that failed is tried again by the next
// Close, and a removal that succeeded is not repeated. It uses its own short deadline, so it still runs after the caller's context was cancelled or timed out.
func (f *WebViewForward) Close() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), removeTimeout)
	defer cancel()
	if _, err := f.runner.RunArgs(ctx, "-s", f.serial, "forward", "--remove", "tcp:"+strconv.Itoa(f.port)); err != nil {
		return fmt.Errorf("adb forward --remove tcp:%d failed on %s: %w", f.port, CleanOneLine(f.serial, 80), err)
	}
	f.removed = true
	return nil
}

// WithWebViewForward creates the forward, runs fn with its port and removes the forward when fn returns, whether fn succeeded, failed, timed out or was cancelled. The removal error is joined to fn's
// error rather than hiding it.
func WithWebViewForward(ctx context.Context, runner AdbRunner, serial string, socket WebViewSocket, fn func(port int) error) (err error) {
	f, err := ForwardWebView(ctx, runner, serial, socket)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			if err == nil {
				err = cerr
			} else {
				err = fmt.Errorf("%w; also: %v", err, cerr)
			}
		}
	}()
	return fn(f.Port())
}

func freeLoopbackPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// forwardHintTimeout bounds the one adb call of ForwardHint.
const forwardHintTimeout = 5 * time.Second

// ForwardHint says, for a failed connect on Android, that the agent port is forwarded to a DIFFERENT device, naming it and the command that removes the forward.
// It speaks only when `adb forward --list` shows this exact host port mapped to another plain serial; with no mapping, an unreadable list or this device's own
// forward it returns "" (never a guess, never a pid). One read-only adb call, bounded by forwardHintTimeout and by ctx. The result starts with " — " to be
// appended to an error.
func ForwardHint(ctx context.Context, runner AdbRunner, hostPort int, ownSerial string) string {
	if runner == nil || ctx.Err() != nil {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, forwardHintTimeout)
	defer cancel()
	other, ok := ForwardSerial(cctx, runner, hostPort)
	if !ok || other == ownSerial {
		return ""
	}
	return fmt.Sprintf(" — agent port %d is forwarded to another device (%s): remove that forward with `adb -s %s forward --remove tcp:%d`, or pick another port with --agent-port", hostPort, other, other, hostPort)
}

var forwardSerialRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)

// ForwardSerial reports which device the `adb forward` for a host port belongs to, by reading `adb forward --list` (lines of "<serial> tcp:<host> tcp:<device>").
// found is false when no forward uses the port or the list cannot be read. Read-only; it never changes or removes a forward.
func ForwardSerial(ctx context.Context, runner AdbRunner, hostPort int) (serial string, found bool) {
	out, err := runner.RunArgs(ctx, "forward", "--list")
	if err != nil || len(out) > 1<<20 {
		return "", false
	}
	rule := "tcp:" + strconv.Itoa(hostPort)
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[1] == rule {
			// the serial goes into a message and a command line: only what a serial can be (a letter or digit first, then letters, digits and . _ : -), at most 64, or nothing
			if !forwardSerialRE.MatchString(f[0]) {
				return "", false
			}
			return f[0], true
		}
	}
	return "", false
}
