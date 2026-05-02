---
title: MCP Server
description: Connect FlutterProbe to Claude Desktop, Cursor, and other AI agents via the Model Context Protocol.
---

FlutterProbe ships an MCP (Model Context Protocol) server as a standalone binary called `probe-mcp` that exposes your test suite as tools callable by AI agents. Once connected, Claude Desktop, Cursor, or any MCP-compatible client can read the live widget tree, write tests, run them, and inspect results — all without leaving the AI chat.

> **As of v0.6.0**, the MCP server is a separate binary `probe-mcp`. The legacy `probe mcp-server` subcommand still works (it runs the same code embedded in the main binary) but prints a deprecation notice on stderr. Update your MCP client configuration to point at `probe-mcp` directly.

## What the MCP server exposes

### Device lifecycle

| Tool | Description |
|---|---|
| `list_devices` | List booted/connected simulators, emulators, and physical devices (id, name, platform, state, OS version) |
| `list_simulators` | List all iOS simulators (booted + shutdown) so the agent can pick one to boot |
| `list_avds` | List Android Virtual Device names available to launch |
| `start_device` | Boot an Android emulator (by AVD name) or iOS simulator (by UDID) |
| `shutdown_device` | Shut down an iOS simulator by UDID |

### Test authoring & execution

| Tool | Description |
|---|---|
| `get_widget_tree` | Dump the live Flutter widget tree from the running app |
| `take_screenshot` | Capture the current screen and return it as an image |
| `read_test` | Read the contents of a `.probe` file |
| `write_test` | Create or overwrite a `.probe` file |
| `run_script` | Execute inline ProbeScript without creating a file |
| `run_tests` | Run `.probe` test files against the connected app |
| `list_files` | List all `.probe` files in a directory |
| `lint` | Validate `.probe` file syntax without running |
| `get_report` | Read the latest JSON test run report |
| `generate_test` | AI-generate a test from a natural language prompt |

`get_widget_tree`, `take_screenshot`, `run_script`, and `run_tests` accept an optional `device` argument (serial or UDID) so the agent can pin a specific target when more than one device is connected.

The workflow this enables: `list_devices` → `start_device` (if needed) → `get_widget_tree` → `write_test` → `run_tests` → `get_report` — a complete AI-driven test authoring loop, including device bring-up.

## Requirements

- `probe-mcp` binary v0.6.0+ installed and in `PATH` (legacy `probe mcp-server` v0.5.7+ also works but is deprecated)
- Flutter app running with `--dart-define=PROBE_AGENT=true` (for live tools like `get_widget_tree`, `take_screenshot`, `run_tests`)
- An MCP-compatible client (Claude Desktop, Cursor, etc.)

Verify the binary is accessible:

```bash
which probe-mcp   # should print /opt/homebrew/bin/probe-mcp or similar
```

`probe-mcp` reads JSON-RPC 2.0 messages from stdin and writes responses to stdout, so it has no `--version` flag of its own — verify by sending an `initialize` request (see [Verifying the connection](#verifying-the-connection)).

## Claude Desktop

### macOS

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "probe-mcp"
    }
  }
}
```

If `probe-mcp` is not in your shell `PATH` (common when Claude Desktop doesn't inherit your shell environment), use the absolute path:

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "/opt/homebrew/bin/probe-mcp"
    }
  }
}
```

Find the absolute path with:

```bash
which probe-mcp
```

### Windows

Edit `%APPDATA%\Claude\claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "probe-mcp.exe"
    }
  }
}
```

After editing the config, **restart Claude Desktop**. The flutter-probe tools appear in the tool picker (hammer icon) in any new conversation.

## Cursor

In Cursor, open **Settings → MCP** (or edit `.cursor/mcp.json` at the repo root):

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "probe-mcp"
    }
  }
}
```

Restart Cursor after saving.

## Migrating from `probe mcp-server`

If you previously used the legacy subcommand, change:

```json
{ "command": "probe", "args": ["mcp-server"] }
```

to:

```json
{ "command": "probe-mcp" }
```

The tools, protocol, and behavior are identical. The legacy form continues to work in v0.6.0 but prints a deprecation notice and will be removed in a future release.

## Other MCP clients

Any client that supports the MCP stdio transport works. The server command is:

```bash
probe-mcp
```

It reads JSON-RPC 2.0 messages from stdin and writes responses to stdout, one message per line (newline-delimited).

## Working directory

The MCP server inherits the working directory from the client. For tools like `run_tests`, `list_files`, and `get_report` to find your test files, the working directory must be your Flutter project root (the folder containing `probe.yaml` and `tests/`).

Claude Desktop launches the server with its own working directory, which may not be your project. Pass the correct directory explicitly:

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "probe-mcp",
      "env": {
        "PWD": "/Users/you/dev/my-flutter-app"
      }
    }
  }
}
```

Or use the `cwd` field if your client supports it:

```json
{
  "mcpServers": {
    "flutter-probe": {
      "command": "probe-mcp",
      "cwd": "/Users/you/dev/my-flutter-app"
    }
  }
}
```

## Verifying the connection

Test the server manually in your terminal:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | probe-mcp
```

Expected response (version reflects the installed binary):

```json
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"probe-mcp","version":"0.6.0"}}}
```

List all available tools:

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | probe-mcp
```

## Example AI session

Once connected, you can prompt Claude to:

> "Look at the widget tree and write a test that verifies the login flow works."

Claude will:
1. Call `get_widget_tree` to inspect the running app's UI
2. Call `take_screenshot` to see what is on screen
3. Call `write_test` to create `tests/login.probe`
4. Call `run_tests` to execute it
5. Call `get_report` to verify all steps passed

### Device bring-up from chat

You can also let Claude pick and start a simulator before the app is running:

> "Boot an iOS simulator and run the smoke tests on it."

Claude will:
1. Call `list_simulators` to discover available UDIDs
2. Call `start_device` with `{platform: "ios", udid: "<chosen>"}`
3. Call `list_devices` to confirm it came online
4. Call `run_tests` with `{tag: "smoke", device: "<udid>"}` to pin the run to the just-booted sim

## Troubleshooting

### Tools don't appear in Claude Desktop

- Restart Claude Desktop after editing the config
- Verify the JSON is valid (no trailing commas)
- Check Claude Desktop logs: `~/Library/Logs/Claude/`

### `probe-mcp: command not found`

Claude Desktop may not inherit your shell `PATH`. Use the absolute binary path:

```bash
which probe-mcp   # copy this output
```

Then set `"command": "/absolute/path/to/probe-mcp"` in the config.

### `get_widget_tree` / `take_screenshot` return errors

These tools require the Flutter app to be running and connected to the probe agent. Start the app first:

```bash
flutter run --dart-define=PROBE_AGENT=true
```

Then confirm the agent is reachable:

```bash
probe test --dry-run
```

### Screenshots saved to wrong directory

The MCP server looks for screenshots in `reports/screenshots/` relative to its working directory. Set `cwd` in your MCP config to your project root (see [Working directory](#working-directory) above).
