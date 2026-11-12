---
title: MCP Server
description: Connect FlutterProbe to Claude Desktop, Cursor, and other AI agents via the Model Context Protocol.
---

FlutterProbe ships an MCP (Model Context Protocol) server as a standalone binary called `probe-mcp` that exposes your entire test suite as tools callable by AI agents. Once connected, Claude Desktop, Cursor, or any MCP-compatible client can manage devices, write and run tests, record interactions, generate reports, and inspect results — all without leaving the AI chat.

> **As of v0.6.0**, the MCP server is a separate binary `probe-mcp`. The legacy `probe mcp-server` subcommand still works but prints a deprecation notice on stderr. Update your MCP client configuration to point at `probe-mcp` directly.

## What the MCP server exposes

### Device lifecycle (6 tools)

| Tool | Description |
|---|---|
| `list_devices` | List booted/connected simulators, emulators, and physical devices (id, name, platform, state, OS version) |
| `list_simulators` | List all iOS simulators (booted + shutdown) so the agent can pick one to boot |
| `list_avds` | List Android Virtual Device names available to launch |
| `start_device` | Boot an Android emulator (by AVD name) or iOS simulator (by UDID or `name`, created when missing); blocks until online. Devices are always named so results are tied to a device |
| `shutdown_device` | Shut down an iOS simulator (`udid`) or Android emulator (`serial`) |
| `set_device_setting` | Change an OS setting of one named device: `appearance` (dark mode), `font_scale`, `locale` (needs `app`), `orientation` (Android) or `open_url` (a deep link). `device` is required and never `booted`; values are checked before anything runs |

### Authoring & execution (9 tools)

| Tool | Description |
|---|---|
| `get_widget_tree` | Dump the live Flutter widget tree from the running app |
| `take_screenshot` | Capture the current screen and return it as an image |
| `read_test` | Read the contents of a `.probe` file |
| `write_test` | Create or overwrite a `.probe` file — content is validated before writing; syntax errors are returned without creating the file |
| `run_script` | Execute inline ProbeScript without creating a file |
| `migrate_maestro` | Convert Maestro YAML flows to ProbeScript (recursive, mirrors subdirectories, 2.x syntax) |
| `run_tests` | Run `.probe` test files (supports composite multi-device tests via `composite_devices`) |
| `list_files` | List all `.probe` files in a directory |
| `lint` | Validate `.probe` file syntax without running |
| `record` | Record user interactions and generate a `.probe` test file (runs for `timeout` duration, default 30s) |

`get_widget_tree`, `take_screenshot`, `run_script`, and `run_tests` accept an optional `device` argument (serial or UDID) to pin a specific target.

### Reporting & generation (5 tools)

| Tool | Description |
|---|---|
| `get_report` | Read the most recently modified JSON test run report |
| `studio_open_workspace`, `studio_list_devices`, `studio_connect` (a device id, or `host`/`port`/`token` of a phone on the network), `studio_run_file`, `studio_cancel`, `studio_run_state`, `studio_results`, `studio_screenshot`, `studio_widget_tree`, `studio_ui_state` | Drive a running Probe Studio (started with `PROBE_STUDIO_AUTOMATION=1`) without a window: open a workspace, connect to a device, run a `.probe` file inside the open workspace (other paths are refused), or part of one with `line` (the test that contains a line), `tag` or `failed_only`, and read step-by-step progress; a failed `see` or `tap` step in `studio_run_state` carries a `triage` with a suggested fix and a reason (rule-based, no AI) |
| `perf_trend` | Show how a measured performance number (memory, CPU, frames, data) changed over past runs |
| `perf_compare` | Compare two performance baseline files and list what got worse |
| `generate_report` | Generate a standalone HTML report from a JSON results file |
| `generate_test` | AI-generate a `.probe` test from a natural language prompt |
| `system_dialog` | List, see, tap, dismiss or wait for OS system dialogs (permission alerts, the iOS share sheet, StoreKit sign-in) on a simulator/emulator; `sign-in-sandbox` provisions the StoreKit tester. Typing is deliberately not exposed (secrets) |
| `triage_failure` | Optional, advisory: explain failures in a JSON report with the configured `ai:` model (a local model works). Never affects results |

### Project management (1 tool)

| Tool | Description |
|---|---|
| `init_project` | Initialize a new FlutterProbe project (creates `probe.yaml` and `tests/` scaffold) |
| `validate_project` | Check `probe.yaml` and the `probe_version` pin without a device. Returns a JSON report (`probe.validate/1`): each check is ok, warn or fail, with a fix for any problem |
| `doctor` | Check that the project and this machine are ready for `probe test`. Returns a JSON report (`probe.doctor/1`): each check is ok, warn or fail, with a fix for any problem. Changes nothing |

The full workflow this enables: `list_devices` → `start_device` (if needed) → `get_widget_tree` → `write_test` → `run_tests` → `get_report` → `generate_report` — a complete AI-driven test authoring loop, including device bring-up and HTML reporting.

## `run_tests` flags

The `run_tests` tool has named parameters for common options (`paths`, `tag`, `device`, `composite_devices`) and a `flags` string for everything else. Key flags an agent should know:

| Flag | What it does |
|---|---|
| `--timeout 60s` | Per-step timeout (default 30s) |
| `--launch-timeout 3m` | Timeout for `restart the app`/`clear app data` specifically (default 120s) — raise this if those steps time out on an app with an expensive cold-launch path (e.g. Firebase App Check init) |
| `-v` | Verbose step output — prints `→ step` before each step and `✓/✗ step (Xs)` after; slow steps (>5s) emit `⏱` progress ticks and a `⚠` warning at 80% of timeout. Recommended when diagnosing failures. |
| `--format json` | Structured JSON results (pipe to `get_report`) |
| `--format junit` | JUnit XML for CI systems |
| `--dry-run` | Validate syntax without a device connection |
| `--parallel` | Distribute tests across all connected devices |
| `--shard 1/3` | Run 1/3 of test files (for CI matrix builds) |
| `--host <ip> --token <t>` | WiFi mode for physical devices |
| `--agent-port 48700` | Agent port (alias of `--port`); pair with the app's `--dart-define=PROBE_PORT=48700` so simulators don't collide. A failed dial names the process holding the port. |
| `--grant notifications,camera` | Pre-grant OS permissions before the first test (Android `pm grant`, iOS simulator `simctl privacy`; iOS notifications: a watcher taps Allow when the alert appears) |
| `--disable-animations` | Set a tiny `timeDilation` (0.001) so animations end within a frame |
| `-y` | Auto-approve destructive operations (CI mode) |
| `--video` | Record device screen during the run |
| `--record` | Write a step-by-step recording of each test attempt under `reports_folder/recordings/`: steps, screenshots, widget trees, the app's Dart HTTP calls (credentials redacted) and an offline viewer. Local only; never uploaded. `--record-dir DIR` changes the folder |
| `--stream` | Emit one ndjson line per test as it completes (requires `--format json`) |
| `--ai-heal` | Optional, advisory: with an `ai:` block in `probe.yaml`, add a `selector_proposal` to each selector step that ran into its timeout in the JSON report. Not applied and not verified; the step stays failed and results do not change |

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

- Flutter app running with `--dart-define=PROBE_AGENT=true` (for live tools like `get_widget_tree`, `take_screenshot`, `run_tests`)
- An MCP-compatible client (Claude Desktop, Cursor, etc.)
- For Option 2 below: `probe-mcp` binary v0.9.0+ installed and in `PATH`

## Install — Option 1: one-click Claude Desktop Extension (recommended)

As of v0.9.4, every release includes a `.mcpb` Claude Desktop Extension that bundles the `probe-mcp` binary. Installation is one click — no `brew`, no JSON config, no `PATH` setup.

1. Open the [latest GitHub release](https://github.com/AlphaWaveSystems/flutter-probe/releases/latest) and download the bundle for your platform:
   - `flutter-probe-darwin-arm64.mcpb` — Apple Silicon Macs
   - `flutter-probe-darwin-amd64.mcpb` — Intel Macs
   - `flutter-probe-linux-amd64.mcpb` — Linux x86_64
   - `flutter-probe-win32-amd64.mcpb` — Windows x86_64
2. In Claude Desktop, open **Settings → Extensions** and click **Install Extension**.
3. Pick the downloaded `.mcpb` file. When prompted, select your **Flutter project directory** (the folder containing `probe.yaml` and `tests/`).
4. Done — all 40 tools are immediately available in any new Claude conversation.

Auto-updates and lifecycle are handled by Claude Desktop. To update, just install a newer `.mcpb` over the older one.

## Install — Option 2: manual MCP server config

Use this path for Cursor, Claude Code, or any MCP-compatible client other than Claude Desktop, or if you prefer to keep the binary on `$PATH`.

Verify the binary is accessible:

```bash
which probe-mcp   # should print /opt/homebrew/bin/probe-mcp or similar
```

### Claude Desktop (manual config)

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

### Cursor

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

## HTTP transport

By default `probe-mcp` talks over stdio: the client starts it and owns it. For a client that connects to a running server instead, such as an agent inside VS Code, `probe-mcp` can also serve the same tools over HTTP on your own machine. It is off unless you start it this way:

```bash
cd /path/to/my-flutter-app
probe-mcp --http 127.0.0.1:7391
```

```
probe-mcp: MCP over HTTP at http://127.0.0.1:7391/mcp
probe-mcp: bearer token written to /Users/you/Library/Application Support/flutter-probe-mcp/http-token (a new one on every start)
```

Pick any free port; `127.0.0.1:0` lets the system choose one and prints it. The server works in the directory you start it in (or in `PROBE_PROJECT_DIR`), so start it in your Flutter project. Stop it with Ctrl+C.

The tools and their results are the same as over stdio. The endpoint follows the MCP "Streamable HTTP" transport in its simplest form: one `POST` per JSON-RPC message, one JSON answer. It does not open event streams, so `GET` answers `405`. A long run is best started with `start_tests` and followed with `test_status`.

### The token

Every request must carry `Authorization: Bearer <token>`. A request without the right token gets `401` and an empty body.

- **Default: a new token on every start.** It is 256 random bits, written to `http-token` in `flutter-probe-mcp` under your user config directory (`~/Library/Application Support` on macOS, `~/.config` on Linux, `%AppData%` on Windows). The file is readable only by you (mode `0600` in a `0700` directory) and is removed when the server stops. The path is printed on start; the token itself is never printed or logged. A token that leaked stops working as soon as the server stops.
- **A token that stays the same:** set `PROBE_MCP_HTTP_TOKEN` to your own value, for example from `openssl rand -base64 32`. It must be at least 43 characters long and use at least 16 different characters; a shorter or more repetitive value is refused. Then no file is written. Keep it out of files you share. `probe-mcp` removes the variable from its own environment at start, so the commands it runs do not inherit it, but the value stays visible in the process's initial environment to programs running as the same user. On a shared machine, do not reuse a stable token after the server has stopped: another program could listen on the same port and receive it.

`probe-mcp` refuses to start if the token directory or an existing token file is a symlink or can be read by other users. Remove the file, or run `chmod 700` on the directory, and start again. On Windows these file-mode checks are skipped: the file is protected by the access rules of your user profile folder, not by a mode.

Run one HTTP server per user config directory. A second one started the same way replaces the first one's token file (it says so on start), and the first server's token is then no longer on disk. For a second server, set `PROBE_MCP_HTTP_TOKEN`.

```bash
TOKEN=$(cat "$HOME/Library/Application Support/flutter-probe-mcp/http-token")
curl -s http://127.0.0.1:7391/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

### VS Code

Add the server to `.vscode/mcp.json` in your workspace. The `inputs` entry makes VS Code ask for the token and keep it out of the file:

```json
{
  "inputs": [
    {
      "type": "promptString",
      "id": "probe-mcp-token",
      "description": "probe-mcp HTTP token",
      "password": true
    }
  ],
  "servers": {
    "flutter-probe": {
      "type": "http",
      "url": "http://127.0.0.1:7391/mcp",
      "headers": {
        "Authorization": "Bearer ${input:probe-mcp-token}"
      }
    }
  }
}
```

VS Code stores the value you enter, so with the default token you enter it again after each restart of `probe-mcp`. To avoid that, start the server with your own `PROBE_MCP_HTTP_TOKEN`. Keep the `127.0.0.1` literal in the URL, not `localhost`: a name can be made to resolve to another address, the literal cannot. VS Code has more than one MCP configuration format; see its [MCP configuration reference](https://code.visualstudio.com/docs/copilot/reference/mcp-configuration) for the one your version uses. The `type`, `url` and `headers` fields are the same in each.

### Security model

What the server does, in plain words:

- **It listens only on your own machine.** The address must be a loopback one: `127.0.0.1`, `localhost` (which means `127.0.0.1` here), or `[::1]` for IPv6. It listens on exactly the address you give, not on both.
- **Every request needs the token.** The comparison takes the same time whatever the wrong token looks like, and a missing token and a wrong one get the same answer. A request without the token is answered before its body is read, and the connection is closed.
- **Web pages cannot use it.** A browser adds an `Origin` header, a `Sec-Fetch-Site` header, or both to requests a page makes. A request with an `Origin` header, or with a `Sec-Fetch-Site` value other than `none`, is refused (`403`), and the server sends no CORS headers. `none` means no page made the request (an address typed by hand, some command-line clients); such a request still needs the token. The `Host` header must name the address the server listens on, which stops a web page that points its own domain name at `127.0.0.1` (DNS rebinding).
- **Only the expected kind of request is read.** `POST` to `/mcp` with `Content-Type: application/json`, up to 4 MiB; at most 8 requests at a time; time limits on reading and writing.

What it does not do:

- **No TLS.** Traffic on the loopback interface does not leave your machine, so the server does not encrypt it.
- **No protection from other programs running as you.** Anything that runs under your user account can read the token file, as it can read your other files.
- **No per-tool permissions.** A client with the token can call every tool, including the ones that write test files and run commands on your devices. This is the same as over stdio.

### Listening on another address

`probe-mcp` refuses any address that is not loopback. The switch `--dangerously-allow-non-loopback` turns that refusal off:

```bash
probe-mcp --http 192.168.1.20:7391 --dangerously-allow-non-loopback
```

**Do not use this on a network you do not fully control.** The token and everything the tools return then cross the network unencrypted, and anyone who captures the token can run tests, write files in your project and drive your devices. If you need remote access, keep the server on loopback and reach it through an SSH tunnel or a proxy that adds TLS.

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
{"jsonrpc":"2.0","id":1,"result":{"capabilities":{"tools":{}},"protocolVersion":"2024-11-05","serverInfo":{"name":"probe-mcp","version":"0.9.8"}}}
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

### AI-powered visual assertion

> "Add a check that the checkout screen's total looks visually correct — it's hard to pin down with a plain selector."

1. Confirm `probe.yaml` has an `ai:` block configured — `provider: openai`/`anthropic` with `api_key: ${...}`, or `provider: local` with `endpoint`/`model` pointing at a local server like Ollama if nothing should leave the device/host at all — `write_test` will reject `with ai` steps at parse time if it's missing, so check this first rather than writing a test that can never pass
2. `get_widget_tree` — understand the surrounding flow so the assertion is placed at the right step
3. `write_test` — add `see "checkout total, tax, and shipping are all visible and add up correctly" with ai` to the relevant test
4. `run_tests` — execute it; a failing AI assertion reports the model's reasoning, not just pass/fail
5. `get_report` — the (possibly redacted, per `ai.redact` in probe.yaml) screenshot sent to the provider is attached as an artifact for that step

For a generic "does this screen look right" smoke check instead of a specific claim, use `assert no visual defects with ai` — same `ai:` config requirement, but no assertion text to write; it flags cut-off, overlapping, or mis-centered elements on its own. Unlike `see ... with ai`, there's no non-AI form of this command.

To pull a specific piece of text off the screen into a variable — an OTP code, a dynamically-generated ID — for use in a later step, use `read "<query>" with ai into <var>`, e.g. `read "the 6-digit OTP code" with ai into otp` followed by `type "<otp>" into the "Code" field`. Same `ai:` requirement and mandatory `with ai` as `assert no visual defects`; fails with a clear error rather than storing an empty value if the requested text isn't visible.

### GPS route simulation

> "Write a test that simulates the delivery courier walking from the restaurant to the drop-off point and checks the ETA updates."

1. `get_widget_tree` — find the ETA/map widgets to assert against
2. `write_test` — add a `travel to` block with the restaurant and drop-off waypoints and an `over N seconds` duration, e.g.:
   ```
   travel to
     37.7749, -122.4194
     37.7849, -122.4094
   over 15 seconds
   see "Arriving in"
   ```
3. `run_tests` — execute it; the device's simulated GPS location moves through the waypoints in order as the test runs
4. `get_report` — confirm the ETA assertion passed

`travel to` is emulator/simulator only (same limitation as the single-point `set location`) — it skips with a warning on physical devices.

### Reaching rows below the fold

> "Write a test that opens Settings and checks the 'Rate' row, which is far down the list."

1. `get_widget_tree` — confirm the row isn't built yet (lists build rows lazily)
2. `write_test` — use `scroll down until` instead of counting `scroll down` steps, and `wait for idle` after dismissing a dialog instead of a fixed sleep:
   ```
   tap #nav_tab_settings
   scroll down until "Rate this app" appears
   see "Rate this app"
   tap #close_button
   wait for idle
   tap #nav_tab_today
   ```
3. `run_tests` with `flags: "--grant notifications"` — pre-grants the OS permission so no system prompt interrupts the flow (Android `pm grant`, iOS simulator `simctl privacy`; iOS notifications are answered by tapping Allow on the system alert)
4. If a step fails, the error carries `line N`, the step, and the visible texts/keys at that moment — read those instead of guessing

`scroll down` reveals later content; `scroll up` goes back toward the top.

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
