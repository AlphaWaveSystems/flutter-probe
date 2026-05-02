# FlutterProbe Wiki

Welcome to the FlutterProbe wiki. This documentation covers architecture details, development guides, and operational procedures that complement the main docs at [flutterprobe.dev](https://flutterprobe.dev).

## Quick Links

- [Architecture Overview](Architecture-Overview) — System components, communication flow, and design decisions
- [Development Setup](Development-Setup) — How to build, test, and contribute
- [ProbeScript Reference](ProbeScript-Reference) — Language syntax, commands, and patterns
- [iOS Integration Guide](iOS-Integration-Guide) — iOS-specific setup, known limitations, and workarounds
- [Android Integration Guide](Android-Integration-Guide) — Android-specific setup and ADB usage
- [CI/CD Integration](CI-CD-Integration) — Running FlutterProbe in CI pipelines
- [Cloud Providers](Cloud-Providers) — BrowserStack, SauceLabs, AWS Device Farm, Firebase Test Lab, LambdaTest
- [Troubleshooting](Troubleshooting) — Common issues and solutions
- [Release Process](Release-Process) — How to cut a release
- [Security](Security) — Security practices and vulnerability reporting

## Project Status

FlutterProbe is in active development. Current version: **0.7.0**.

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
