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
package main

import (
	"fmt"
	"os"

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

	if err := mcp.NewServer().Run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe-mcp:", err)
		os.Exit(1)
	}
}
