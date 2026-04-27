// Command probe-mcp is the standalone MCP (Model Context Protocol) server
// for FlutterProbe. It speaks JSON-RPC 2.0 over stdio and exposes the same
// 10 tools available via `probe mcp-server` (which now delegates here).
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
package main

import (
	"fmt"
	"os"

	"github.com/alphawavesystems/flutter-probe/internal/mcp"
)

// Version is set at build time via -ldflags, e.g.
//
//	go build -ldflags="-X main.Version=0.6.0" ./cmd/probe-mcp
var Version = "dev"

func main() {
	mcp.Version = Version
	if err := mcp.NewServer().Run(); err != nil {
		fmt.Fprintln(os.Stderr, "probe-mcp:", err)
		os.Exit(1)
	}
}
