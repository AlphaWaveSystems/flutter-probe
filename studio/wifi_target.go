package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alphawavesystems/flutter-probe/internal/probelink"
)

// defaultAgentPort is the port the agent listens on unless the app moved it.
const defaultAgentPort = 48686

var (
	hostLabel  = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	tokenChars = regexp.MustCompile(`^[A-Za-z0-9_.~-]{8,128}$`)
)

// ValidateWiFiTarget checks what a person typed into the manual WiFi entry before anything is dialled.
// The host is an IPv4 address, an IPv6 address or a hostname (such as an mDNS name ending in .local); the port is
// 1 to 65535; the token has the characters the agent can produce (letters, digits and _ . ~ -), 8 to 128 of them.
// The returned messages are meant to be shown as they are.
func ValidateWiFiTarget(host string, port int, token string) error {
	host = strings.TrimSpace(host)
	switch {
	case host == "":
		return errors.New("enter the phone's IP address or hostname (Settings > Wi-Fi > the network's (i) button on an iPhone)")
	case strings.Contains(host, "://") || strings.ContainsAny(host, "/ \t") || strings.Contains(host, "?"):
		return fmt.Errorf("host %q must be an address or hostname only: no scheme (ws://), path or spaces, and no port (use the port field)", host)
	}
	if !validHost(host) {
		return fmt.Errorf("host %q is not a valid IPv4 address, IPv6 address or hostname", host)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d is out of range: use 1 to 65535 (the agent default is %d)", port, defaultAgentPort)
	}
	if !tokenChars.MatchString(strings.TrimSpace(token)) {
		return errors.New("the token must be 8 to 128 characters of letters, digits and _ . ~ -: copy the value after PROBE_TOKEN= from the app's console, without the PROBE_TOKEN= part")
	}
	return nil
}

func validHost(h string) bool {
	h = strings.TrimSuffix(strings.Trim(h, "[]"), ".")
	if h == "" {
		return false
	}
	if ip := net.ParseIP(strings.Split(h, "%")[0]); ip != nil {
		return true // IPv4, or IPv6 with an optional zone such as fe80::1%en0
	}
	if len(h) > 253 {
		return false
	}
	// A string of digits and dots that is not an IPv4 address is a typo, not a hostname.
	if strings.Trim(h, "0123456789.") == "" {
		return false
	}
	for _, l := range strings.Split(h, ".") {
		if !hostLabel.MatchString(l) {
			return false
		}
	}
	return true
}

// wifiHint turns a dial or ping failure into what to check, in the order that finds the cause fastest.
func wifiHint(host string, port int, err error) string {
	checks := fmt.Sprintf("Check: the phone and this computer are on the same network and can reach each other (try `curl http://%s/probe/status`); "+
		"the app is running in the foreground with the screen unlocked; it is a profile build made with --dart-define=PROBE_AGENT=true and --dart-define=PROBE_WIFI=true "+
		"(a production or debug build does not listen); the port is the one the app printed (PROBE_PORT=, default %d); the token is the latest PROBE_TOKEN= line (a new one is created whenever the app restarts).",
		net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)), defaultAgentPort)
	s := ""
	if err != nil {
		s = err.Error()
	}
	switch {
	case strings.Contains(s, "unauthorized") || strings.Contains(s, "401") || strings.Contains(s, "forbidden") || strings.Contains(s, "403"):
		return "The agent answered but refused the token. Use the latest PROBE_TOKEN= line from the app's console: it changes whenever the app restarts."
	case strings.Contains(s, "refused"):
		return "Nothing is listening on that port. " + checks
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline") || strings.Contains(s, "no route"):
		return "The phone did not answer in time. " + checks
	}
	return checks
}

// TestWiFi checks that an agent is reachable and accepts the token, then closes the connection again: it does
// not make it the active connection. It is the "test before connecting" step of the manual entry.
func (a *App) TestWiFi(host string, port int, token string) (string, error) {
	host, token = strings.TrimSpace(host), strings.TrimSpace(token)
	if err := ValidateWiFiTarget(host, port, token); err != nil {
		return "", err
	}
	if err := a.wifiLimit.allow(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	if err := a.checkWiFiTarget(ctx, host); err != nil {
		return "", err
	}
	client, err := probelink.DialWithOptions(ctx, probelink.DialOptions{
		Host: strings.Trim(host, "[]"), Port: port, Token: token, DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return "", fmt.Errorf("%s", redactToken(fmt.Sprintf("could not reach an agent at %s:%d: %v. %s", host, port, err, wifiHint(host, port, err)), token))
	}
	defer client.Close()
	if err := client.Ping(ctx); err != nil {
		return "", fmt.Errorf("%s", redactToken(fmt.Sprintf("the agent at %s:%d did not answer: %v. %s", host, port, err, wifiHint(host, port, err)), token))
	}
	return fmt.Sprintf("Agent reachable at %s:%d and the token is accepted.", host, port), nil
}
