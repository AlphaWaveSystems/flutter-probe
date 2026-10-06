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
