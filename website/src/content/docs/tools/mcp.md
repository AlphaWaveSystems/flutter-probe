---
title: MCP Server
description: Connect FlutterProbe to Claude Desktop, Cursor, and other AI agents via the Model Context Protocol.
---

FlutterProbe ships an MCP (Model Context Protocol) server as a standalone binary called `probe-mcp` that exposes your entire test suite as tools callable by AI agents. Once connected, Claude Desktop, Cursor, or any MCP-compatible client can manage devices, write and run tests, record interactions, generate reports, and inspect results — all without leaving the AI chat.

> **As of v0.6.0**, the MCP server is a separate binary `probe-mcp`. The legacy `probe mcp-server` subcommand still works but prints a deprecation notice on stderr. Update your MCP client configuration to point at `probe-mcp` directly.

## What the MCP server exposes

### Device lifecycle (5 tools)

| Tool | Description |
|---|---|
| `list_devices` | List booted/connected simulators, emulators, and physical devices (id, name, platform, state, OS version) |
| `list_simulators` | List all iOS simulators (booted + shutdown) so the agent can pick one to boot |
| `list_avds` | List Android Virtual Device names available to launch |
| `start_device` | Boot an Android emulator (by AVD name) or iOS simulator (by UDID); blocks until online |
| `shutdown_device` | Shut down an iOS simulator (`udid`) or Android emulator (`serial`) |

### Authoring & execution (9 tools)

| Tool | Description |
|---|---|
| `get_widget_tree` | Dump the live Flutter widget tree from the running app |
| `take_screenshot` | Capture the current screen and return it as an image |
| `read_test` | Read the contents of a `.probe` file |
| `write_test` | Create or overwrite a `.probe` file — content is validated before writing; syntax errors are returned without creating the file |
| `run_script` | Execute inline ProbeScript without creating a file |
| `run_tests` | Run `.probe` test files (supports composite multi-device tests via `composite_devices`) |
| `list_files` | List all `.probe` files in a directory |
| `lint` | Validate `.probe` file syntax without running |
| `record` | Record user interactions and generate a `.probe` test file (runs for `timeout` duration, default 30s) |

`get_widget_tree`, `take_screenshot`, `run_script`, and `run_tests` accept an optional `device` argument (serial or UDID) to pin a specific target.

### Reporting & generation (3 tools)

| Tool | Description |
|---|---|
| `get_report` | Read the most recently modified JSON test run report |
| `generate_report` | Generate a standalone HTML report from a JSON results file |
| `generate_test` | AI-generate a `.probe` test from a natural language prompt |

### Project management (1 tool)

| Tool | Description |
|---|---|
| `init_project` | Initialize a new FlutterProbe project (creates `probe.yaml` and `tests/` scaffold) |

The full workflow this enables: `list_devices` → `start_device` (if needed) → `get_widget_tree` → `write_test` → `run_tests` → `get_report` → `generate_report` — a complete AI-driven test authoring loop, including device bring-up and HTML reporting.

## `run_tests` flags

The `run_tests` tool has named parameters for common options (`paths`, `tag`, `device`, `composite_devices`) and a `flags` string for everything else. Key flags an agent should know:

| Flag | What it does |
|---|---|
| `--timeout 60s` | Per-step timeout (default 30s) |
| `-v` | Verbose step output — prints `→ step` before each step and `✓/✗ step (Xs)` after; slow steps (>5s) emit `⏱` progress ticks and a `⚠` warning at 80% of timeout. Recommended when diagnosing failures. |
| `--format json` | Structured JSON results (pipe to `get_report`) |
| `--format junit` | JUnit XML for CI systems |
| `--dry-run` | Validate syntax without a device connection |
| `--parallel` | Distribute tests across all connected devices |
| `--shard 1/3` | Run 1/3 of test files (for CI matrix builds) |
| `--host <ip> --token <t>` | WiFi mode for physical devices |
| `--disable-animations` | Set `timeDilation=0` for faster tests |
| `-y` | Auto-approve destructive operations (CI mode) |
| `--video` | Record device screen during the run |
| `--stream` | Emit one ndjson line per test as it completes (requires `--format json`) |

> **Tip for agents:** pass `-v` in `flags` when a test is failing and you need to understand which step is slow or stuck. The `⏱` progress ticks and `⚠` timeout warnings appear in `run_tests` output and tell you exactly where time is being spent before the failure.

## Composite (multi-device) tests

`write_test` supports the full `composite test` syntax for coordinating multiple devices:

```
composite test "alice sends bob a message"
  devices
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator

  A:
    tap "New Message"
    type "Hello Bob" in "compose"
    tap "Send"

  sync "message sent"

  B:
    wait until "Hello Bob" appears
    see "Hello Bob"
```

Pass device connections to `run_tests` via `composite_devices` (space-separated `ALIAS=SPEC` pairs):

- WiFi: `"A=192.168.1.10:48686/token"` 
- iOS simulator: `"B=A1B2C3D4-E5F6-..."`
- Android: `"C=emulator-5554"`

Or configure them in `probe.yaml` under `composite.devices`.

## Annotation-driven tests (`flutter_probe_annotation` + `flutter_probe_gen`)

When the user's Flutter project uses the new annotation packages
(`flutter_probe_annotation` for the DSL and `flutter_probe_gen` for the
build_runner generator), `.probe` test files appear under
`tests/generated/` after `dart run build_runner build`. The MCP `run_tests`
tool runs them like any other `.probe` file — pass `tests/` as the `paths`
argument.

For agents authoring tests in this style:
- Don't use `write_test` to write to `tests/generated/` directly — those
  files are regenerated. Use `write_test` to author the **annotated Dart
  source** instead (the screen class with `@ProbeSuite`).
- After the user runs `dart run build_runner build`, call `run_tests` as
  usual. The generated `.probe` files are picked up automatically.
- `read_test` works on either hand-written or generated files.

## Requirements

- `probe-mcp` binary v0.9.0+ installed and in `PATH`
- Flutter app running with `--dart-define=PROBE_AGENT=true` (for live tools like `get_widget_tree`, `take_screenshot`, `run_tests`)
- An MCP-compatible client (Claude Desktop, Cursor, etc.)

Verify the binary is accessible:

```bash
which probe-mcp   # should print /opt/homebrew/bin/probe-mcp or similar
```

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

Find the absolute path with `which probe-mcp`.

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

## Working directory

The MCP server inherits the working directory from the client. For tools like `run_tests`, `list_files`, and `get_report` to find your test files, the working directory must be your Flutter project root (the folder containing `probe.yaml` and `tests/`).

Claude Desktop launches the server with its own working directory, which may not be your project. Set it explicitly:

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

Expected response:

```json
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"probe-mcp","version":"0.9.3"}}}
```

List all available tools:

```bash
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | probe-mcp
```

## Example AI sessions

### Single-device test authoring

> "Look at the widget tree and write a test that verifies the login flow works."

1. `get_widget_tree` — inspect the running app's UI
2. `take_screenshot` — see what is on screen
3. `write_test` — create `tests/login.probe` (content validated before writing)
4. `run_tests` — execute it
5. `get_report` — verify all steps passed

### Device bring-up from chat

> "Boot an iOS simulator and run the smoke tests on it."

1. `list_simulators` — discover available UDIDs
2. `start_device` with `{platform: "ios", udid: "<chosen>"}`
3. `list_devices` — confirm it came online
4. `run_tests` with `{tag: "smoke", device: "<udid>"}`

### Composite (multi-device) test authoring

> "Write a test that verifies push notifications are delivered between two simulators."

1. `list_simulators` — discover two available iOS simulators
2. `start_device` for each if needed
3. `get_widget_tree` on both devices (using the `device` argument) to understand the UI
4. `write_test` — create a `composite test` block with device aliases and `sync` barriers
5. `run_tests` with `composite_devices: "A=<udid1> B=<udid2>"`
6. `get_report` — inspect per-device pass/fail results
7. `generate_report` — produce a shareable HTML report

### Record then refine

> "Record my interactions for 30 seconds and turn them into a reusable test."

1. `record` with `{timeout: "30s", output: "tests/recorded.probe"}` — returns the generated file
2. `lint` on the recorded file to check for issues
3. `write_test` to clean up and refine the generated steps
4. `run_tests` to verify the refined test passes

### HTML report from CI results

> "The CI run produced JSON results. Generate a report I can share."

1. `get_report` — reads the most recently modified JSON in `reports/`
2. `generate_report` — produces `reports/report.html`

## Migrating from `probe mcp-server`

Change:

```json
{ "command": "probe", "args": ["mcp-server"] }
```

to:

```json
{ "command": "probe-mcp" }
```

The tools, protocol, and behavior are identical. The legacy form continues to work but prints a deprecation notice.

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

### `write_test` returns a syntax error

The content you provided failed ProbeScript validation. The file was **not** created. Check the error message for the line and column of the problem. Common issues:

- Unterminated strings (`"` without closing `"`)
- Wrong indentation (use 2 spaces, not tabs)
- Unknown step keywords

Run `lint` on a corrected version to verify before calling `write_test` again.

### `record` returns "no output file"

Recording requires a WebSocket-connected device (simulators or emulators). It does not work over HTTP (physical-device WiFi mode). Ensure the device is a simulator or emulator and the app is running.
