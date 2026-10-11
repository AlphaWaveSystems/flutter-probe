package main

import (
	"context"
	"net"
	"strings"
	"testing"
)

const goodToken = "abcdefghijklmnopqrstuvwxyz012345"

func TestValidateWiFiTarget(t *testing.T) {
	cases := []struct {
		name  string
		host  string
		port  int
		token string
		want  string // substring of the error, "" = valid
	}{
		{"ipv4", "192.168.1.20", 48686, goodToken, ""},
		{"ipv6", "fe80::1c2b:3d4e", 48686, goodToken, ""},
		{"ipv6 in brackets", "[fe80::1]", 48686, goodToken, ""},
		{"ipv6 with zone", "fe80::1%en0", 48686, goodToken, ""},
		{"mdns hostname", "janes-iphone.local", 48686, goodToken, ""},
		{"hostname with trailing dot", "phone.example.", 48686, goodToken, ""},
		{"surrounding spaces are trimmed", "  10.0.0.5 ", 48686, goodToken, ""},
		{"port 1", "10.0.0.5", 1, goodToken, ""},
		{"port 65535", "10.0.0.5", 65535, goodToken, ""},
		{"empty host", "", 48686, goodToken, "IP address or hostname"},
		{"scheme in host", "ws://10.0.0.5", 48686, goodToken, "no scheme"},
		{"host with port", "10.0.0.5:48686", 48686, goodToken, "not a valid"},
		{"path in host", "10.0.0.5/probe", 48686, goodToken, "no scheme"},
		{"bad ipv4", "300.1.1.1", 48686, goodToken, "not a valid"},
		{"digits only label", "1.2.3", 48686, goodToken, "not a valid"},
		{"underscore hostname", "my_phone.local", 48686, goodToken, "not a valid"},
		{"leading hyphen label", "-phone.local", 48686, goodToken, "not a valid"},
		{"port zero", "10.0.0.5", 0, goodToken, "out of range"},
		{"port too large", "10.0.0.5", 65536, goodToken, "out of range"},
		{"negative port", "10.0.0.5", -1, goodToken, "out of range"},
		{"empty token", "10.0.0.5", 48686, "", "PROBE_TOKEN="},
		{"short token", "10.0.0.5", 48686, "abc", "8 to 128"},
		{"token with the prefix", "10.0.0.5", 48686, "PROBE_TOKEN=" + goodToken, "PROBE_TOKEN="},
		{"token with a space", "10.0.0.5", 48686, "abcd efgh1234", "8 to 128"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateWiFiTarget(c.host, c.port, c.token)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestWiFiHintSaysWhatToCheck(t *testing.T) {
	for _, c := range []struct {
		err  string
		want []string
	}{
		{"dial tcp 10.0.0.5:48686: connect: connection refused", []string{"Nothing is listening", "PROBE_WIFI=true", "foreground", "same network", "curl http://10.0.0.5:48686/probe/status"}},
		{"dial tcp: i/o timeout", []string{"did not answer in time", "same network"}},
		{"websocket: bad handshake: 401 Unauthorized", []string{"refused the token", "PROBE_TOKEN="}},
		{"something odd", []string{"Check:", "PROBE_AGENT=true"}},
	} {
		got := wifiHint("10.0.0.5", 48686, errString(c.err))
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("hint for %q lacks %q:\n%s", c.err, w, got)
			}
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestTestWiFiOnAClosedPortSaysWhatToCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens there any more
	a := &App{ctx: context.Background()}
	_, err = a.TestWiFi("127.0.0.1", port, goodToken)
	if err == nil || !strings.Contains(err.Error(), "PROBE_WIFI=true") || !strings.Contains(err.Error(), "foreground") {
		t.Fatalf("err = %v", err)
	}
	// An invalid target is refused before any dial.
	if _, err := a.TestWiFi("ws://x", port, goodToken); err == nil || !strings.Contains(err.Error(), "no scheme") {
		t.Fatalf("err = %v", err)
	}
}
