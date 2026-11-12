package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// The bearer token of the HTTP transport.
//
// By default a new token (32 bytes from crypto/rand, hex) is made on every start and written to
// <user config dir>/flutter-probe-mcp/http-token, mode 0600 in a 0700 directory. The file is
// removed when the server stops. A token that changes on every start is the safer default: one
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
	tokenFileName = "http-token"
	tokenBytes    = 32
	// minSuppliedToken is the shortest supplied token accepted: 32 bytes in base64 are 43 characters.
	minSuppliedToken = 43
)

// permsEnforced is false where the file mode does not describe who can read a file.
var permsEnforced = runtime.GOOS != "windows"

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
		return fmt.Errorf("%s is too short: use at least %d characters from a random source, for example `openssl rand -hex 32`", TokenEnv, minSuppliedToken)
	}
	for i := 0; i < len(token); i++ {
		if c := token[i]; c <= 0x20 || c >= 0x7f {
			return fmt.Errorf("%s must be printable ASCII without spaces", TokenEnv)
		}
	}
	return nil
}

// writeTokenFile writes the token to dir/http-token and returns the path. It refuses a directory or
// an existing token file that is a symlink or that other users can reach, instead of repairing it:
// a token location someone else set up is not one to trust. The token is written to a new 0600
// file that is then renamed into place, so the write never follows a link at the final name.
func writeTokenFile(dir, token string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mcp http: token directory: %w", err)
	}
	di, err := os.Lstat(dir)
	if err != nil {
		return "", fmt.Errorf("mcp http: token directory: %w", err)
	}
	if di.Mode()&os.ModeSymlink != 0 || !di.IsDir() {
		return "", fmt.Errorf("mcp http: token directory %s is a symlink or not a directory; refusing to use it", dir)
	}
	if permsEnforced && di.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("mcp http: token directory %s is open to other users (mode %04o); run `chmod 700` on it or remove it", dir, di.Mode().Perm())
	}

	path := filepath.Join(dir, tokenFileName)
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
			return "", fmt.Errorf("mcp http: token file %s is a symlink or not a regular file; refusing to use it. Remove it and start again", path)
		}
		if permsEnforced && fi.Mode().Perm()&0o077 != 0 {
			return "", fmt.Errorf("mcp http: token file %s is open to other users (mode %04o); refusing to use it. Remove it and start again", path, fi.Mode().Perm())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("mcp http: token file: %w", err)
	}

	tmp, err := os.CreateTemp(dir, tokenFileName+".*") // created 0600, with O_EXCL
	if err != nil {
		return "", fmt.Errorf("mcp http: token file: %w", err)
	}
	_, werr := tmp.WriteString(token + "\n")
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), 0o600)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("mcp http: token file: %w", werr)
	}
	return path, nil
}

// removeTokenFile removes the token file on shutdown, but only while it still holds this server's
// token: a second server started later has put its own token there.
func removeTokenFile(path, token string) {
	if b, err := os.ReadFile(path); err == nil && string(b) == token+"\n" {
		_ = os.Remove(path)
	}
}
