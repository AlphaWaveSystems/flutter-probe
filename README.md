# FlutterProbe

A high-performance local E2E testing framework for Flutter mobile apps. Write tests in natural language, execute with sub-50ms command round-trips via direct widget-tree access.

[![License: BSL 1.1](https://img.shields.io/badge/License-BSL%201.1-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.1.0-green.svg)](CHANGELOG.md)

```
test "user can log in"
  open the app
  see "Email"
  type "user@example.com" into "Email"
  type "secret" into "Password"
  tap "Sign In"
  see "Dashboard"
```

## Quick Start

```bash
# 1. Build the CLI
git clone https://github.com/AlphaWaveSystems/flutter-probe.git
cd FlutterProbe
make build        # → bin/probe

# 2. Add ProbeAgent to your Flutter app
# In pubspec.yaml:
#   dev_dependencies:
#     probe_agent:
#       path: /path/to/FlutterProbe/probe_agent

# 3. Initialize and run
cd your-flutter-app
probe init
probe test tests/
```

## How It Works

```
┌──────────────┐     WebSocket / JSON-RPC 2.0      ┌─────────────────┐
│  probe CLI   │ ──────────────────────────────────▶│  ProbeAgent     │
│  (Go)        │   localhost:48686                   │  (Dart, on      │
│              │   tap, type, see, wait, swipe       │   device)       │
│  Parses .probe│  screenshot, dump_tree             │                 │
│  Manages devs │  One-time token auth               │  Walks widget   │
│  Reports     │                                     │  tree directly  │
└──────────────┘                                    └─────────────────┘
```

- **Android**: CLI → `adb forward` → WebSocket → ProbeAgent
- **iOS Simulator**: CLI → `localhost` directly (shared loopback)

## Features

- **Natural language tests** — ProbeScript with indent-based blocks
- **Sub-50ms execution** — Direct widget-tree access, no UI automation overhead
- **Cross-platform** — iOS simulators + Android emulators
- **Recipes** — Parameterized reusable step sequences
- **Data-driven** — `Examples:` blocks for table-driven tests
- **Visual regression** — Screenshot comparison with configurable threshold
- **Recording** — `probe record` captures interactions as ProbeScript
- **7-format converter** — Migrate from Maestro, Gherkin, Robot, Detox, Appium
- **CI/CD ready** — JUnit XML, JSON, HTML reports + GitHub Actions examples
- **VS Code extension** — Syntax highlighting, snippets, commands
- **Self-healing** — AI-powered selector recovery via fuzzy matching
- **Custom plugins** — YAML-defined commands dispatched to Dart handlers

## CLI Commands

| Command | Description |
|---------|-------------|
| `probe init` | Scaffold project |
| `probe test [path]` | Run tests |
| `probe lint [path]` | Validate syntax |
| `probe device list` | List devices |
| `probe record` | Record interactions |
| `probe report` | Generate HTML report |
| `probe version` | Print version |
| `probe-convert` | Convert from other frameworks |

## Documentation

Full documentation: [flutterprobe.github.io](https://flutterprobe.github.io)

- [Installation](https://flutterprobe.github.io/docs/getting-started/installation/)
- [Quick Start](https://flutterprobe.github.io/docs/getting-started/quick-start/)
- [ProbeScript Syntax](https://flutterprobe.github.io/docs/probescript/syntax/)
- [CLI Reference](https://flutterprobe.github.io/docs/tools/cli-reference/)
- [CI/CD Integration](https://flutterprobe.github.io/docs/ci-cd/github-actions/)
- [Configuration](https://flutterprobe.github.io/docs/advanced/configuration/)

## Requirements

- Go 1.23+
- Dart 3.3+ / Flutter 3.19+
- Android: ADB + Android SDK
- iOS: Xcode + `xcrun simctl`

## License

[Business Source License 1.1](LICENSE) — free for all use except competing commercial hosted testing services. Converts to Apache 2.0 after 4 years per release.
