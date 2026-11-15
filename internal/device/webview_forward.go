package device

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
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
	closed bool
}

// ForwardWebView creates `adb -s <serial> forward tcp:0 localabstract:<socket>`. adb prints the port it allocated (adb 1.0.41 does); when it prints nothing usable, probe picks a free loopback port
// itself, checks it is free, and forwards that one. The port is recorded either way. socket must be a webview_devtools_remote_<pid> name.
func ForwardWebView(ctx context.Context, runner AdbRunner, serial string, socket WebViewSocket) (*WebViewForward, error) {
	if strings.TrimSpace(serial) == "" {
		return nil, fmt.Errorf("webview forward: a device serial is required")
	}
	if !webViewSocketName.MatchString(socket.Name) {
		return nil, fmt.Errorf("webview forward: not a WebView DevTools socket name")
	}
	target := "localabstract:" + socket.Name
	out, err := runner.RunArgs(ctx, "-s", serial, "forward", "tcp:0", target)
	if err != nil {
		return nil, fmt.Errorf("adb forward for the WebView socket failed on %s: %w", serial, err)
	}
	port, perr := strconv.Atoi(strings.TrimSpace(string(out)))
	if perr == nil && port > 0 && port < 65536 {
		return &WebViewForward{runner: runner, serial: serial, port: port}, nil
	}
	// the fallback: a port chosen here and checked free
	free, err := freeLoopbackPort()
	if err != nil {
		return nil, fmt.Errorf("adb did not print the forwarded port and no free local port was found: %w", err)
	}
	if _, err := runner.RunArgs(ctx, "-s", serial, "forward", "tcp:"+strconv.Itoa(free), target); err != nil {
		return nil, fmt.Errorf("adb forward tcp:%d for the WebView socket failed on %s: %w", free, serial, err)
	}
	return &WebViewForward{runner: runner, serial: serial, port: free}, nil
}

// Port is the loopback port the forward listens on (127.0.0.1).
func (f *WebViewForward) Port() int { return f.port }

// Close removes the forward this value created. It is safe to call more than once. It uses its own short deadline, so it still runs after the caller's context was cancelled or timed out.
func (f *WebViewForward) Close() error {
	if f == nil || f.closed {
		return nil
	}
	f.closed = true
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), removeTimeout)
	defer cancel()
	if _, err := f.runner.RunArgs(ctx, "-s", f.serial, "forward", "--remove", "tcp:"+strconv.Itoa(f.port)); err != nil {
		return fmt.Errorf("adb forward --remove tcp:%d failed on %s: %w", f.port, f.serial, err)
	}
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
