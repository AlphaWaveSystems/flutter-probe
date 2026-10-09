# internal

Go packages behind the `probe` CLI and `probe-mcp`.

| Package | Role |
|---|---|
| `parser` | Lexer and parser: `.probe` source to an AST |
| `runner` | Runs the AST: executor, reconnect logic, reporters, parallel/composite orchestration |
| `probelink` | JSON-RPC 2.0 client for the on-device agent (WebSocket and HTTP) |
| `device`, `ios` | adb / simctl / devicectl management, permissions |
| `textfold` | Dependency-free text folding for loose, language-tolerant matching (case, accents, apostrophes, whitespace); the Dart agent has a tested mirror |
| `sysdialog` | System dialogs on iOS (XCUITest driver) and Android (uiautomator); buttons are resolved by role so labels work in any device language |
| `config` | `probe.yaml` loading and defaults |
| `report` | HTML report |
| `visual` | Screenshot regression comparison |
| `redact` | Region redaction applied before any screenshot goes to an AI provider |
| `ai` | **Optional** AI helpers: vision/text assertions, failure triage, generation, `probe ai doctor`. Tests never depend on it: only a step the author wrote with `with ai` can use it to decide a result |
| `cloud` | Device-farm providers and FlutterProbe Cloud upload |
| `migrate` | Maestro YAML to ProbeScript |
| `plugin` | Custom ProbeScript commands |
| `mcp` | MCP server tool definitions |
| `cli` | Cobra commands |
