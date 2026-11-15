package main

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// Hosts are remembered, shown to Go's checks and dialed in one form: ASCII (a name in punycode), lower case, no trailing dot, an IPv6
// literal without its zone, and host and port joined the way net.JoinHostPort does (an IPv6 literal in brackets). The unicode form of a
// name is only a label for the person.

const (
	sendMaxHostBytes = 253 // the longest name DNS has
	sendMaxKeyBytes  = sendMaxHostBytes + len("[]:65535")
)

// asciiHost returns the normal form of a host, or an error.
func asciiHost(host string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "" || len(host) > sendMaxHostBytes*4 || strings.ContainsAny(host, "%/@ \t\r\n\x00") {
		return "", errors.New("the host is not valid")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" || len(ascii) > sendMaxHostBytes {
		return "", errors.New("the host is not valid")
	}
	return strings.ToLower(ascii), nil
}

// hostKey is the form hosts are remembered in: host and port of the normal form.
func hostKey(host, port string) string { return net.JoinHostPort(host, port) }

// parseKey reads a key back: the host must already be in the normal form and the port a number from 1 to 65535.
func parseKey(key string) (host, port string, err error) {
	if len(key) > sendMaxKeyBytes {
		return "", "", errors.New("not a host and port")
	}
	host, port, err = net.SplitHostPort(strings.TrimSpace(key))
	if err != nil {
		return "", "", errors.New("not a host and port")
	}
	if n, perr := strconv.Atoi(port); perr != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return "", "", errors.New("not a host and port")
	}
	norm, herr := asciiHost(host)
	if herr != nil || norm != host {
		return "", "", errors.New("not a host and port")
	}
	return host, port, nil
}
