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
	sendCheckPerMin  = 30 // CheckTarget looks a name up, so it is limited too
	sendMaxAllowed   = 100
	sendResolveTime  = 3 * time.Second
	levelInternet    = 1 // an address anyone can reach
	levelLocal       = 2 // this machine, a private network, link-local, a reserved range
	levelNever       = 3 // metadata, unspecified, multicast
)

// levelOf is how strong an allow an address needs. An allow made for the internet never covers an address on this machine or its
// network, and an allow for those never covers a kind that is never allowed.
func levelOf(class httpcall.Class) int {
	switch class {
	case httpcall.Public:
		return levelInternet
	case httpcall.Loopback, httpcall.Private, httpcall.LinkLocal, httpcall.Reserved:
		return levelLocal
	}
	return levelNever
}

var (
	errSendCheckRate      = errors.New("too many address checks in the last minute")
	errSendNotChecked     = errors.New("that host was not checked first: Studio shows where it leads before it is allowed")
	errSendHostNotAllowed = errors.New("that host has not been allowed in this session: confirm it first")
	errSendBusy           = errors.New("another request is still running")
	errSendRate           = errors.New("too many requests in the last minute")
)

// sendState is what a session remembers: the hosts that were allowed, the recent calls, and whether one is running. Nothing in it is
// written to disk.
type sendState struct {
	mu       sync.Mutex
	allowed  map[string]int // host:port in the normal form -> the level the person was shown and allowed
	order    []string       // allowed keys, oldest first, for eviction
	shown    map[string]int // host:port -> the level CheckTarget last showed for it: an allow is for what was shown
	shownOrd []string
	lookups  []time.Time // CheckTarget calls
	recent   []time.Time
	inflight bool
	refusal  string // why the policy refused the last address it refused, for the sentence a person reads
	// headless is true in the browser-driven build, which refuses an address on this machine or its network unless testAllowLocal names
	// the host. The native app has no such list: it asks the person, with a warning, and the person's allow is the decision.
	headless       bool
	testAllowLocal map[string]bool
	// maxPerMinute is sendMaxPerMinute; the headless test build can raise it (PROBE_STUDIO_TEST_SEND_RATE) so a long browser suite sharing one
	// server does not trip the limit. Zero means the default.
	maxPerMinute int
	now          func() time.Time
	resolver     httpcall.Resolver
	dial         func(ctx context.Context, network, address string) (net.Conn, error) // tests only: answers without a real connection
}

func (s *sendState) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// note remembers what CheckTarget showed for a host:port: the strongest kind among its addresses.
func (s *sendState) note(key string, level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shown == nil {
		s.shown = map[string]int{}
	}
	if _, ok := s.shown[key]; !ok {
		s.shownOrd = append(s.shownOrd, key)
		if len(s.shownOrd) > sendMaxAllowed {
			delete(s.shown, s.shownOrd[0])
			s.shownOrd = s.shownOrd[1:]
		}
	}
	s.shown[key] = level
}

// allow remembers a host:port as allowed for the rest of the session, at the level CheckTarget last showed for it. A host that was not
// checked, or whose check found an address that is never allowed, cannot be allowed.
func (s *sendState) allow(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	level, ok := s.shown[key]
	if !ok || level >= levelNever {
		return errSendNotChecked
	}
	if s.allowed == nil {
		s.allowed = map[string]int{}
	}
	if _, had := s.allowed[key]; !had {
		s.order = append(s.order, key)
		if len(s.order) > sendMaxAllowed {
			delete(s.allowed, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.allowed[key] = level
	return nil
}

// allowedLevel is the level a host:port was allowed at, or 0.
func (s *sendState) allowedLevel(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowed[key]
}

func (s *sendState) isAllowed(key string) bool { return s.allowedLevel(key) > 0 }

// checkAllowed takes one of the address checks of this minute.
func (s *sendState) checkAllowed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.clock().Add(-time.Minute)
	kept := s.lookups[:0]
	for _, t := range s.lookups {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.lookups = kept
	if len(s.lookups) >= sendCheckPerMin {
		return errSendCheckRate
	}
	s.lookups = append(s.lookups, s.clock())
	return nil
}

// refuse records why the policy said no and returns the error httpcall turns into a PolicyError.
func (s *sendState) refuse(reason string) error {
	s.mu.Lock()
	s.refusal = reason
	s.mu.Unlock()
	return errors.New("refused")
}

// takeRefusal returns the reason of the last refusal, once.
func (s *sendState) takeRefusal() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.refusal
	s.refusal = ""
	return r
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

// policy is what httpcall asks about every address that is about to be dialed, after the name is resolved. The host and port must have
// been allowed by the person in this session, and the kind of the address must not be stronger than the kind the person was shown and
// allowed: a name that led to the internet when it was allowed and leads to this machine now is refused (DNS rebinding after the yes).
// The cloud metadata addresses, the unspecified address and multicast are never allowed.
func (s *sendState) policy(host string, port int, ip net.IP) error {
	allowed := s.allowedLevel(hostKey(host, strconv.Itoa(port)))
	if allowed == 0 {
		return s.refuse("that host has not been allowed in this session")
	}
	class := httpcall.ClassOf(ip)
	level := levelOf(class)
	switch {
	case level >= levelNever:
		return s.refuse("it leads to " + class.String() + ", which is never allowed")
	case level > allowed:
		return s.refuse("it was allowed as the internet, but now leads to " + class.String() + ": check it again")
	}
	return s.classPolicy(host, ip)
}

// classPolicy decides by the kind of address, for what the check shows and for the headless build: an address on this machine or its
// network, or in a reserved range, is allowed in the native app (the person was warned and allowed the host) and, in the headless build,
// only for a host the test allow-list names. The cloud metadata addresses, the unspecified address and multicast are never allowed.
func (s *sendState) classPolicy(host string, ip net.IP) error {
	switch class := httpcall.ClassOf(ip); levelOf(class) {
	case levelNever:
		return fmt.Errorf("%s is never allowed", class)
	case levelInternet:
		return nil
	default:
		s.mu.Lock()
		headless, listed := s.headless, s.testAllowLocal[strings.ToLower(host)]
		s.mu.Unlock()
		if headless && !listed {
			return s.refuse(class.String() + " is refused in this build")
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
