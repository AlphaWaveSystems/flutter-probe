package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/alphawavesystems/flutter-probe/internal/secretfile"
)

// The bearer token of the HTTP transport.
//
// By default a new token (32 bytes from crypto/rand, hex) is made on every start and written to
// <user config dir>/flutter-probe-mcp/http-token-<address>-<port>, mode 0600 in a 0700 directory, one
// file for each address and port a server listens on, so two servers of one user never overwrite each other's token. The
// file is removed when the server stops. A token that changes on every start is the safer default: one
// that leaked (a shell history, a screen share, a copied config) stops working with the process.
// Someone who wants a token that stays the same supplies their own in PROBE_MCP_HTTP_TOKEN; then
// nothing is written to disk.
//
// The same conventions as the Studio automation endpoint: 32 random bytes in hex, a 0600 file in a
// 0700 directory under the user config dir, the path (never the token) on stderr.

const (
	// TokenEnv names the environment variable that supplies the token.
	TokenEnv = "PROBE_MCP_HTTP_TOKEN"

	tokenDirName  = "flutter-probe-mcp"
	tokenFileBase = "http-token"
	tokenBytes    = 32
	// minSuppliedToken is the shortest supplied token accepted: 32 bytes in base64 are 43 characters.
	minSuppliedToken = 43
	// minSuppliedDistinct is a cheap floor against a long token that is one word repeated: it must
	// use at least this many different characters.
	minSuppliedDistinct = 16
)

// DefaultTokenDir is the directory of the token file.
func DefaultTokenDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("mcp http: no user config directory for the token file (set %s instead): %w", TokenEnv, err)
	}
	return filepath.Join(dir, tokenDirName), nil
}

// newToken returns 256 random bits as 64 hex characters.
func newToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mcp http: generate token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// checkSuppliedToken refuses a supplied token that is short or would not survive an HTTP header.
// The message never repeats the token.
func checkSuppliedToken(token string) error {
	if len(token) < minSuppliedToken {
		return fmt.Errorf("%s is too short: use at least %d characters from a random source, for example `openssl rand -base64 32`", TokenEnv, minSuppliedToken)
	}
	var seen [128]bool
	distinct := 0
	for i := 0; i < len(token); i++ {
		c := token[i]
		if c <= 0x20 || c >= 0x7f {
			return fmt.Errorf("%s must be printable ASCII without spaces", TokenEnv)
		}
		if !seen[c] {
			seen[c] = true
			distinct++
		}
	}
	if distinct < minSuppliedDistinct {
		return fmt.Errorf("%s uses too few different characters (at least %d are needed): take it from a random source, for example `openssl rand -base64 32`", TokenEnv, minSuppliedDistinct)
	}
	return nil
}

// tokenFileName is the name of the token file of the server that listens on ip:port: http-token-<address>-<port>, with -z<hex of the zone>- before the
// port for an IPv6 zone. An address and port belong to one server at a time, so the name does too: a server on another port, or on another address with
// the same port (127.0.0.1 and ::1 can both be bound), has its own file, and a second one on the same address and port cannot start (the listen fails
// before any file is touched). Different servers get different names, always: the address is its own text with each colon written as an underscore
// (a colon is not allowed in a file name everywhere; ::1 is __1, and no address text has an underscore or a dash, so two addresses cannot give one text),
// the zone (fe80::1%en0) is written as hexadecimal after a dash, so no zone can look like part of an address, and the segments are told apart by the dashes.
func tokenFileName(ip net.IP, zone string, port int) string {
	addr := strings.ReplaceAll(ip.String(), ":", "_")
	if zone != "" {
		addr += "-z" + hex.EncodeToString([]byte(zone))
	}
	return fmt.Sprintf("%s-%s-%d", tokenFileBase, addr, port)
}

// boundForTokenFile is the address the token file name is made from: the bound one. A variable so a test can give it a zone (one cannot be bound
// on every system) and see that RunHTTP's file name has it.
var boundForTokenFile = func(a *net.TCPAddr) *net.TCPAddr { return a }

// tokenFileForAddr is the token file name of the server bound to addr (what RunHTTP uses): its address, its zone and its port.
func tokenFileForAddr(addr *net.TCPAddr) string { return tokenFileName(addr.IP, addr.Zone, addr.Port) }

// writeTokenFile writes the token to dir/name and returns the path, through the one writer of secret files (internal/secretfile): it refuses a
// directory or an existing token file that is a link, not ours or reachable by other users, instead of repairing it. replaced reports that a token
// file was already there: one left by a server that was killed, or the one a running server still depends on.
func writeTokenFile(dir, name, token string) (path string, replaced bool, err error) {
	return secretfile.Write(secretfile.Options{Label: "mcp http", Dir: dir, Name: name, Content: token + "\n"})
}

// removeTokenFile removes the token file on shutdown, but only while it still holds this server's token: a second server started later has put its
// own token there.
func removeTokenFile(path, token string) { secretfile.Remove(path, token+"\n") }
