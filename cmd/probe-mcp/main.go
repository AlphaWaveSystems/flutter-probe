// Command probe-mcp is the standalone MCP (Model Context Protocol) server
// for FlutterProbe. It speaks JSON-RPC 2.0 over stdio and exposes the same
// tools available via `probe mcp-server` (which now delegates here).
//
// Configure your MCP client (Claude Desktop, Cursor, etc.) to point at this
// binary directly:
//
//	{
//	  "mcpServers": {
//	    "flutter-probe": {
//	      "command": "probe-mcp"
//	    }
//	  }
//	}
//
// When packaged as a Claude Desktop Extension (.mcpb bundle), users select
// their Flutter project directory at install time and the host passes it
// via PROBE_PROJECT_DIR. probe-mcp chdirs into it on startup so tools like
// run_tests, list_files, and get_report find probe.yaml and tests/.
//
// With --http host:port the same tools are served over HTTP instead of stdio
// (see usage below and internal/mcp/http.go). Without it nothing changes.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alphawavesystems/flutter-probe/internal/mcp"
)

// Version is set at build time via -ldflags, e.g.
//
//	go build -ldflags="-X main.Version=0.9.4" ./cmd/probe-mcp
var Version = "dev"

func main() {
	mcp.Version = Version

	// Honor PROBE_PROJECT_DIR — set by .mcpb installs from the user_config
	// directory picker so tools resolve relative paths (probe.yaml, tests/)
	// against the user's Flutter project rather than Claude Desktop's cwd.
	if dir := os.Getenv("PROBE_PROJECT_DIR"); dir != "" {
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintf(os.Stderr, "probe-mcp: PROBE_PROJECT_DIR=%q: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// Take the HTTP token out of the environment on every path: the tools start child processes
	// (probe test, adb), and they have no business with it.
	token := takeToken()

	if code, handled := runArgs(os.Args[1:], token, os.Stderr); handled {
		os.Exit(code)
	}

	if err := mcp.NewServer().Run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe-mcp:", err)
		os.Exit(1)
	}
}

// takeToken returns PROBE_MCP_HTTP_TOKEN and removes it from this process's environment, so that
// no child process inherits it.
func takeToken() string {
	token := os.Getenv(mcp.TokenEnv)
	_ = os.Unsetenv(mcp.TokenEnv)
	return token
}

const usage = `probe-mcp is the FlutterProbe MCP server.

  probe-mcp                         serve MCP over stdio (the default; what MCP clients launch)
  probe-mcp --http 127.0.0.1:7391   serve the same tools over HTTP at http://127.0.0.1:7391/mcp

HTTP transport:
  --http host:port
        Listen address. The host must be a loopback address: 127.0.0.1:7391,
        [::1]:7391 (IPv6, in brackets) or localhost:7391 (taken as 127.0.0.1). A host
        name or any other address is refused. Port 0 picks a free port, printed on
        stderr. Every request needs "Authorization: Bearer <token>".
        A new token is made on every start and written to a file only you can read
        (mode 0600); its path is printed on stderr, the token is not. To keep one token
        across starts, set PROBE_MCP_HTTP_TOKEN (at least 43 random characters, at least
        16 different ones) and no file is written.
  --dangerously-allow-non-loopback
        Allow --http on an address other machines can reach. The server runs tests and
        reads and writes files for anyone who holds the token, and the connection is
        not encrypted. Leave this off unless the network is yours alone.

Environment:
  PROBE_PROJECT_DIR      directory to run in (probe.yaml, tests/)
  PROBE_MCP_HTTP_TOKEN   bearer token for --http instead of a generated one
`

const usageLine = "usage: probe-mcp [--http host:port [--dangerously-allow-non-loopback]]   (probe-mcp --help for more)"

// knownFlag reports whether a dash-prefixed argument is one of the options.
func knownFlag(arg string) bool {
	name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	return name == "http" || name == "dangerously-allow-non-loopback" || name == "h" || name == "help"
}

// runArgs looks at the arguments. With none, or with words only, it reports handled = false and the
// caller serves stdio, as it always did: an existing client configuration behaves the same. An
// option that is not known (a mistyped --http, say) is an error, exit code 2: starting stdio in its
// place would look like a server that hangs.
func runArgs(args []string, token string, stderr io.Writer) (code int, handled bool) {
	flags := false
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || a == "-" {
			continue
		}
		if !knownFlag(a) {
			fmt.Fprintf(stderr, "probe-mcp: unknown option %s\n%s\n", a, usageLine)
			return 2, true
		}
		flags = true
	}
	if !flags {
		return 0, false
	}
	return runWithFlags(args, token, stderr), true
}

// runWithFlags handles --help and --http and returns the exit code.
func runWithFlags(args []string, token string, stderr io.Writer) int {
	fs := flag.NewFlagSet("probe-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	addr := fs.String("http", "", "")
	allow := fs.Bool("dangerously-allow-non-loopback", false, "")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *addr == "" {
		fmt.Fprintf(stderr, "probe-mcp: --http needs an address before any other word, for example --http 127.0.0.1:7391\n%s\n", usageLine)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := mcp.RunHTTP(ctx, mcp.HTTPOptions{Addr: *addr, AllowNonLoopback: *allow, Token: token, Log: stderr})
	if err != nil {
		fmt.Fprintln(stderr, "probe-mcp:", err)
		return 1
	}
	return 0
}
