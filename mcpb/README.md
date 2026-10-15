# mcpb

Packaging for the **Claude Desktop Extension** (`.mcpb`) that bundles the `probe-mcp` server.

`template/manifest.json` is the extension manifest. `scripts/build-mcpb.sh` fills it in with the platform
binary and version, and CI attaches one `.mcpb` per platform (darwin-arm64, darwin-amd64, linux-amd64,
windows-amd64) to every GitHub release.

The extension exposes the FlutterProbe MCP tools (35 in 0.26.0, 31 in 0.25.2): device lifecycle, test authoring and
execution, reporting, and the optional `triage_failure` helper. Full list and install steps:
[flutterprobe.dev/tools/mcp](https://flutterprobe.dev/tools/mcp/).
