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
	adb := describeHolders(48686, []Holder{{PID: "7", Name: "adb", Command: "adb -L tcp:5037 fork-server server"}})
	if !strings.Contains(adb, "adb port forward") || !strings.Contains(adb, "adb forward --remove tcp:48686") || strings.Contains(adb, "simulator") {
		t.Errorf("adb hint: %s", adb)
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
