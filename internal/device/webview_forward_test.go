package device

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeAdb records every command line and answers from a script.
type fakeAdb struct {
	mu      sync.Mutex
	calls   [][]string
	forward func(args []string) ([]byte, error)
	remove  func(ctx context.Context, args []string) error
	ctxDone []bool // whether the context of each call was already done
}

func (f *fakeAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.ctxDone = append(f.ctxDone, ctx.Err() != nil)
	f.mu.Unlock()
	if len(args) >= 4 && args[2] == "forward" && args[3] == "--remove" {
		if f.remove != nil {
			return nil, f.remove(ctx, args)
		}
		return nil, nil
	}
	if f.forward != nil {
		return f.forward(args)
	}
	return []byte("62395\n"), nil
}

func (f *fakeAdb) assertSafe(t *testing.T) {
	t.Helper()
	for _, c := range f.calls {
		if len(c) < 2 || c[0] != "-s" || c[1] == "" {
			t.Errorf("adb call without -s <serial>: %v", c)
		}
		for _, a := range c {
			if a == "--remove-all" || strings.Contains(a, "remove-all") {
				t.Errorf("adb call removes all forwards: %v", c)
			}
		}
	}
}

var ownSocket = WebViewSocket{PID: 2783, Name: "webview_devtools_remote_2783"}

func TestForwardWebViewUsesPortAdbPrinted(t *testing.T) {
	f := &fakeAdb{}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil {
		t.Fatal(err)
	}
	if fw.Port() != 62395 {
		t.Fatalf("port = %d, want 62395", fw.Port())
	}
	want := []string{"-s", "emulator-5600", "forward", "tcp:0", "localabstract:webview_devtools_remote_2783"}
	if strings.Join(f.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("call = %v, want %v", f.calls[0], want)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil { // second close does nothing
		t.Fatal(err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %v, want one forward and one removal", f.calls)
	}
	wantRm := []string{"-s", "emulator-5600", "forward", "--remove", "tcp:62395"}
	if strings.Join(f.calls[1], " ") != strings.Join(wantRm, " ") {
		t.Fatalf("removal = %v, want %v", f.calls[1], wantRm)
	}
	f.assertSafe(t)
}

func TestForwardWebViewFallbackWhenAdbPrintsNoPort(t *testing.T) {
	for _, printed := range []string{"", "\n", "not a port", "0", "70000", "-1"} {
		f := &fakeAdb{forward: func(args []string) ([]byte, error) {
			if args[3] == "tcp:0" {
				return []byte(printed), nil
			}
			return nil, nil
		}}
		fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
		if err != nil {
			t.Fatalf("%q: %v", printed, err)
		}
		if fw.Port() <= 0 || fw.Port() >= 65536 {
			t.Fatalf("%q: port %d", printed, fw.Port())
		}
		if len(f.calls) != 2 || f.calls[1][3] != "tcp:"+strconv.Itoa(fw.Port()) {
			t.Fatalf("%q: fallback did not forward the chosen port: %v", printed, f.calls)
		}
		_ = fw.Close()
		last := f.calls[len(f.calls)-1]
		if last[3] != "--remove" || last[4] != "tcp:"+strconv.Itoa(fw.Port()) {
			t.Fatalf("%q: removal = %v", printed, last)
		}
		f.assertSafe(t)
	}
}

func TestForwardWebViewRejectsBadInput(t *testing.T) {
	f := &fakeAdb{}
	for _, serial := range []string{"", "  "} {
		if _, err := ForwardWebView(context.Background(), f, serial, ownSocket); err == nil {
			t.Errorf("serial %q accepted", serial)
		}
	}
	for _, name := range []string{"", "webview_devtools_remote_", "webview_devtools_remote_0", "webview_devtools_remote_12a", "chrome_devtools_remote", "webview_devtools_remote_1 ; rm", "localabstract:webview_devtools_remote_1"} {
		if _, err := ForwardWebView(context.Background(), f, "emulator-5600", WebViewSocket{Name: name}); err == nil {
			t.Errorf("socket name %q accepted", name)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("adb was called for rejected input: %v", f.calls)
	}
}

func TestForwardWebViewAdbFailureCreatesNothingToRemove(t *testing.T) {
	f := &fakeAdb{forward: func([]string) ([]byte, error) { return nil, errors.New("device offline") }}
	if _, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket); err == nil {
		t.Fatal("expected an error")
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %v", f.calls)
	}
}

func TestWithWebViewForwardRemovesOnEveryExit(t *testing.T) {
	boom := errors.New("step failed")
	cases := []struct {
		name string
		fn   func(ctx context.Context, cancel context.CancelFunc) func(port int) error
		want error
	}{
		{"success", func(context.Context, context.CancelFunc) func(int) error { return func(int) error { return nil } }, nil},
		{"failure", func(context.Context, context.CancelFunc) func(int) error { return func(int) error { return boom } }, boom},
		{"cancel", func(ctx context.Context, cancel context.CancelFunc) func(int) error {
			return func(int) error { cancel(); return ctx.Err() }
		}, context.Canceled},
		{"timeout", func(ctx context.Context, cancel context.CancelFunc) func(int) error {
			return func(int) error { <-ctx.Done(); return ctx.Err() }
		}, context.DeadlineExceeded},
	}
	for _, c := range cases {
		f := &fakeAdb{}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := WithWebViewForward(ctx, f, "emulator-5600", ownSocket, c.fn(ctx, cancel))
		cancel()
		if !errors.Is(err, c.want) && !(c.want == nil && err == nil) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if len(f.calls) != 2 || f.calls[1][3] != "--remove" || f.calls[1][4] != "tcp:62395" {
			t.Errorf("%s: calls = %v, want a forward then the removal of tcp:62395", c.name, f.calls)
		}
		if c.name == "cancel" || c.name == "timeout" {
			if f.ctxDone[1] {
				t.Errorf("%s: the removal ran with a finished context", c.name)
			}
		}
		f.assertSafe(t)
	}
}

func TestWithWebViewForwardReportsRemovalFailureWithoutHidingTheStepError(t *testing.T) {
	boom := errors.New("step failed")
	f := &fakeAdb{remove: func(context.Context, []string) error { return errors.New("adb gone") }}
	err := WithWebViewForward(context.Background(), f, "emulator-5600", ownSocket, func(int) error { return boom })
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "adb gone") {
		t.Fatalf("err = %v", err)
	}
	// with a successful step, a removal failure is the error
	err = WithWebViewForward(context.Background(), f, "emulator-5600", ownSocket, func(int) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "adb gone") {
		t.Fatalf("err = %v", err)
	}
	f.assertSafe(t)
}

func TestWithWebViewForwardDoesNotRunTheStepWhenForwardFails(t *testing.T) {
	f := &fakeAdb{forward: func([]string) ([]byte, error) { return nil, errors.New("no device") }}
	ran := false
	err := WithWebViewForward(context.Background(), f, "emulator-5600", ownSocket, func(int) error { ran = true; return nil })
	if err == nil || ran {
		t.Fatalf("err = %v, ran = %v", err, ran)
	}
}

func TestFreeLoopbackPortIsFree(t *testing.T) {
	p, err := freeLoopbackPort()
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("port %d was not free: %v", p, err)
	}
	l.Close()
}
