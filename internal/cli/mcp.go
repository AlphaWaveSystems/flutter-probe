package cli

import (
	"fmt"
	"os"

	"github.com/alphawavesystems/flutter-probe/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "Start the ProbeScript MCP server (stdio transport) — deprecated, use `probe-mcp` instead",
	Long: `Start a Model Context Protocol (MCP) server that exposes FlutterProbe
capabilities as tools callable by AI agents (Claude, GPT-4, etc.).

DEPRECATED: as of v0.6.0, the MCP server ships as a separate binary `+"`probe-mcp`"+`.
Update your MCP client configuration to call that binary directly:

  {
    "mcpServers": {
      "flutter-probe": {
        "command": "probe-mcp"
      }
    }
  }

This subcommand is kept for backwards compatibility and runs the same server
embedded in the main `+"`probe`"+` binary. It will be removed in a future release.`,
	// SilenceErrors keeps stdout clean for MCP JSON-RPC traffic; deprecation
	// notice is written to stderr only.
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(os.Stderr,
			"⚠  `probe mcp-server` is deprecated. Configure your MCP client to call `probe-mcp` directly.")
		return mcp.NewServer().Run()
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
