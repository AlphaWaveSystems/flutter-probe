package device

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// pocap reads a real capture (see testdata/portowner/README.md).
func pocap(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/portowner/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// poAdb answers the three commands of the lookup from a table and records every call, so a test sees what was asked and that nothing else was.
type poAdb struct {
	tcp6, tcp, pm string
	fail          map[string]bool // key: "tcp6", "tcp" or "pm"
	calls         [][]string
}

func (a *poAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	a.calls = append(a.calls, append([]string(nil), args...))
	line := strings.Join(args, " ")
	key, out := "", ""
	switch {
	case strings.HasSuffix(line, "cat /proc/net/tcp6"):
		key, out = "tcp6", a.tcp6
	case strings.HasSuffix(line, "cat /proc/net/tcp"):
		key, out = "tcp", a.tcp
	case strings.HasSuffix(line, "pm list packages -U"):
		key, out = "pm", a.pm
	default:
		return nil, errors.New("unexpected command: " + line)
	}
	if a.fail[key] {
		return nil, errors.New("adb: failed")
	}
	return []byte(out), nil
}

func capturedDevice(t *testing.T, api string) *poAdb {
	return &poAdb{tcp6: pocap(t, "tcp6-"+api+".txt"), tcp: pocap(t, "tcp-"+api+".txt"), pm: pocap(t, "pm-list-U-"+api+".txt")}
}

// The real captures: the first app holds 48911, the second lost it. Measured on an Android 15 phone image and an Android 14 TV image.
func TestListeningUIDsOnRealCaptures(t *testing.T) {
	for api, want := range map[string]int{"api35": 10207, "api34": 10122} {
		got := listeningUIDs(pocap(t, "tcp6-"+api+".txt"), 48911)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%s: uids on 48911 = %v, want [%d]", api, got, want)
		}
		if got := listeningUIDs(pocap(t, "tcp-"+api+".txt"), 48911); len(got) != 0 {
			t.Errorf("%s: tcp has only its header, got %v", api, got)
		}
		if got := listeningUIDs(pocap(t, "tcp6-"+api+".txt"), 48912); len(got) != 0 {
			t.Errorf("%s: nothing listens on 48912, got %v", api, got)
		}
	}
	// the port of adbd on the emulator (0x15B3 = 5555) belongs to the shell user
	if got := listeningUIDs(pocap(t, "tcp6-api35.txt"), 5555); len(got) != 1 || got[0] != 2000 {
		t.Errorf("5555 = %v, want [2000]", got)
	}
}

func TestListeningUIDsRowRules(t *testing.T) {
	row := func(addr, state, uid string) string {
		return "   0: " + addr + " 00000000000000000000000000000000:0000 " + state + " 00000000:00000000 00:00000000 00000000 " + uid + " 0 1 1 0 100 0 0 10 0\n"
	}
	const mapped = "0000000000000000FFFF00000100007F:BF0F"
	cases := map[string]struct {
		in   string
		want []int
	}{
		"listening, mapped loopback":   {row(mapped, "0A", "10207"), []int{10207}},
		"lower case hex":               {row(strings.ToLower(mapped), "0A", "10207"), []int{10207}},
		"established is not listening": {row(mapped, "01", "10207"), nil},
		"time wait":                    {row(mapped, "06", "10207"), nil},
		"another port":                 {row("0000000000000000FFFF00000100007F:BF10", "0A", "10207"), nil},
		"port prefix only":             {row("0000000000000000FFFF00000100007F:0BF0F", "0A", "10207"), nil},
		"an outside address":           {row("0000000000000000FFFF0000C0A80105:BF0F", "0A", "10207"), nil},
		"the any address":              {row("00000000000000000000000000000000:BF0F", "0A", "10207"), []int{10207}},
		"ipv6 loopback":                {row("00000000000000000000000001000000:BF0F", "0A", "10207"), []int{10207}},
		"ipv4 loopback":                {row("0100007F:BF0F", "0A", "10207"), []int{10207}},
		"ipv4 any":                     {row("00000000:BF0F", "0A", "10207"), []int{10207}},
		"ipv4 outside":                 {row("0105A8C0:BF0F", "0A", "10207"), nil},
		"negative uid":                 {row(mapped, "0A", "-1"), nil},
		"uid not a number":             {row(mapped, "0A", "abc"), nil},
		"huge uid":                     {row(mapped, "0A", "99999999999"), nil},
		"too few fields":               {"   0: " + mapped + " 0A\n", nil},
		"two sockets, one uid":         {row(mapped, "0A", "10207") + row(mapped, "0A", "10207"), []int{10207}},
		"two uids sorted":              {row(mapped, "0A", "10300") + row(mapped, "0A", "10207"), []int{10207, 10300}},
		"empty":                        {"", nil},
		"garbage":                      {"\x00\x01 not a table\n\n\xff\n", nil},
	}
	for name, c := range cases {
		got := listeningUIDs(c.in, 48911)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", name, got, c.want)
			}
		}
	}
	for _, port := range []int{0, -1, 65536} {
		if got := listeningUIDs(row(mapped, "0A", "10207"), port); got != nil {
			t.Errorf("port %d is not a port: %v", port, got)
		}
	}
	// a flood of rows is cut at the line cap and does not run away
	var b strings.Builder
	for i := 0; i < procNetMaxLines+50; i++ {
		b.WriteString(row(mapped, "01", "10207"))
	}
	b.WriteString(row(mapped, "0A", "10207"))
	if got := listeningUIDs(b.String(), 48911); len(got) != 0 {
		t.Errorf("a row after the line cap must not be read: %v", got)
	}
}

func TestParsePackageUIDs(t *testing.T) {
	m := parsePackageUIDs(pocap(t, "pm-list-U-api35.txt"))
	if got := m[10207]; len(got) != 1 || got[0] != "dev.probe.portspikea" {
		t.Errorf("uid 10207 = %v", got)
	}
	if got := m[2000]; len(got) != 1 || got[0] != "com.android.shell" {
		t.Errorf("uid 2000 = %v", got)
	}
	in := strings.Join([]string{
		"package:com.example.one uid:10300",
		"package:com.example.two uid:10300",         // shared uid
		"package:com.example.multi uid:10400,10500", // several users: the first is read
		"package:evil;rm -rf uid:10301",             // not a package name
		"package:com.example.neg uid:-5",
		"package:com.example.nan uid:xyz",
		"package: uid:10302",
		"package:com.example.nouid",
		"uid:10303",
		"",
		"\x00\xff garbage",
	}, "\n")
	m = parsePackageUIDs(in)
	if got := m[10300]; len(got) != 2 {
		t.Errorf("a shared uid lists both packages: %v", got)
	}
	if got := m[10400]; len(got) != 1 {
		t.Errorf("multi-user uid: %v", m)
	}
	for _, bad := range []int{10301, 10302, 10303, -5, 10500} {
		if _, ok := m[bad]; ok {
			t.Errorf("uid %d must not be listed: %v", bad, m)
		}
	}
}

// The loser of the port is told who holds it: on both measured Android versions.
func TestPortOwnersNamesTheHolderOnBothCapturedVersions(t *testing.T) {
	for _, api := range []string{"api35", "api34"} {
		a := capturedDevice(t, api)
		got := PortOwners(context.Background(), a, "emulator-5600", 48911, "dev.probe.portspikeb")
		if len(got) != 1 || got[0] != "dev.probe.portspikea" {
			t.Errorf("%s: owners = %v, want [dev.probe.portspikea]", api, got)
		}
		// exactly the three read-only reads, nothing else
		want := []string{
			"-s emulator-5600 shell cat /proc/net/tcp6",
			"-s emulator-5600 shell cat /proc/net/tcp",
			"-s emulator-5600 shell pm list packages -U",
		}
		var calls []string
		for _, c := range a.calls {
			calls = append(calls, strings.Join(c, " "))
		}
		if strings.Join(calls, "|") != strings.Join(want, "|") {
			t.Errorf("%s: calls = %v, want %v", api, calls, want)
		}
	}
}

func TestPortOwnersSaysNothingWhenItCannotTell(t *testing.T) {
	ctx := context.Background()
	const app = "dev.probe.portspikeb"
	own := func(a *poAdb) []string { return PortOwners(ctx, a, "emulator-5600", 48911, app) }

	// the app under test is itself the holder: not "another app"
	if got := PortOwners(ctx, capturedDevice(t, "api35"), "emulator-5600", 48911, "dev.probe.portspikea"); got != nil {
		t.Errorf("the holder is the app under test: %v", got)
	}
	// nothing listens on the port
	if got := PortOwners(ctx, capturedDevice(t, "api35"), "emulator-5600", 48999, app); got != nil {
		t.Errorf("a free port: %v", got)
	}
	// the holder is the shell user (a leftover nc, adbd): not an app
	if got := PortOwners(ctx, capturedDevice(t, "api35"), "emulator-5600", 5555, app); got != nil {
		t.Errorf("a port held by the shell: %v", got)
	}
	// the holder's uid has no package in the list
	a := capturedDevice(t, "api35")
	a.pm = "package:dev.probe.portspikeb uid:10211\n"
	if got := own(a); got != nil {
		t.Errorf("a uid with no package: %v", got)
	}
	// the files or the list cannot be read
	for _, k := range []string{"tcp6", "pm"} {
		a := capturedDevice(t, "api35")
		a.fail = map[string]bool{k: true}
		if got := own(a); got != nil {
			t.Errorf("%s unreadable: %v", k, got)
		}
	}
	// tcp unreadable but tcp6 fine: still named
	a = capturedDevice(t, "api35")
	a.fail = map[string]bool{"tcp": true}
	if got := own(a); len(got) != 1 {
		t.Errorf("tcp unreadable, tcp6 fine: %v", got)
	}
	// a runaway output is not parsed
	a = capturedDevice(t, "api35")
	a.tcp6 = a.tcp6 + strings.Repeat("x", procNetMaxBytes)
	if got := own(a); got != nil {
		t.Errorf("an oversized table: %v", got)
	}
	// nothing that does not look like a serial or a package reaches adb
	for name, c := range map[string]struct{ serial, app string }{
		"serial with shell characters": {"emu;rm", app},
		"empty serial":                 {"", app},
		"no app":                       {"emulator-5600", ""},
		"app that is not a package":    {"emulator-5600", "a;b"},
	} {
		a := capturedDevice(t, "api35")
		if got := PortOwners(ctx, a, c.serial, 48911, c.app); got != nil || len(a.calls) != 0 {
			t.Errorf("%s: got %v, calls %v", name, got, a.calls)
		}
	}
	if got := PortOwners(ctx, nil, "emulator-5600", 48911, app); got != nil {
		t.Errorf("no adb: %v", got)
	}
	// a cancelled context makes no call
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	a = capturedDevice(t, "api35")
	if got := PortOwners(cancelled, a, "emulator-5600", 48911, app); got != nil || len(a.calls) != 0 {
		t.Errorf("cancelled: %v %v", got, a.calls)
	}
}

func TestPortOwnersSharedUidAndTheCap(t *testing.T) {
	a := capturedDevice(t, "api35")
	a.tcp6 = "  sl\n   0: 0000000000000000FFFF00000100007F:BF0F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 10300 0 1 1 0 100 0 0 10 0\n"
	a.pm = "package:com.example.d uid:10300\npackage:com.example.a uid:10300\npackage:com.example.c uid:10300\npackage:com.example.b uid:10300\npackage:dev.probe.portspikeb uid:10211\n"
	got := PortOwners(context.Background(), a, "emulator-5600", 48911, "dev.probe.portspikeb")
	if strings.Join(got, ",") != "com.example.a,com.example.b,com.example.c" {
		t.Errorf("a shared uid: sorted, at most %d names, got %v", portOwnerMaxNames, got)
	}
	// a second user's copy of the app under test is another uid and is another app
	a = capturedDevice(t, "api35")
	a.tcp6 = "  sl\n   0: 0000000000000000FFFF00000100007F:BF0F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 1010211 0 1 1 0 100 0 0 10 0\n"
	if got := PortOwners(context.Background(), a, "emulator-5600", 48911, "dev.probe.portspikeb"); got != nil {
		t.Errorf("a uid of another user with no listed package: %v", got)
	}
	// the app under test listens on two uids' worth of sockets: it is never named as its own blocker
	a = capturedDevice(t, "api35")
	a.pm = "package:dev.probe.portspikeb uid:10207\npackage:dev.probe.portspikea uid:10207\n"
	if got := PortOwners(context.Background(), a, "emulator-5600", 48911, "dev.probe.portspikeb"); got != nil {
		t.Errorf("the app under test shares the holder's uid: %v", got)
	}
}

func TestPortOwnerHintText(t *testing.T) {
	ctx := context.Background()
	one := PortOwnerHint(ctx, capturedDevice(t, "api35"), "emulator-5600", 48911, "dev.probe.portspikeb")
	for _, want := range []string{" — agent port 48911 on the device is held by dev.probe.portspikea (another app on this device)", "PROBE_PORT", "agent.port"} {
		if !strings.Contains(one, want) {
			t.Errorf("hint %q lacks %q", one, want)
		}
	}
	if !strings.HasPrefix(one, " — ") || strings.Contains(one, "pid") || strings.Contains(one, "10207") {
		t.Errorf("the hint names no pid or uid: %q", one)
	}
	a := capturedDevice(t, "api35")
	a.tcp6 = "  sl\n   0: 0000000000000000FFFF00000100007F:BF0F 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 10300 0 1 1 0 100 0 0 10 0\n"
	a.pm = "package:com.example.a uid:10300\npackage:com.example.b uid:10300\n"
	if many := PortOwnerHint(ctx, a, "emulator-5600", 48911, "dev.probe.portspikeb"); !strings.Contains(many, "held by one of com.example.a, com.example.b (other apps") {
		t.Errorf("shared uid hint: %q", many)
	}
	if got := PortOwnerHint(ctx, capturedDevice(t, "api35"), "emulator-5600", 48999, "dev.probe.portspikeb"); got != "" {
		t.Errorf("a free port gives no hint: %q", got)
	}
}

// The bound is five seconds, and a shorter deadline of the caller wins.
func TestPortOwnersAreBounded(t *testing.T) {
	if portOwnerTimeout != 5*time.Second {
		t.Fatalf("portOwnerTimeout = %v", portOwnerTimeout)
	}
	rec := &deadlineAdb{inner: capturedDevice(t, "api35")}
	start := time.Now()
	PortOwners(context.Background(), rec, "emulator-5600", 48911, "dev.probe.portspikeb")
	if !rec.has || rec.deadline.Sub(start) < 4*time.Second || rec.deadline.Sub(start) > 6*time.Second {
		t.Errorf("deadline %v away (has=%v), want about 5s", rec.deadline.Sub(start), rec.has)
	}
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	rec = &deadlineAdb{inner: capturedDevice(t, "api35")}
	PortOwners(parent, rec, "emulator-5600", 48911, "dev.probe.portspikeb")
	if time.Until(rec.deadline) > time.Second {
		t.Errorf("a shorter deadline of the caller must win, got %v", time.Until(rec.deadline))
	}
}

type deadlineAdb struct {
	inner    *poAdb
	deadline time.Time
	has      bool
}

func (d *deadlineAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	d.deadline, d.has = ctx.Deadline()
	return d.inner.RunArgs(ctx, args...)
}

// ConnectHint names a forward to another device first, and only when there is none another app on the device.
func TestConnectHintOrder(t *testing.T) {
	ctx := context.Background()
	both := &mixedAdb{poAdb: capturedDevice(t, "api35"), forward: "emulator-5554 tcp:48911 tcp:48911\n"}
	if h := ConnectHint(ctx, both, "emulator-5600", 48911, 48911, "dev.probe.portspikeb"); !strings.Contains(h, "forwarded to another device (emulator-5554)") || strings.Contains(h, "held by") {
		t.Errorf("a forward to another device comes first: %q", h)
	}
	none := &mixedAdb{poAdb: capturedDevice(t, "api35"), forward: ""}
	if h := ConnectHint(ctx, none, "emulator-5600", 48911, 48911, "dev.probe.portspikeb"); !strings.Contains(h, "held by dev.probe.portspikea") {
		t.Errorf("with no forward the holder is named: %q", h)
	}
	own := &mixedAdb{poAdb: capturedDevice(t, "api35"), forward: "emulator-5600 tcp:48911 tcp:48911\n"}
	if h := ConnectHint(ctx, own, "emulator-5600", 48911, 48911, "dev.probe.portspikeb"); !strings.Contains(h, "held by dev.probe.portspikea") {
		t.Errorf("this device's own forward is not blamed: %q", h)
	}
	free := &mixedAdb{poAdb: capturedDevice(t, "api35"), forward: ""}
	if h := ConnectHint(ctx, free, "emulator-5600", 48911, 48999, "dev.probe.portspikeb"); h != "" {
		t.Errorf("nothing to say: %q", h)
	}
}

type mixedAdb struct {
	*poAdb
	forward string
}

func (m *mixedAdb) RunArgs(ctx context.Context, args ...string) ([]byte, error) {
	if strings.Join(args, " ") == "forward --list" {
		return []byte(m.forward), nil
	}
	return m.poAdb.RunArgs(ctx, args...)
}

func TestBlocksLoopbackBind(t *testing.T) {
	for addr, want := range map[string]bool{
		"00000000":                         true,
		"0100007F":                         true,
		"0200007F":                         true, // 127.0.0.2
		"0105A8C0":                         false,
		"00000000000000000000000000000000": true,
		"00000000000000000000000001000000": true,
		"0000000000000000FFFF00000100007F": true,
		"0000000000000000FFFF000000000000": true,
		"0000000000000000FFFF0000C0A80105": false,
		"20010DB8000000000000000000000001": false,
		"":                                 false,
		"ZZZZZZZZ":                         false,
		"0100007":                          false,
	} {
		if got := blocksLoopbackBind(addr); got != want {
			t.Errorf("blocksLoopbackBind(%q) = %v, want %v", addr, got, want)
		}
	}
}

// The token error of a dead app and the marker the runner looks for are one text: a reword of one cannot silently change the other.
func TestTokenAppNotRunningErrorCarriesTheMarker(t *testing.T) {
	err := TokenAppNotRunningError(30*time.Second, "dev.probe.app")
	if !strings.Contains(err.Error(), AppNotRunningMarker) || !strings.Contains(err.Error(), "dev.probe.app") || !strings.Contains(err.Error(), "30s") {
		t.Errorf("error = %q", err)
	}
	// the token timeout of an app that does run does not say it is not running
	if strings.Contains(NeutralPortHint, AppNotRunningMarker) {
		t.Error("the hedged line must not look like the dead-app error")
	}
}
