package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// allowPublicEnv lets Studio dial a public address. It is deliberately an environment variable of the Studio
// process and not a setting an automation or MCP caller can pass: the manual WiFi entry sends the agent token
// to whatever address it is given, and an agent driving Studio must not be able to point that at the internet.
const allowPublicEnv = "PROBE_STUDIO_ALLOW_PUBLIC_TARGETS"

var privateNets = mustCIDRs(
	"127.0.0.0/8",    // loopback
	"::1/128",        // loopback
	"169.254.0.0/16", // link-local
	"fe80::/10",      // link-local
	"10.0.0.0/8",     // RFC 1918
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7", // unique local (ULA)
)

func mustCIDRs(list ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range list {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// isLocalAddress reports whether ip is on this machine or a local network: loopback, link-local,
// RFC 1918 private IPv4, IPv6 unique-local. An IPv4-mapped IPv6 address is judged by its IPv4 form.
func isLocalAddress(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func publicTargetsAllowed() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(allowPublicEnv)))
	return v == "1" || v == "true" || v == "yes"
}

// checkWiFiTarget refuses a host that is not on this machine or a local network, unless the owner of the Studio
// process opted in with PROBE_STUDIO_ALLOW_PUBLIC_TARGETS=1. A hostname is resolved and every address it resolves to
// must be local too: a name such as phone.local that resolves to a public address (a poisoned or hostile DNS answer)
// is refused like the address itself.
func (a *App) checkWiFiTarget(ctx context.Context, host string) error {
	h := strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	zoneless := strings.Split(h, "%")[0]
	var ips []net.IP
	if ip := net.ParseIP(zoneless); ip != nil {
		ips = []net.IP{ip}
	} else {
		lookup := a.lookupHost
		if lookup == nil {
			lookup = defaultLookup
		}
		rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		found, err := lookup(rctx, h)
		if err != nil || len(found) == 0 {
			return fmt.Errorf("could not resolve %q to an address: check the name, or use the phone's IP address", host)
		}
		ips = found
	}
	if publicTargetsAllowed() {
		return nil
	}
	for _, ip := range ips {
		if !isLocalAddress(ip) {
			return fmt.Errorf("%s is not on this machine or a local network (it resolves to %s), so Studio will not send an agent token there. "+
				"Use the phone's address on your local network (10.x, 172.16-31.x, 192.168.x, link-local, an IPv6 ULA address, or a .local name). "+
				"To allow a public address deliberately, start Studio with %s=1", host, ip, allowPublicEnv)
		}
	}
	return nil
}

func defaultLookup(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IP)
	}
	return out, nil
}

// wifiLimiter allows a few manual WiFi attempts per minute, so the entry cannot be used to scan a network.
type wifiLimiter struct {
	mu    sync.Mutex
	times []time.Time
	now   func() time.Time
}

const (
	wifiAttemptsPerMinute = 6
)

var errWiFiRateLimited = errors.New("too many WiFi connection attempts: wait a minute and try again (the limit is 6 per minute)")

func (l *wifiLimiter) allow() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now
	if l.now != nil {
		now = l.now
	}
	cut := now().Add(-time.Minute)
	kept := l.times[:0]
	for _, t := range l.times {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	l.times = kept
	if len(l.times) >= wifiAttemptsPerMinute {
		return errWiFiRateLimited
	}
	l.times = append(l.times, now())
	return nil
}

// redactToken removes the agent token from text that is about to be shown: dial errors can carry the URL,
// which has the token in its query.
func redactToken(s, token string) string {
	if token == "" {
		return s
	}
	return strings.ReplaceAll(s, token, "[redacted]")
}
