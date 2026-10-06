# cmd

Entry points.

| Directory | Binary | What |
|---|---|---|
| `probe/` | `probe` | The CLI: `probe test`, `lint`, `record`, `report`, `generate`, `ai doctor`, `triage`, `studio`, ... |
| `probe-mcp/` | `probe-mcp` | Standalone MCP server (JSON-RPC 2.0 over stdio) exposing FlutterProbe to AI agents |

The logic lives under `internal/`. Build with `go build ./cmd/probe` or `make install`.
