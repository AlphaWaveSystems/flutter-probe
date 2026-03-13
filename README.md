# FlutterProbe

A high-performance local E2E testing system for Flutter mobile applications.

## Quick Start

```bash
# 1. Build the CLI
make build

# 2. Scaffold a new Flutter project
cd your-flutter-project
probe init

# 3. Add the ProbeAgent to your Flutter app (dev dependency)
# In pubspec.yaml:
#   dev_dependencies:
#     probe_agent:
#       path: /path/to/flutterprobe/probe_agent

# 4. Wrap your main() in debug mode
# lib/main_debug.dart:
#   import 'package:probe_agent/probe_agent.dart';
#   void main() async {
#     WidgetsFlutterBinding.ensureInitialized();
#     await ProbeAgent.start();
#     runApp(const MyApp());
#   }

# 5. Start your app on an emulator
flutter run --dart-define=PROBE_AGENT=true -d emulator-5554

# 6. Run your tests
probe test
```

## Example Test

```
test "a user can sign in"
  open the app
  wait until "Sign In" appears
  tap on "Sign In"
  type "user@example.com" into the "Email" field
  type "mypassword" into the "Password" field
  tap "Continue"
  see "Dashboard"
```

## Commands

| Command | Description |
|---------|-------------|
| `probe init` | Scaffold probe.yaml and tests/ directory |
| `probe test` | Run all .probe files |
| `probe test <file>` | Run a specific file |
| `probe test --tag smoke` | Run tests by tag |
| `probe test --watch` | Watch mode |
| `probe test --format junit -o results.xml` | JUnit output |
| `probe lint tests/` | Validate .probe files |
| `probe device list` | List connected devices |
| `probe device start --platform android` | Start an emulator |

## Architecture

```
probe CLI (Go)  ──WebSocket/JSON-RPC──▶  ProbeAgent (Dart)
     │                                         │
     │  parses .probe files                    │  queries widget tree
     │  manages devices via ADB                │  executes actions
     │  generates reports                      │  triple-signal sync
```

## Project Structure

```
flutterprobe/
├── cmd/probe/          # CLI entry point
├── internal/
│   ├── cli/            # cobra commands (init, test, lint, device, report)
│   ├── config/         # probe.yaml parsing
│   ├── parser/         # ProbeScript lexer + AST + parser
│   ├── probelink/      # JSON-RPC 2.0 WebSocket client
│   ├── device/         # ADB + emulator management
│   └── runner/         # test execution, executor, reporter
└── probe_agent/        # Dart package (on-device agent)
    └── lib/
        └── src/
            ├── agent.dart      # top-level API
            ├── server.dart     # WebSocket server
            ├── executor.dart   # command dispatcher
            ├── finder.dart     # widget selector engine
            ├── sync.dart       # triple-signal synchronization
            └── protocol.dart   # JSON-RPC types
```

## Performance Targets

| Metric | Target |
|--------|--------|
| Command round-trip | < 50ms |
| CLI cold start | < 100ms |
| 50-test suite | < 90 seconds |
| Flake rate | < 0.5% |
