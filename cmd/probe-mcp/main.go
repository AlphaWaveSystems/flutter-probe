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

	if wantsFlags(os.Args[1:]) {
		os.Exit(runWithFlags(os.Args[1:], os.Stderr))
	}

	if err := mcp.NewServer().Run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe-mcp:", err)
		os.Exit(1)
	}
}

const usage = `probe-mcp is the FlutterProbe MCP server.

  probe-mcp                         serve MCP over stdio (the default; what MCP clients launch)
  probe-mcp --http 127.0.0.1:7391   serve the same tools over HTTP at http://127.0.0.1:7391/mcp

HTTP transport:
  --http host:port
        Listen address. The host must be 127.0.0.1, ::1 or localhost; port 0 picks a free
        port, printed on stderr. Every request needs "Authorization: Bearer <token>".
        A new token is made on every start and written to a file only you can read
        (mode 0600); its path is printed on stderr, the token is not. To keep one token
        across starts, set PROBE_MCP_HTTP_TOKEN (at least 43 random characters) and no
        file is written.
  --dangerously-allow-non-loopback
        Allow --http on an address other machines can reach. The server runs tests and
        reads and writes files for anyone who holds the token, and the connection is
        not encrypted. Leave this off unless the network is yours alone.

Environment:
  PROBE_PROJECT_DIR      directory to run in (probe.yaml, tests/)
  PROBE_MCP_HTTP_TOKEN   bearer token for --http instead of a generated one
`

// wantsFlags reports whether the arguments ask for something other than plain stdio. Anything
// else is ignored, as it always was, so an existing client configuration behaves the same.
func wantsFlags(args []string) bool {
	for _, a := range args {
		name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if strings.HasPrefix(a, "-") && (name == "http" || name == "dangerously-allow-non-loopback" || name == "h" || name == "help") {
			return true
		}
	}
	return false
}

// runWithFlags handles --help and --http and returns the exit code.
func runWithFlags(args []string, stderr io.Writer) int {
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
		fmt.Fprintln(stderr, "probe-mcp: --http needs an address, for example --http 127.0.0.1:7391")
		return 2
	}

	// Take the token out of the environment: the tools start child processes (probe test, adb),
	// and they have no business with it.
	token := os.Getenv(mcp.TokenEnv)
	_ = os.Unsetenv(mcp.TokenEnv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := mcp.RunHTTP(ctx, mcp.HTTPOptions{Addr: *addr, AllowNonLoopback: *allow, Token: token, Log: stderr})
	if err != nil {
		fmt.Fprintln(stderr, "probe-mcp:", err)
		return 1
	}
	return 0
}
