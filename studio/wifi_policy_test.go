package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsLocalAddressClasses(t *testing.T) {
	cases := []struct {
		ip    string
		local bool
	}{
		{"127.0.0.1", true}, {"127.8.9.10", true}, {"::1", true},
		{"169.254.10.20", true}, {"fe80::1", true},
		{"10.0.0.5", true}, {"10.255.255.255", true},
		{"172.16.0.1", true}, {"172.31.255.1", true}, {"172.15.255.255", false}, {"172.32.0.1", false},
		{"192.168.1.23", true}, {"192.169.0.1", false},
		{"fc00::1", true}, {"fd12:3456::9", true}, {"fe00::1", false},
		{"8.8.8.8", false}, {"1.1.1.1", false}, {"2001:4860:4860::8888", false},
		{"::ffff:8.8.8.8", false}, {"::ffff:192.168.1.5", true},
		{"0.0.0.0", false}, {"::", false}, {"224.0.0.1", false}, {"255.255.255.255", false},
		{"100.64.0.1", false}, // carrier-grade NAT (some VPNs) is not in the default set
	}
	for _, c := range cases {
		if got := isLocalAddress(net.ParseIP(c.ip)); got != c.local {
			t.Errorf("%s: local=%v, want %v", c.ip, got, c.local)
		}
	}
}

func stubResolver(m map[string][]string) func(context.Context, string) ([]net.IP, error) {
	return func(_ context.Context, host string) ([]net.IP, error) {
		addrs, ok := m[host]
		if !ok {
			return nil, errors.New("no such host")
		}
		var out []net.IP
		for _, a := range addrs {
			out = append(out, net.ParseIP(a))
		}
		return out, nil
	}
}

func TestCheckWiFiTargetByClass(t *testing.T) {
	t.Setenv(allowPublicEnv, "")
	a := &App{lookupHost: stubResolver(map[string][]string{
		"iphone.local":    {"192.168.1.45"},
		"printer":         {"10.0.0.9"},
		"evil.example":    {"8.8.8.8"},
		"mixed.example":   {"192.168.1.5", "8.8.4.4"},
		"rebinding.local": {"1.2.3.4"}, // a .local name that resolves to a public address
		"v6.local":        {"fd00::5"},
	})}
	cases := []struct {
		host string
		want string // "" = allowed, else a substring of the error
	}{
		{"127.0.0.1", ""}, {"[::1]", ""}, {"192.168.1.23", ""}, {"10.1.2.3", ""}, {"172.20.0.4", ""},
		{"169.254.7.7", ""}, {"fe80::1c2b%en0", ""}, {"fd00::1", ""},
		{"iphone.local", ""}, {"printer", ""}, {"v6.local", ""},
		{"8.8.8.8", "not on this machine or a local network"},
		{"2001:4860:4860::8888", "not on this machine or a local network"},
		{"evil.example", "resolves to 8.8.8.8"},
		{"mixed.example", "resolves to 8.8.4.4"},
		{"rebinding.local", "resolves to 1.2.3.4"},
		{"nonexistent.example", "could not resolve"},
		{"0.0.0.0", "not on this machine"},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			err := a.checkWiFiTarget(context.Background(), c.host)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("should be allowed: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestPublicTargetsNeedAnExplicitOptIn(t *testing.T) {
	a := &App{lookupHost: stubResolver(map[string][]string{"evil.example": {"8.8.8.8"}})}
	t.Setenv(allowPublicEnv, "")
	if err := a.checkWiFiTarget(context.Background(), "8.8.8.8"); err == nil || !strings.Contains(err.Error(), allowPublicEnv) {
		t.Fatalf("the refusal must name the opt-in: %v", err)
	}
	for _, v := range []string{"1", "true", "YES"} {
		t.Setenv(allowPublicEnv, v)
		if err := a.checkWiFiTarget(context.Background(), "8.8.8.8"); err != nil {
			t.Errorf("%s=%s should allow a public address: %v", allowPublicEnv, v, err)
		}
		if err := a.checkWiFiTarget(context.Background(), "evil.example"); err != nil {
			t.Errorf("%s=%s should allow a public hostname: %v", allowPublicEnv, v, err)
		}
	}
	t.Setenv(allowPublicEnv, "0")
	if err := a.checkWiFiTarget(context.Background(), "8.8.8.8"); err == nil {
		t.Error("0 must not opt in")
	}
	// Even with the opt-in, an unresolvable name is still an error.
	t.Setenv(allowPublicEnv, "1")
	if err := a.checkWiFiTarget(context.Background(), "nonexistent.example"); err == nil {
		t.Error("an unresolvable name must fail")
	}
}

func TestWiFiAttemptsAreRateLimited(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	l := &wifiLimiter{now: func() time.Time { return now }}
	for i := 0; i < wifiAttemptsPerMinute; i++ {
		if err := l.allow(); err != nil {
			t.Fatalf("attempt %d refused: %v", i+1, err)
		}
	}
	if err := l.allow(); !errors.Is(err, errWiFiRateLimited) {
		t.Fatalf("the next attempt must be limited: %v", err)
	}
	now = now.Add(61 * time.Second)
	if err := l.allow(); err != nil {
		t.Fatalf("a minute later it must work again: %v", err)
	}
}

func TestTestWiFiIsLimitedAndRefusesPublicTargetsBeforeDialling(t *testing.T) {
	t.Setenv(allowPublicEnv, "")
	a := &App{ctx: context.Background(), lookupHost: stubResolver(nil)}
	if _, err := a.TestWiFi("8.8.8.8", 48686, goodToken); err == nil || !strings.Contains(err.Error(), "not on this machine") {
		t.Fatalf("public target: %v", err)
	}
	var last error
	for i := 0; i < wifiAttemptsPerMinute+2; i++ {
		_, last = a.TestWiFi("8.8.8.8", 48686, goodToken)
	}
	if !errors.Is(last, errWiFiRateLimited) {
		t.Fatalf("repeated attempts must be rate limited, got %v", last)
	}
	// ConnectWiFi shares the limit, and its refusal does not echo the token either.
	_, err := a.ConnectWiFi("8.8.8.8", 48686, goodToken)
	if err == nil || strings.Contains(err.Error(), goodToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestRedactToken(t *testing.T) {
	got := redactToken("dial ws://10.0.0.5:48686/probe?token=abcdefghijklmnop: connection refused", "abcdefghijklmnop")
	if strings.Contains(got, "abcdefghijklmnop") || !strings.Contains(got, "[redacted]") {
		t.Fatalf("got %q", got)
	}
	if redactToken("x", "") != "x" {
		t.Fatal("an empty token changes nothing")
	}
}

func TestTestWiFiErrorsNeverEchoTheToken(t *testing.T) {
	t.Setenv(allowPublicEnv, "")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	a := &App{ctx: context.Background()}
	token := "supersecrettoken1234567"
	_, err = a.TestWiFi("127.0.0.1", port, token)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("err = %v", err)
	}
}
