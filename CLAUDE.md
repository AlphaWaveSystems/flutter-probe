# CLAUDE.md — FlutterProbe Development Guide

## Project Overview

FlutterProbe is a Go CLI + Dart agent E2E testing framework for Flutter apps. Tests are written in ProbeScript (`.probe` files) — natural language syntax. The CLI orchestrates device connections, parses tests, dispatches commands via WebSocket/HTTP to the on-device Dart agent.

## Architecture

```
Go CLI (cmd/probe)  <-- WebSocket/HTTP -->  Dart Agent (probe_agent)
   |                                              |
   ├── parser/       AST from .probe files       ├── server.dart    WS + HTTP server
   ├── runner/       Test orchestration           ├── executor.dart  Command dispatch
   ├── probelink/    JSON-RPC 2.0 client          ├── finder.dart    Widget tree search
   ├── device/       ADB + iproxy management      ├── sync.dart      Triple-signal sync
   ├── ios/          simctl + devicectl            └── recorder.dart  Test recording
   ├── config/       probe.yaml parsing
   └── report/       HTML/JUnit/JSON output
```

## Build & Test

```bash
go build ./...           # Build CLI
go test ./...            # Run all Go tests (115+)
go build -o /tmp/probe-test ./cmd/probe  # Build binary for testing
```

## Key Development Rules

### Git
- **Email**: patrick@alphawavesystems.com for all commits
- **Branches**: Always feature branches + PRs, never push to main
- **Docs**: Every PR must update CHANGELOG.md, wiki, and landing page version

### Physical Device Testing
- **Profile mode + flavor**: Debug builds can't cold-launch from home screen. Use `flutter build ios --profile --flavor <flavor> --dart-define=PROBE_AGENT=true`
- **WiFi > USB**: USB-C causes intermittent drops. Use `--host <ip> --token <token> --dart-define=PROBE_WIFI=true`
- **No `clear app data`**: Unsupported on physical iOS. Tests must be self-contained with explicit login/logout
- **`tap "X" if visible`**: Use instead of verbose `dismiss dialogs` recipes
- **Semantics + GestureDetector**: Put `ValueKey` on the `GestureDetector`, not the `Semantics` wrapper

### Flutter Widget Tree Gotchas
- **Navigator keeps back routes in tree**: `if "X" appears` matches widgets on background routes. Don't use it to detect which screen is active.
- **Visibility filtering**: `Offstage`/`Visibility` checks don't catch Navigator route hiding (uses opacity, not Offstage)

### Test Patterns for Physical Devices
- Each test starts from known state (login screen) and cleans up explicitly
- Use `do logout` + `wait until "Usuario" appears` at end of tests that log in
- No `restart the app` in middle of WiFi test suites (new token needed after restart)
- `tap "Aceptar" if visible` for dialog dismissal (2 lines vs 20-line recipe)

## Transport Modes

| Mode | When | How |
|---|---|---|
| WebSocket | Simulators, emulators | Default — persistent connection, ping/pong keepalive |
| HTTP POST | Physical devices (USB) | Auto-selected — stateless, no connection to drop |
| HTTP POST | Physical devices (WiFi) | `--host <ip> --token <token>` — zero drops |

## Release Checklist

1. `CHANGELOG.md` — new version section
2. `vscode/package.json` — bump version
3. `website/src/pages/index.astro` — update version badge
4. `docs/wiki/Home.md` — update current version
5. `git tag v0.X.Y && git push origin v0.X.Y` — triggers release workflow

## File Locations

| What | Where |
|---|---|
| CLI entry | `cmd/probe/main.go` |
| Test command | `internal/cli/test.go` |
| Parser | `internal/parser/{lexer,parser,ast,token}.go` |
| Runner | `internal/runner/{executor,runner,device_context}.go` |
| WS Client | `internal/probelink/client.go` |
| HTTP Client | `internal/probelink/http_client.go` |
| Interface | `internal/probelink/iface.go` |
| Device mgmt | `internal/device/{manager,adb,permissions}.go` |
| iOS tools | `internal/ios/{simctl,devicectl}.go` |
| Dart agent | `probe_agent/lib/src/{server,executor,finder}.dart` |
| Config | `internal/config/config.go` |
| Landing page | `website/src/pages/index.astro` |
| Wiki docs | `docs/wiki/*.md` |
