package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/httpcall"
)

// The rules for who Studio's Send request panel may call. The panel makes requests FROM this machine to an address a person types, so
// the decision is made here, in Go, and not only in the window: the headless bridge exposes every method of App to a page script.

const (
	sendMaxPerMinute = 10
	sendResolveTime  = 3 * time.Second
)

var (
	errSendHostNotAllowed = errors.New("that host has not been allowed in this session: confirm it first")
	errSendBusy           = errors.New("another request is still running")
	errSendRate           = errors.New("too many requests in the last minute")
)

// sendState is what a session remembers: the hosts that were allowed, the recent calls, and whether one is running. Nothing in it is
// written to disk.
type sendState struct {
	mu       sync.Mutex
	allowed  map[string]bool // "host:port", lower case, allowed by a person in this session
	recent   []time.Time
	inflight bool
	// headless is true in the browser-driven build, which refuses an address on this machine or its network unless testAllowLocal names
	// the host. The native app has no such list: it asks the person, with a warning, and the person's allow is the decision.
	headless       bool
	testAllowLocal map[string]bool
	// maxPerMinute is sendMaxPerMinute; the headless test build can raise it (PROBE_STUDIO_TEST_SEND_RATE) so a long browser suite sharing one
	// server does not trip the limit. Zero means the default.
	maxPerMinute int
	now          func() time.Time
	resolver     httpcall.Resolver
}

func (s *sendState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// hostKey is the form hosts are remembered in.
func hostKey(host, port string) string { return strings.ToLower(host) + ":" + port }

// allow remembers a host:port as allowed for the rest of the session.
func (s *sendState) allow(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.allowed == nil {
		s.allowed = map[string]bool{}
	}
	s.allowed[key] = true
}

func (s *sendState) isAllowed(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowed[key]
}

// begin takes the one running slot and counts the call against the limit per minute.
func (s *sendState) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight {
		return errSendBusy
	}
	cutoff := s.clock().Add(-time.Minute)
	kept := s.recent[:0]
	for _, t := range s.recent {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.recent = kept
	limit := sendMaxPerMinute
	if s.maxPerMinute > 0 {
		limit = s.maxPerMinute
	}
	if len(s.recent) >= limit {
		return errSendRate
	}
	s.recent = append(s.recent, s.clock())
	s.inflight = true
	return nil
}

func (s *sendState) end() {
	s.mu.Lock()
	s.inflight = false
	s.mu.Unlock()
}

// policy is what httpcall asks about every address that is about to be dialed, after the name is resolved: the host and port must have
// been allowed by the person in this session (the decision is made here and not only in the window), and the kind of address must be
// one that may be called.
func (s *sendState) policy(host string, port int, ip net.IP) error {
	if !s.isAllowed(hostKey(host, strconv.Itoa(port))) {
		return errSendHostNotAllowed
	}
	return s.classPolicy(host, ip)
}

// classPolicy decides by the kind of address. The cloud metadata addresses, the unspecified address and multicast are never allowed. An
// address on this machine or its network, or in a reserved range, is allowed in the native app (the person was warned and allowed the
// host) and, in the headless build, only for a host the test allow-list names.
func (s *sendState) classPolicy(host string, ip net.IP) error {
	switch class := httpcall.ClassOf(ip); class {
	case httpcall.Metadata, httpcall.Unspecified, httpcall.Multicast:
		return fmt.Errorf("%s is never allowed", class)
	case httpcall.Public:
		return nil
	default:
		s.mu.Lock()
		headless, listed := s.headless, s.testAllowLocal[strings.ToLower(host)]
		s.mu.Unlock()
		if headless && !listed {
			return fmt.Errorf("%s is refused in this build", class)
		}
		return nil
	}
}

// parseTestAllowLocal reads PROBE_STUDIO_TEST_ALLOW_LOCAL: host names, comma separated, for the headless build only.
func parseTestAllowLocal(headless bool, env string) map[string]bool {
	if !headless || strings.TrimSpace(env) == "" {
		return nil
	}
	out := map[string]bool{}
	for _, h := range strings.Split(env, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out[h] = true
		}
	}
	return out
}

// resolveForDisplay looks a host up once, for the warning shown before it is allowed.
func (s *sendState) resolveForDisplay(host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	r := s.resolver
	if r == nil {
		r = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendResolveTime)
	defer cancel()
	addrs, err := r.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, errors.New("the name could not be resolved")
	}
	var out []net.IP
	for _, a := range addrs {
		out = append(out, a.IP)
	}
	return out, nil
}

// parseIP reads an address from text.
func parseIP(s string) (net.IP, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, errors.New("not an address")
	}
	return ip, nil
}

// parseTestSendRate reads PROBE_STUDIO_TEST_SEND_RATE. It is honoured only in the headless build and only for a whole number from 1 to
// 1000; anything else keeps the default.
func parseTestSendRate(headless bool, env string) int {
	if !headless {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(env))
	if err != nil || n < 1 || n > 1000 {
		return 0
	}
	return n
}
