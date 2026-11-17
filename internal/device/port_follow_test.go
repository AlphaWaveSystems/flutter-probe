package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// procRow is one row of /proc/net/tcp6 in the format of the real captures (testdata/portowner): a socket in LISTEN state ("0A") on the IPv4-mapped loopback, owned by uid.
// The captures hold one app that owns the port; the rows for the second app that moved on are made from the same format, because the capture apps could not be made to
// move (see the README there).
func procRow(port, uid int) string {
	return fmt.Sprintf("   0: 0000000000000000FFFF00000100007F:%04X 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 %5d        0 12345 1 0000000000000000 100 0 0 10 0\n", port, uid)
}

const procHeader = "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

func twoApps(heldBy, ownPort int) *poAdb {
	// uid 10100 = com.first (holds heldBy), uid 10200 = com.second (the app under test, listens on ownPort when ownPort != 0)
	tcp6 := procHeader + procRow(heldBy, 10100)
	if ownPort != 0 {
		tcp6 += procRow(ownPort, 10200)
	}
	return &poAdb{tcp6: tcp6, tcp: procHeader, pm: "package:com.first uid:10100\npackage:com.second uid:10200\npackage:android uid:1000\n"}
}

type fwdCall struct {
	serial         string
	host, devicePt int
}

type fakeFwd struct {
	calls []fwdCall
	err   error
}

func (f *fakeFwd) ForwardPort(ctx context.Context, serial string, hostPort, devicePort int) error {
	f.calls = append(f.calls, fwdCall{serial, hostPort, devicePort})
	return f.err
}

func TestDiscoverAppPort(t *testing.T) {
	cases := []struct {
		name   string
		adb    *poAdb
		app    string
		want   int
		wantOK bool
	}{
		{"the app moved to the next port", twoApps(48911, 48912), "com.second", 48912, true},
		{"the app moved three ports on", twoApps(48911, 48914), "com.second", 48914, true},
		{"the app holds the configured port itself", twoApps(48911, 0), "com.first", 48911, true},
		{"the app listens nowhere", twoApps(48911, 0), "com.second", 0, false},
		{"the port is out of the range", twoApps(48911, 48921), "com.second", 0, false},
		{"another app holds the next port: not the app's", &poAdb{tcp6: procHeader + procRow(48911, 10100) + procRow(48912, 10100), tcp: procHeader, pm: "package:com.first uid:10100\npackage:com.second uid:10200\n"}, "com.second", 0, false},
		{"the app is not installed", twoApps(48911, 48912), "com.third", 0, false},
		{"two packages share the uid: nothing proves the app", &poAdb{tcp6: procHeader + procRow(48912, 10200), tcp: procHeader, pm: "package:com.second uid:10200\npackage:com.sibling uid:10200\n"}, "com.second", 0, false},
	}
	for _, tc := range cases {
		got, ok, why := DiscoverAppPort(context.Background(), tc.adb, "emulator-5554", 48911, tc.app)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("%s: got (%d, %v, %q), want (%d, %v)", tc.name, got, ok, why, tc.want, tc.wantOK)
		}
	}
}

func TestDiscoverAppPortSeveralPorts(t *testing.T) {
	two := &poAdb{tcp6: procHeader + procRow(48912, 10200) + procRow(48913, 10200), tcp: procHeader, pm: "package:com.second uid:10200\n"}
	if p, ok, _ := DiscoverAppPort(context.Background(), two, "emulator-5554", 48911, "com.second"); ok {
		t.Errorf("two ports of the app without the configured one followed %d, want no answer", p)
	}
	withConfigured := &poAdb{tcp6: procHeader + procRow(48911, 10200) + procRow(48913, 10200), tcp: procHeader, pm: "package:com.second uid:10200\n"}
	if p, ok, _ := DiscoverAppPort(context.Background(), withConfigured, "emulator-5554", 48911, "com.second"); !ok || p != 48911 {
		t.Errorf("with the configured port among them: got (%d, %v), want (48911, true)", p, ok)
	}
}

func TestDiscoverAppPortReadsBothTables(t *testing.T) {
	// the app listens in tcp (IPv4) rather than tcp6
	adb := &poAdb{tcp6: procHeader, tcp: procHeader + "   0: 0100007F:BF10 00000000:0000 0A 00000000:00000000 00:00000000 00000000 10200        0 1 1 0000000000000000 100 0 0 10 0\n", pm: "package:com.second uid:10200\n"}
	if p, ok, _ := DiscoverAppPort(context.Background(), adb, "emulator-5554", 48911, "com.second"); !ok || p != 0xBF10 {
		t.Errorf("tcp table: got (%d, %v), want (%d, true)", p, ok, 0xBF10)
	}
}

func TestDiscoverAppPortOnRealCaptures(t *testing.T) {
	// on the real captures the first app holds 48911; asked for the first app it stays, asked for the second (installed, not listening) it finds nothing
	for _, api := range []string{"api34", "api35"} {
		adb := capturedDevice(t, api)
		if p, ok, why := DiscoverAppPort(context.Background(), adb, "emulator-5554", 48911, "dev.probe.portspikea"); !ok || p != 48911 {
			t.Errorf("%s: first app: got (%d, %v, %q), want (48911, true)", api, p, ok, why)
		}
		if p, ok, _ := DiscoverAppPort(context.Background(), adb, "emulator-5554", 48911, "dev.probe.portspikeb"); ok {
			t.Errorf("%s: second app: followed %d, want no answer", api, p)
		}
	}
}

// A device on adb over Wi-Fi has a serial like 192.168.1.5:5555; it is a serial like any other for this lookup.
func TestDiscoverAppPortOverNetworkAdb(t *testing.T) {
	if p, ok, why := DiscoverAppPort(context.Background(), twoApps(48911, 48912), "192.168.1.5:5555", 48911, "com.second"); !ok || p != 48912 {
		t.Errorf("network serial: got (%d, %v, %q), want (48912, true)", p, ok, why)
	}
}

func TestDiscoverAppPortRefusesWhatItCannotLookAt(t *testing.T) {
	adb := twoApps(48911, 48912)
	for _, tc := range []struct {
		name, serial, app string
		port              int
	}{
		{"an option as serial", "-e", "com.second", 48911},
		{"an app id with a shell character", "emulator-5554", "com.second;id", 48911},
		{"port zero", "emulator-5554", "com.second", 0},
		{"a port near the top", "emulator-5554", "com.second", 65530},
	} {
		adb.calls = nil
		if _, ok, _ := DiscoverAppPort(context.Background(), adb, tc.serial, tc.port, tc.app); ok || len(adb.calls) != 0 {
			t.Errorf("%s: ok=%v with %d adb call(s), want a refusal before any call", tc.name, ok, len(adb.calls))
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, _ := DiscoverAppPort(cancelled, adb, "emulator-5554", 48911, "com.second"); ok || len(adb.calls) != 0 {
		t.Errorf("a cancelled context must stop before any adb call")
	}
	adb.fail = map[string]bool{"pm": true}
	if _, ok, _ := DiscoverAppPort(context.Background(), adb, "emulator-5554", 48911, "com.second"); ok {
		t.Errorf("an unreadable package list must give no answer")
	}
}

func newTestFollower(adb AdbRunner, fwd PortForwarder, notices *[]string, changes *[]int) *PortFollower {
	f := NewPortFollower(adb, fwd, "emulator-5554", "com.second", 48686, 48911,
		func(s string) { *notices = append(*notices, s) }, func(p int) { *changes = append(*changes, p) })
	return f
}

// The happy path: building a follower, asking for its port and a preflight after a run-as token make no adb call at all.
func TestPortFollowerHappyPathMakesNoAdbCall(t *testing.T) {
	adb := twoApps(48911, 0)
	fwd := &fakeFwd{}
	var notices []string
	var changes []int
	f := newTestFollower(adb, fwd, &notices, &changes)
	if f.DevicePort() != 48911 {
		t.Fatalf("DevicePort = %d, want the configured 48911", f.DevicePort())
	}
	f.NoteTokenSource(TokenSourceRunAs)
	if moved, err := f.Preflight(context.Background(), time.Minute); moved || err != nil {
		t.Errorf("a run-as token proves the app: no preflight move and no refusal (got %v, %v)", moved, err)
	}
	if len(adb.calls) != 0 || len(fwd.calls) != 0 || len(notices) != 0 || len(changes) != 0 {
		t.Errorf("the happy path made %d adb call(s), %d forward(s), %d notice(s), %d change(s); want none", len(adb.calls), len(fwd.calls), len(notices), len(changes))
	}
}

func TestPortFollowerSuspectMovesTheForwardToTheAppsPort(t *testing.T) {
	adb := twoApps(48911, 48912)
	fwd := &fakeFwd{}
	var notices []string
	var changes []int
	f := newTestFollower(adb, fwd, &notices, &changes)
	if !f.Suspect(context.Background()) {
		t.Fatalf("Suspect did not move the forward")
	}
	if len(fwd.calls) != 1 || fwd.calls[0] != (fwdCall{"emulator-5554", 48686, 48912}) {
		t.Errorf("forward calls = %+v, want one: host 48686 -> device 48912", fwd.calls)
	}
	if f.DevicePort() != 48912 || len(changes) != 1 || changes[0] != 48912 {
		t.Errorf("DevicePort = %d, changes = %v, want 48912 and [48912]", f.DevicePort(), changes)
	}
	want := "agent port 48911 on the device is held by com.first; using 48912 for com.second"
	if len(notices) != 1 || notices[0] != want {
		t.Errorf("notices = %q, want [%q]", notices, want)
	}
}

func TestPortFollowerSuspectIsRateLimited(t *testing.T) {
	adb := twoApps(48911, 0) // the app is nowhere: nothing to follow
	fwd := &fakeFwd{}
	var notices []string
	var changes []int
	f := newTestFollower(adb, fwd, &notices, &changes)
	clock := time.Unix(1000, 0)
	f.now = func() time.Time { return clock }
	f.Suspect(context.Background())
	first := len(adb.calls)
	if first == 0 {
		t.Fatalf("the first Suspect made no lookup")
	}
	clock = clock.Add(portFollowInterval - time.Millisecond)
	f.Suspect(context.Background())
	if len(adb.calls) != first {
		t.Errorf("a second Suspect inside the interval made %d more adb call(s)", len(adb.calls)-first)
	}
	clock = clock.Add(2 * time.Millisecond)
	f.Suspect(context.Background())
	if len(adb.calls) == first {
		t.Errorf("a Suspect after the interval made no lookup")
	}
	if len(fwd.calls) != 0 || f.DevicePort() != 48911 {
		t.Errorf("nothing found: the forward must stay (calls %v, port %d)", fwd.calls, f.DevicePort())
	}
}

func TestPortFollowerKeepsThePortWhenTheForwardFails(t *testing.T) {
	adb := twoApps(48911, 48912)
	fwd := &fakeFwd{err: errors.New("adb: forward failed")}
	var notices []string
	var changes []int
	f := newTestFollower(adb, fwd, &notices, &changes)
	if f.Suspect(context.Background()) {
		t.Errorf("Suspect reported a move although the forward failed")
	}
	if f.DevicePort() != 48911 || len(notices) != 0 || len(changes) != 0 {
		t.Errorf("a failed forward changed state: port %d, notices %q, changes %v", f.DevicePort(), notices, changes)
	}
}

// fakeTime gives a follower a clock that only moves when Preflight sleeps, so the retry window is exercised without waiting. onSleep, when set, runs at every sleep
// with the number of sleeps so far, to change what the device shows while the window lasts.
func fakeTime(f *PortFollower, onSleep func(n int)) *int {
	clock := time.Unix(5000, 0)
	sleeps := 0
	f.now = func() time.Time { return clock }
	f.sleep = func(ctx context.Context, d time.Duration) {
		sleeps++
		clock = clock.Add(d)
		if onSleep != nil {
			onSleep(sleeps)
		}
	}
	return &sleeps
}

func TestPortFollowerPreflight(t *testing.T) {
	t.Run("a token that does not prove the app, another app holds the port, the app is on the next one: moves", func(t *testing.T) {
		adb := twoApps(48911, 48912)
		fwd := &fakeFwd{}
		var notices []string
		var changes []int
		f := newTestFollower(adb, fwd, &notices, &changes)
		f.NoteTokenSource(TokenSourceLogcat)
		if moved, err := f.Preflight(context.Background(), time.Minute); !moved || err != nil || f.DevicePort() != 48912 {
			t.Errorf("preflight did not move to 48912 (moved %v, err %v, port %d)", moved, err, f.DevicePort())
		}
	})
	t.Run("a token that does not prove the app, nobody else holds the port: stays", func(t *testing.T) {
		adb := twoApps(48911, 0)
		adb.tcp6 = procHeader + procRow(48911, 10200) // the app itself holds it
		fwd := &fakeFwd{}
		var notices []string
		var changes []int
		f := newTestFollower(adb, fwd, &notices, &changes)
		f.NoteTokenSource(TokenSourceTmpFile)
		if moved, err := f.Preflight(context.Background(), time.Minute); moved || err != nil || len(fwd.calls) != 0 || f.DevicePort() != 48911 {
			t.Errorf("moved or refused although the app holds its own port (%v, %v, forwards %v)", moved, err, fwd.calls)
		}
	})
	// The wrong-app case: the token is a shared one (the holder's, maybe), another app holds the port, and the app under test has opened none. Dialing could reach the
	// holder, so the connect is refused, after the whole dial window, naming the holder.
	t.Run("another app holds the port and the app has none: refused after the window, never dialed", func(t *testing.T) {
		for _, src := range []TokenSource{TokenSourceNone, TokenSourceTmpFile, TokenSourceLogcat} {
			adb := twoApps(48911, 0)
			fwd := &fakeFwd{}
			var notices []string
			var changes []int
			f := newTestFollower(adb, fwd, &notices, &changes)
			sleeps := fakeTime(f, nil)
			f.NoteTokenSource(src)
			moved, err := f.Preflight(context.Background(), 5*time.Second)
			var held *PortHeldError
			if moved || !errors.As(err, &held) {
				t.Fatalf("source %v: got (%v, %v), want a *PortHeldError", src, moved, err)
			}
			if held.Port != 48911 || len(held.Holders) != 1 || held.Holders[0] != "com.first" || held.App != "com.second" {
				t.Errorf("source %v: error fields = %+v", src, *held)
			}
			for _, want := range []string{"agent port 48911", "com.first", "com.second has not opened an agent port", "not connecting"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("source %v: message lacks %q: %s", src, want, err)
				}
			}
			if *sleeps != 5 {
				t.Errorf("source %v: waited %d time(s), want 5 (a second each for the 5 s window)", src, *sleeps)
			}
			if len(fwd.calls) != 0 || f.DevicePort() != 48911 || len(notices) != 0 {
				t.Errorf("source %v: the forward or the port changed (%v, %d, %q)", src, fwd.calls, f.DevicePort(), notices)
			}
		}
	})
	t.Run("the app is still starting: it opens its port during the window and the run follows it", func(t *testing.T) {
		adb := twoApps(48911, 0)
		fwd := &fakeFwd{}
		var notices []string
		var changes []int
		f := newTestFollower(adb, fwd, &notices, &changes)
		sleeps := fakeTime(f, func(n int) {
			if n == 2 {
				adb.tcp6 = procHeader + procRow(48911, 10100) + procRow(48912, 10200)
			}
		})
		f.NoteTokenSource(TokenSourceLogcat)
		moved, err := f.Preflight(context.Background(), 30*time.Second)
		if !moved || err != nil || f.DevicePort() != 48912 || *sleeps != 2 {
			t.Errorf("got (%v, %v), port %d after %d wait(s), want (true, nil), 48912 after 2", moved, err, f.DevicePort(), *sleeps)
		}
	})
	t.Run("the holder leaves during the window: nobody else holds the port, no refusal", func(t *testing.T) {
		adb := twoApps(48911, 0)
		fwd := &fakeFwd{}
		var notices []string
		var changes []int
		f := newTestFollower(adb, fwd, &notices, &changes)
		fakeTime(f, func(n int) { adb.tcp6 = procHeader })
		f.NoteTokenSource(TokenSourceLogcat)
		if moved, err := f.Preflight(context.Background(), 30*time.Second); moved || err != nil {
			t.Errorf("got (%v, %v), want (false, nil)", moved, err)
		}
	})
	t.Run("no window: one look, then the refusal", func(t *testing.T) {
		adb := twoApps(48911, 0)
		var notices []string
		var changes []int
		f := newTestFollower(adb, &fakeFwd{}, &notices, &changes)
		sleeps := fakeTime(f, nil)
		f.NoteTokenSource(TokenSourceLogcat)
		if _, err := f.Preflight(context.Background(), 0); err == nil || *sleeps != 0 {
			t.Errorf("err %v after %d wait(s), want a refusal without waiting", err, *sleeps)
		}
	})
	t.Run("a cancelled run stops waiting", func(t *testing.T) {
		adb := twoApps(48911, 0)
		var notices []string
		var changes []int
		f := newTestFollower(adb, &fakeFwd{}, &notices, &changes)
		ctx, cancel := context.WithCancel(context.Background())
		sleeps := fakeTime(f, func(n int) { cancel() })
		f.NoteTokenSource(TokenSourceLogcat)
		if _, err := f.Preflight(ctx, time.Hour); !errors.Is(err, context.Canceled) || *sleeps != 1 {
			t.Errorf("err %v after %d wait(s), want context.Canceled after the first", err, *sleeps)
		}
	})
	// The combination that stays unprotected, and the docs say so: when /proc/net cannot be read nobody can say who holds the port, so nothing is refused.
	t.Run("the socket table cannot be read: nothing is known, nothing is refused", func(t *testing.T) {
		adb := twoApps(48911, 0)
		adb.fail = map[string]bool{"tcp6": true, "tcp": true}
		var notices []string
		var changes []int
		f := newTestFollower(adb, &fakeFwd{}, &notices, &changes)
		f.NoteTokenSource(TokenSourceLogcat)
		if moved, err := f.Preflight(context.Background(), time.Minute); moved || err != nil {
			t.Errorf("got (%v, %v), want (false, nil)", moved, err)
		}
	})
}

// Two connections of one run (the lanes of a parallel run) each have their own follower: moving one does not move the other.
func TestPortFollowersShareNothing(t *testing.T) {
	var n1, n2 []string
	var c1, c2 []int
	fwd1, fwd2 := &fakeFwd{}, &fakeFwd{}
	a := newTestFollower(twoApps(48911, 48912), fwd1, &n1, &c1)
	b := newTestFollower(twoApps(48911, 0), fwd2, &n2, &c2)
	a.Suspect(context.Background())
	b.Suspect(context.Background())
	if a.DevicePort() != 48912 || b.DevicePort() != 48911 {
		t.Errorf("ports = %d and %d, want 48912 and 48911", a.DevicePort(), b.DevicePort())
	}
	if len(fwd2.calls) != 0 || len(n2) != 0 {
		t.Errorf("the second connection was touched by the first: %v %q", fwd2.calls, n2)
	}
}

func TestPortFollowerFollowsBackWhenTheConfiguredPortIsTheAppsAgain(t *testing.T) {
	adb := twoApps(48911, 48912)
	fwd := &fakeFwd{}
	var notices []string
	var changes []int
	f := newTestFollower(adb, fwd, &notices, &changes)
	clock := time.Unix(1000, 0)
	f.now = func() time.Time { return clock }
	f.Suspect(context.Background())
	// the first app stops and the second app, restarted, takes the configured port
	adb.tcp6 = procHeader + procRow(48911, 10200)
	clock = clock.Add(time.Minute)
	if !f.Suspect(context.Background()) || f.DevicePort() != 48911 {
		t.Errorf("did not follow back to 48911 (port %d)", f.DevicePort())
	}
	if !strings.Contains(strings.Join(notices, "\n"), "using 48911") {
		t.Errorf("notices = %q, want a line for the move back", notices)
	}
}

// Without an app id nothing can be looked up, so a preflight with a token that proves nothing still makes no adb call.
func TestPortFollowerPreflightWithoutAnAppMakesNoCall(t *testing.T) {
	adb := twoApps(48911, 48912)
	fwd := &fakeFwd{}
	f := NewPortFollower(adb, fwd, "emulator-5554", "", 48686, 48911, nil, nil)
	f.NoteTokenSource(TokenSourceLogcat)
	if moved, err := f.Preflight(context.Background(), time.Minute); moved || err != nil || f.Suspect(context.Background()) {
		t.Errorf("moved or refused without an app id (%v, %v)", moved, err)
	}
	if len(adb.calls) != 0 || len(fwd.calls) != 0 {
		t.Errorf("made %d adb call(s) and %d forward(s) without an app id, want none", len(adb.calls), len(fwd.calls))
	}
}
