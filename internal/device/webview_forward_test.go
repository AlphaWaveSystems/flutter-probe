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

	lists   []string // the answers to successive `forward --list` calls; "" after they run out
	listErr error
}

func (f *fakeAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.ctxDone = append(f.ctxDone, ctx.Err() != nil)
	if len(args) >= 4 && args[2] == "forward" && args[3] == "--list" {
		var out string
		if len(f.lists) > 0 {
			out, f.lists = f.lists[0], f.lists[1:]
		}
		err := f.listErr
		f.mu.Unlock()
		return []byte(out), err
	}
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
	if strings.Join(f.calls[1], " ") != strings.Join(want, " ") {
		t.Fatalf("call = %v, want %v", f.calls[1], want)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err != nil { // second close does nothing
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %v, want a list, one forward and one removal", f.calls)
	}
	wantRm := []string{"-s", "emulator-5600", "forward", "--remove", "tcp:62395"}
	if strings.Join(f.calls[2], " ") != strings.Join(wantRm, " ") {
		t.Fatalf("removal = %v, want %v", f.calls[2], wantRm)
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
		if len(f.calls) != 4 || f.calls[3][3] != "--no-rebind" || f.calls[3][4] != "tcp:"+strconv.Itoa(fw.Port()) {
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
	if len(f.calls) != 2 {
		t.Fatalf("calls = %v, want a list and the failed forward", f.calls)
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
		if len(f.calls) != 3 || f.calls[2][3] != "--remove" || f.calls[2][4] != "tcp:62395" {
			t.Errorf("%s: calls = %v, want a list, a forward and the removal of tcp:62395", c.name, f.calls)
		}
		if c.name == "cancel" || c.name == "timeout" {
			if f.ctxDone[2] {
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

func TestParseForwardList(t *testing.T) {
	const sock = "webview_devtools_remote_2783"
	list := strings.Join([]string{
		"emulator-5600 tcp:62395 localabstract:" + sock,
		"emulator-5600 tcp:62396 localabstract:webview_devtools_remote_999",
		"emulator-5554 tcp:62397 localabstract:" + sock,
		"emulator-5600 tcp:48692 tcp:48686",
		"emulator-5600 tcp:abc localabstract:" + sock,
		"emulator-5600 tcp:0 localabstract:" + sock,
		"emulator-5600 tcp:070 localabstract:" + sock,
		"emulator-5600 tcp:99999 localabstract:" + sock,
		"emulator-5600 tcp:-1 localabstract:" + sock,
		"emulator-5600 udp:5 localabstract:" + sock,
		"emulator-5600 tcp:62398 localabstract:" + sock + " extra",
		"emulator-5600 tcp:62399 localabstract:" + sock,
		"",
		"garbage",
	}, "\n")
	got := ParseForwardList(list, "emulator-5600", sock)
	if len(got) != 2 || got[0] != 62395 || got[1] != 62399 {
		t.Fatalf("ports = %v, want [62395 62399]", got)
	}
	if len(ParseForwardList("", "emulator-5600", sock)) != 0 {
		t.Fatal("an empty list gave ports")
	}
}

func TestForwardWebViewAdoptsTheRuleAdbCreatedWhenItPrintsNoPort(t *testing.T) {
	const rule = "emulator-5600 tcp:50001 localabstract:webview_devtools_remote_2783"
	f := &fakeAdb{
		forward: func(args []string) ([]byte, error) { return []byte("\n"), nil },
		lists:   []string{"", rule},
	}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil {
		t.Fatal(err)
	}
	if fw.Port() != 50001 {
		t.Fatalf("port = %d, want the created rule's 50001", fw.Port())
	}
	for _, c := range f.calls {
		if len(c) > 3 && c[2] == "forward" && c[3] != "tcp:0" && c[3] != "--list" {
			t.Fatalf("a second forward was created: %v", c)
		}
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	last := f.calls[len(f.calls)-1]
	if strings.Join(last, " ") != "-s emulator-5600 forward --remove tcp:50001" {
		t.Fatalf("removal = %v", last)
	}
	f.assertSafe(t)
}

func TestForwardWebViewAdoptsOnlyTheNewRuleNotAnExistingOne(t *testing.T) {
	const other = "emulator-5600 tcp:40000 localabstract:webview_devtools_remote_2783" // someone else's rule to the same socket
	const mine = "emulator-5600 tcp:50001 localabstract:webview_devtools_remote_2783"
	f := &fakeAdb{
		forward: func(args []string) ([]byte, error) { return nil, nil },
		lists:   []string{other, other + "\n" + mine},
	}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil || fw.Port() != 50001 {
		t.Fatalf("port = %v, %v", fw, err)
	}
	_ = fw.Close()
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), "tcp:40000") {
			t.Fatalf("a rule that was not ours was touched: %v", c)
		}
	}
}

func TestForwardWebViewFallsBackWhenTheNewRuleCannotBeIdentified(t *testing.T) {
	r := func(p int) string {
		return "emulator-5600 tcp:" + strconv.Itoa(p) + " localabstract:webview_devtools_remote_2783"
	}
	cases := map[string]*fakeAdb{
		"two new rules": {forward: func([]string) ([]byte, error) { return nil, nil }, lists: []string{"", r(50001) + "\n" + r(50002)}},
		"no new rule":   {forward: func([]string) ([]byte, error) { return nil, nil }, lists: []string{"", ""}},
		"list failed":   {forward: func([]string) ([]byte, error) { return nil, nil }, listErr: errors.New("adb gone")},
	}
	for name, f := range cases {
		fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if fw.Port() == 50001 || fw.Port() == 50002 {
			t.Fatalf("%s: a rule that could not be identified was adopted: %d", name, fw.Port())
		}
		_ = fw.Close()
		// nothing but our own chosen port was ever removed
		for _, c := range f.calls {
			if len(c) > 4 && c[3] == "--remove" && c[4] != "tcp:"+strconv.Itoa(fw.Port()) {
				t.Fatalf("%s: removed %v", name, c)
			}
		}
		f.assertSafe(t)
	}
}

func TestForwardWebViewRetriesWhenThePortIsInUse(t *testing.T) {
	attempts := 0
	f := &fakeAdb{forward: func(args []string) ([]byte, error) {
		if args[3] == "tcp:0" {
			return nil, nil // prints no port
		}
		attempts++
		if attempts < 3 {
			return nil, errors.New("adb: error: cannot bind listener: Address already in use")
		}
		return nil, nil
	}}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil || attempts != 3 {
		t.Fatalf("attempts = %d, err = %v", attempts, err)
	}
	_ = fw.Close()

	// every port in use: a clear error after the limit
	attempts = 0
	f2 := &fakeAdb{forward: func(args []string) ([]byte, error) {
		if args[3] == "tcp:0" {
			return nil, nil
		}
		attempts++
		return nil, errors.New("address already in use")
	}}
	if _, err := ForwardWebView(context.Background(), f2, "emulator-5600", ownSocket); err == nil || attempts != maxForwardAttempts {
		t.Fatalf("attempts = %d, err = %v", attempts, err)
	}
	// another failure is not retried
	attempts = 0
	f3 := &fakeAdb{forward: func(args []string) ([]byte, error) {
		if args[3] == "tcp:0" {
			return nil, nil
		}
		attempts++
		return nil, errors.New("device offline")
	}}
	if _, err := ForwardWebView(context.Background(), f3, "emulator-5600", ownSocket); err == nil || attempts != 1 {
		t.Fatalf("attempts = %d, err = %v", attempts, err)
	}
}

func TestCloseIsSafeFromSeveralGoroutinesAndRunsTheRemovalOnce(t *testing.T) {
	f := &fakeAdb{}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = fw.Close() }()
	}
	wg.Wait()
	removals := 0
	for _, c := range f.calls {
		if len(c) > 3 && c[3] == "--remove" {
			removals++
		}
	}
	if removals != 1 {
		t.Fatalf("the forward was removed %d times", removals)
	}
}

func TestFailedRemovalIsRetriedByTheNextClose(t *testing.T) {
	failures := 1
	f := &fakeAdb{remove: func(context.Context, []string) error {
		if failures > 0 {
			failures--
			return errors.New("adb busy")
		}
		return nil
	}}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil {
		t.Fatal(err)
	}
	if err := fw.Close(); err == nil {
		t.Fatal("the first Close should report the failed removal")
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("the second Close did not retry: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	removals := 0
	for _, c := range f.calls {
		if len(c) > 3 && c[3] == "--remove" {
			removals++
		}
	}
	if removals != 2 {
		t.Fatalf("removals = %d, want the failed one and the retry", removals)
	}
}

func TestFallbackForwardUsesNoRebind(t *testing.T) {
	f := &fakeAdb{forward: func(args []string) ([]byte, error) { return nil, nil }}
	fw, err := ForwardWebView(context.Background(), f, "emulator-5600", ownSocket)
	if err != nil {
		t.Fatal(err)
	}
	var chosen []string
	for _, c := range f.calls {
		if len(c) > 4 && c[2] == "forward" && strings.HasPrefix(c[4], "tcp:") && c[4] != "tcp:0" {
			chosen = c
		}
	}
	want := []string{"-s", "emulator-5600", "forward", "--no-rebind", "tcp:" + strconv.Itoa(fw.Port()), "localabstract:webview_devtools_remote_2783"}
	if strings.Join(chosen, " ") != strings.Join(want, " ") {
		t.Fatalf("the chosen-port forward = %v, want %v", chosen, want)
	}
}

// ForwardSerial names the device a host port is forwarded for, and says nothing when it cannot tell.
func TestForwardSerial(t *testing.T) {
	list := "emulator-5554 tcp:48792 tcp:48792\nemulator-5600 tcp:50000 tcp:50001\n0123456789ABCDEF tcp:50002 localabstract:x\nnot a line\nlonely\ntwo tcp:1234\n\n"
	for port, want := range map[int]string{48792: "emulator-5554", 50000: "emulator-5600", 50002: "0123456789ABCDEF"} {
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(list), port); !ok || got != want {
			t.Errorf("port %d: %q %v, want %q", port, got, ok, want)
		}
	}
	for name, c := range map[string]struct {
		out  string
		port int
	}{"no forward for the port": {list, 1234}, "an empty list": {"", 48792}, "a prefix of a port is not the port": {list, 4879}} {
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(c.out), c.port); ok || got != "" {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	if _, ok := ForwardSerial(context.Background(), failingRunner{}, 48792); ok {
		t.Error("an adb that failed gave an answer")
	}
	// an adb that fails but printed something is not believed either
	if _, ok := ForwardSerial(context.Background(), partialRunner(list), 48792); ok {
		t.Error("an adb that failed after printing gave an answer")
	}
	// only the listing is asked for: nothing that changes a forward
	rec := &listRecorder{out: list}
	ForwardSerial(context.Background(), rec, 48792)
	if len(rec.calls) != 1 || strings.Join(rec.calls[0], " ") != "forward --list" {
		t.Errorf("calls = %v", rec.calls)
	}
}

type forwardListRunner string

func (f forwardListRunner) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	return []byte(f), nil
}

type failingRunner struct{}

func (failingRunner) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	return nil, errors.New("adb: no devices")
}

type listRecorder struct {
	out   string
	calls [][]string
}

func (l *listRecorder) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	l.calls = append(l.calls, append([]string(nil), args...))
	return []byte(l.out), nil
}

type partialRunner string

func (p partialRunner) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	return []byte(p), errors.New("adb: connection lost")
}

// The serial ends up in a message and a command line: anything that is not a plain serial is not returned at all, so device text cannot reach either.
func TestForwardSerialRefusesAnythingThatIsNotAPlainSerial(t *testing.T) {
	for name, serial := range map[string]string{
		"escape sequence":  "emu\x1b[2Jlator",
		"shell characters": "a;rm",
		"a substitution":   "$(id)",
		"a quote":          "a'b",
		"bidi":             "a\u202eb",
		"65 characters":    strings.Repeat("a", 65),
		"a slash":          "a/b",
	} {
		list := serial + " tcp:48792 tcp:48792\n"
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(list), 48792); ok || got != "" {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	for _, serial := range []string{"emulator-5554", "0123456789ABCDEF", "192.168.1.5:5555", "R58M12345AB", "adb-0123abcd._tcp", strings.Repeat("a", 64)} {
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(serial+" tcp:48792 tcp:48792\n"), 48792); !ok || got != serial {
			t.Errorf("%q refused: %q %v", serial, got, ok)
		}
	}
}

// A serial starts with a letter or a digit, so it can never read as an option of the command it is put into.
func TestForwardSerialCannotLookLikeAnOption(t *testing.T) {
	for _, serial := range []string{"--force", "-x", "-", ".x", ":5555", "_a"} {
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(serial+" tcp:48792 tcp:48792\n"), 48792); ok || got != "" {
			t.Errorf("%q: %q %v", serial, got, ok)
		}
	}
	for _, serial := range []string{"1abc", "a", "9", "x-y"} {
		if got, ok := ForwardSerial(context.Background(), forwardListRunner(serial+" tcp:48792 tcp:48792\n"), 48792); !ok || got != serial {
			t.Errorf("%q refused: %q %v", serial, got, ok)
		}
	}
}
