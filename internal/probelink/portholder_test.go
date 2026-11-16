package probelink

import (
	"net"
	"strings"
	"testing"
)

func TestParseLsofHolders(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{"empty", "", ""},
		{"one", "p4242\ncRunner\n", "pid 4242 (Runner)"},
		{"two", "p1\ncadb\np22\ncRunner\n", "pid 1 (adb), pid 22 (Runner)"},
		{"pid without command ignored", "p9\n", ""},
	}
	for _, c := range cases {
		if got := parseLsofHolders(c.out); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPortHolderHint_RemoteHostHasNoHint(t *testing.T) {
	if got := portHolderHint("192.168.1.10", 48686); got != "" {
		t.Errorf("remote host must not be probed via local lsof, got %q", got)
	}
}

// A real listener on a loopback port is found by lsof when it is installed;
// without lsof the helper degrades to "" rather than failing.
func TestPortHolder_FindsOwnListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	got := PortHolder(port)
	if got == "" {
		t.Skip("lsof unavailable or restricted in this environment")
	}
	if !strings.HasPrefix(got, "pid ") {
		t.Errorf("want \"pid N (cmd)\", got %q", got)
	}
	if hint := portHolderHint("127.0.0.1", port); !strings.Contains(hint, "--agent-port") {
		t.Errorf("hint should name the --agent-port escape hatch, got %q", hint)
	}
}

func TestDescribeHolders_DistinguishesAdbFromASimulatorApp(t *testing.T) {
	// The process listening for an `adb forward` is the shared adb server, whatever device the forward belongs to (and the CLI's own forward looks the same):
	// it is never named, so no pid and no "forward" claim reach the message.
	adbOnly := describeHolders(48686, []Holder{{PID: "7", Name: "adb", Command: "adb -L tcp:5037 fork-server server"}})
	if adbOnly != "" {
		t.Errorf("the adb server alone must give no hint, got: %s", adbOnly)
	}
	if got := describeHolders(48686, []Holder{{PID: "7", Name: "ADB"}, {PID: "8", Name: "adb"}}); got != "" {
		t.Errorf("adb holders (any case, several) must give no hint, got: %s", got)
	}
	mixed := describeHolders(48686, []Holder{{PID: "7", Name: "adb"}, {PID: "3", Name: "python3", Command: "python3 -m http.server"}})
	if !strings.Contains(mixed, "pid 3 (python3)") || strings.Contains(mixed, "adb") || strings.Contains(mixed, "pid 7") || strings.Contains(mixed, "forward") {
		t.Errorf("with another holder only that one is named, got: %s", mixed)
	}
	sim := describeHolders(48686, []Holder{{PID: "9", Name: "Runner",
		Command: "/Users/x/Library/Developer/CoreSimulator/Devices/11111111-2222-3333-4444-555555555555/data/Containers/Bundle/Application/AAA/Runner.app/Runner"}})
	if !strings.Contains(sim, "iOS simulator app") || !strings.Contains(sim, "11111111-2222-3333-4444-555555555555") || !strings.Contains(sim, "simctl terminate") {
		t.Errorf("simulator hint: %s", sim)
	}
	if strings.Contains(sim, "adb forward") {
		t.Errorf("a simulator app must not be described as an adb forward: %s", sim)
	}
	other := describeHolders(48686, []Holder{{PID: "3", Name: "python3", Command: "python3 -m http.server"}})
	if !strings.Contains(other, "pid 3 (python3)") || !strings.Contains(other, "--agent-port") {
		t.Errorf("generic hint: %s", other)
	}
	if describeHolders(48686, nil) != "" {
		t.Error("a free port needs no hint")
	}
}

func TestSimulatorUDID(t *testing.T) {
	if got := simulatorUDID("/a/CoreSimulator/Devices/ABC-123/data/x"); got != "ABC-123" {
		t.Errorf("got %q", got)
	}
	if simulatorUDID("/usr/bin/adb") != "" || simulatorUDID("") != "" {
		t.Error("non-simulator commands have no UDID")
	}
}

// What lsof printed comes from another process: it is cleaned to one short plain line before it reaches a message, and a pid or simulator id that is not what it
// should be is not printed as it came.
func TestHolderTextIsCleanedBeforeItIsPrinted(t *testing.T) {
	long := strings.Repeat("é", 200)
	for name, c := range map[string]struct{ in, want string }{
		"plain":            {"python3", "python3"},
		"escape sequence":  {"\x1b[31mred\x1b[0m", "[31mred [0m"},
		"newline and tab":  {"a\nb\tc", "a b c"},
		"bidi and zero":    {"ab‮​cd", "ab cd"},
		"runs of spaces":   {"  a   b  ", "a b"},
		"invalid utf8":     {"ab\xffcd", "ab cd"},
		"empty":            {"", ""},
		"only hidden text": {"​‮", ""},
	} {
		if got := cleanHolderText(c.in, 48); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	got := cleanHolderText(long, 48)
	if r := []rune(got); len(r) != 48 || r[47] != '…' {
		t.Errorf("a long name must be cut to 48 runes ending in an ellipsis, got %d runes: %q", len([]rune(got)), got)
	}
	if holderPID("4242") != "4242" || holderPID("42\n42") != "?" || holderPID("") != "?" || holderPID("12345678901") != "?" || holderPID("\x1b[2J") != "?" {
		t.Error("a pid is only digits, at most 10")
	}
	if !validUDID("11111111-2222-3333-4444-555555555555") || validUDID("../x") || validUDID("11111111-2222-3333-4444-55555555555;") || validUDID("") {
		t.Error("a simulator id is a UUID")
	}

	hostile := describeHolders(48686, []Holder{{PID: "9\x1b[2J", Name: "ru\x1b[31mn\nner‮" + long}})
	for _, bad := range []string{"\x1b", "\n", "‮", long} {
		if strings.Contains(hostile, bad) {
			t.Errorf("a hostile holder name or pid reached the message: %q", hostile)
		}
	}
	if !strings.Contains(hostile, "pid ?") {
		t.Errorf("a pid that is not digits is shown as ?: %q", hostile)
	}
	// a path that only looks like a simulator one, with a made-up id: not described as a simulator app, and the id is not printed
	fake := describeHolders(48686, []Holder{{PID: "9", Name: "Runner", Command: "/x/CoreSimulator/Devices/x;rm -rf ~/data/Containers/Runner"}})
	if strings.Contains(fake, "simctl") || strings.Contains(fake, "rm -rf") {
		t.Errorf("an id that is not a UUID must not reach a command line: %q", fake)
	}
}
