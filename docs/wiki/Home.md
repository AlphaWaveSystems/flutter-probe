# FlutterProbe Wiki

Welcome to the FlutterProbe wiki. This documentation covers architecture details, development guides, and operational procedures that complement the main docs at [flutterprobe.dev](https://flutterprobe.dev).

## Quick Links

- [Architecture Overview](Architecture-Overview) — System components, communication flow, and design decisions
- [Development Setup](Development-Setup) — How to build, test, and contribute
- [ProbeScript syntax](https://flutterprobe.dev/probescript/syntax/) — Language syntax, commands, and patterns
- [iOS Integration Guide](iOS-Integration-Guide) — iOS-specific setup, known limitations, and workarounds
- [Android platform guide](https://flutterprobe.dev/platform/android/) — Android-specific setup and ADB usage
- [CI/CD with GitHub Actions](https://flutterprobe.dev/ci-cd/github-actions/) — Running FlutterProbe in CI pipelines
- [Device clouds](https://flutterprobe.dev/tools/cloud-farms/) — the providers the CLI implements (BrowserStack, SauceLabs, AWS Device Farm, Firebase Test Lab, LambdaTest and others) and which of them have been verified on a live account
- [Composite Tests](Composite-Tests) — Multi-device coordinated testing
- [Troubleshooting](Troubleshooting) — Common issues and solutions
- [Release pipeline](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/docs/ops/release-pipeline.md) — How a release is cut
- [Security policy](https://github.com/AlphaWaveSystems/flutter-probe/security/policy) — Security practices and vulnerability reporting

## Studio

**FlutterProbe Studio** (beta) is the desktop app for authoring and running ProbeScript tests with a live device view, a widget-tree inspector, lint (a Problems tab), a Network tab and optional AI help (advice and one-line fixes that are never applied or run by themselves). See the [Studio page](https://flutterprobe.dev/tools/studio/) for what it does, its limits and how to build it, and `studio/README.md` in the repository for development.

## Project Status

FlutterProbe is in active development. Compatibility policy for 1.x: https://flutterprobe.dev/advanced/compatibility/. Performance testing (`start measuring`, `see memory below`, baselines, `probe perf trend`). Per-step time budgets: `... within 90 seconds`. React to backend data with `wait for response` / `see response` / `if response` and real mocks. Use ARB keys with `l10n "key"`. Change the app language with `set language "de"` or `probe test --locales de,ja,ar`. Devices are always named: `probe device start --name`, and every result records its device. Multi-language: see [Testing localized apps](https://flutterprobe.dev/advanced/multi-language/) (loose text matching, alternatives, system dialog roles).

Current version: **0.27.0**.

### Repository Structure

| Directory | Description |
|---|---|
| `cmd/probe/` | CLI entry point |
| `cmd/probe-mcp/` | Standalone MCP server binary |
| `internal/` | Go packages (parser, runner, probelink, device, ios, cloud, ai, mcp, etc.) |
| `probe_agent/` | Dart package that runs on-device |
| `studio/` | FlutterProbe Studio — Wails desktop app (beta preview) |
| `tools/probe-convert/` | Multi-format test converter |
| `website/` | Documentation site (Starlight/Astro) |
| `tests/` | E2E test suites and health checks |
