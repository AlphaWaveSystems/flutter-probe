# Security Policy

FlutterProbe is a local E2E testing framework for Flutter mobile apps. While it is primarily a developer tool that runs on trusted machines against local or cloud-connected devices, we take security seriously and appreciate responsible disclosure of any vulnerabilities.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, use one of the following methods:

1. **GitHub Private Vulnerability Reporting** (preferred): Navigate to the [Security Advisories](https://github.com/AlphaWaveSystems/flutter-probe/security/advisories) page and click "Report a vulnerability." This allows for private, coordinated disclosure directly within GitHub.

2. **Email**: Send details to [support@alphawavesystems.com](mailto:support@alphawavesystems.com). If possible, encrypt your message using our PGP key (available on request).

When reporting, please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof of concept
- The version(s) of FlutterProbe affected
- Any suggested remediation, if applicable

We will acknowledge receipt within 48 hours and aim to provide a substantive response within 7 business days. We will work with you to understand the issue and coordinate a fix before any public disclosure.

We kindly ask that you:

- Allow reasonable time for us to address the issue before public disclosure
- Avoid accessing or modifying other users' data
- Act in good faith to avoid degradation of our services

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | Yes                |
| < 0.1.0 | No                 |

Security fixes are applied to the latest release in each supported minor version series. We recommend always running the latest patch release.

## GitHub Security Features

As a public repository, FlutterProbe leverages the following GitHub security features:

### Dependency Graph

Enabled by default for public repositories. The dependency graph identifies all transitive dependencies across the project's three ecosystems:

- **Go modules** (`go.mod`) -- CLI and probe-convert tool
- **Dart/Flutter** (`pubspec.yaml`) -- ProbeAgent on-device component
- **npm** (`package.json`) -- Documentation website (Starlight/Astro)

### Dependabot Alerts

Dependabot monitors the dependency graph and creates alerts when known vulnerabilities (from the GitHub Advisory Database) affect any dependency. Alerts include severity ratings, affected version ranges, and links to patches.

### Dependabot Security Updates

When a Dependabot alert has a known fix, Dependabot automatically opens a pull request to bump the vulnerable dependency to the minimum patched version. These PRs include compatibility scores and changelog details.

### Dependabot Version Updates

Configured via `.github/dependabot.yml`, Dependabot opens pull requests to keep dependencies up to date on a weekly schedule. This covers:

- Go modules (root and `tools/probe-convert/`)
- npm dependencies for the documentation website
- GitHub Actions versions in CI/CD workflows

### Code Scanning (CodeQL)

CodeQL static analysis is free for public repositories. A CodeQL workflow is configured to scan Go and JavaScript/TypeScript code on every push to `main` and on pull requests. CodeQL detects common vulnerability patterns including SQL injection, path traversal, command injection, and insecure data handling.

### Secret Scanning

Enabled by default for public repositories. GitHub scans all commits for known secret formats (API keys, tokens, credentials) from over 200 service providers.

- **Partner alerts**: GitHub notifies the issuing service provider so they can revoke compromised credentials
- **User alerts**: Repository administrators are notified of detected secrets
- **Push protection**: Blocks pushes that contain high-confidence secret patterns, preventing accidental credential exposure before it reaches the repository

### Security Advisories

GitHub Security Advisories are used for coordinated vulnerability disclosure. When a vulnerability is confirmed, we create a private advisory, develop a fix, request a CVE (if applicable), and publish the advisory alongside the patched release.

### Artifact Attestations

The release workflow produces signed build provenance attestations for all release artifacts. This allows users to verify that binaries were built from the expected source commit in the official CI environment.

### Software Bill of Materials (SBOM)

The dependency graph supports exporting SPDX-compatible SBOMs, providing a machine-readable inventory of all dependencies for supply chain auditing and compliance.

## Application Security

FlutterProbe includes several security measures in its architecture:

### Input Validation

- **Bundle ID validation**: The `project.app` field (used in ADB/simctl commands) is validated against the regex `^[a-zA-Z][a-zA-Z0-9_.]*$` at config load time. Invalid values are rejected before any shell interaction.
- **Device serial validation**: The `--device` flag value is validated against `^[a-zA-Z0-9._:/-]+$` via `config.ValidateDeviceSerial()` before being passed to any shell command, preventing command injection.
- **Selector sanitization**: Selector text captured during recording is sanitized (newlines and control characters stripped) before being written to `.probe` files, preventing ProbeScript syntax injection.
- **iOS path validation**: iOS data clearing operations use `validateIOSDataPath()` to prevent accidental deletion of paths outside the simulator container.

### Token Handling

- **One-time tokens**: The ProbeAgent authentication token is a 32-character cryptographically random string, generated fresh for each session.
- **Loopback-only**: The WebSocket server (and token exchange) operates exclusively on the loopback interface (`127.0.0.1`). Tokens are never transmitted over a network.
- **Token masking**: Dial errors and log messages display `ws://host:port/probe?token=***` instead of the actual token value, preventing accidental token leakage in logs or error reports.
- **Relay mode security**: When using cloud relay mode, connections are automatically upgraded from `ws://` to `wss://` for non-localhost hosts.

### Studio Security

The `probe studio` interactive UI server implements:

- **Localhost binding**: The HTTP server binds exclusively to `127.0.0.1`, preventing access from other machines on the network.
- **CORS protection**: API requests from origins other than the Studio's own address (`http://127.0.0.1:<port>`) are rejected with HTTP 403.
- **XSS prevention**: All user-controlled content (widget types, keys, error messages) is HTML-escaped via `escHtml()` before rendering in the DOM.

### Thread Safety

- The `VideoRecorder` uses a `sync.Mutex` to protect shared fields (`cmd`, `segments`, `frameIdx`, `remotePath`) that are accessed concurrently by background goroutines during screenrecord chaining and screencap capture.

### Configurable Tool Paths

The `--adb` and `--flutter` CLI flags (and `tools:` section in `probe.yaml`) allow overriding binary paths. These paths are resolved and validated before use, supporting non-standard installations and locked-down CI environments.

### Protocol Constants

All 22 JSON-RPC method names are defined as named constants in both Go (`internal/probelink/protocol.go`) and Dart (`ProbeMethods` class). No string literals are used in dispatchers, reducing the risk of typo-driven bugs in the protocol layer.

## Dependency Management

FlutterProbe maintains minimal dependencies across three ecosystems:

- **Go modules**: The CLI uses well-established libraries (`gorilla/websocket`, `spf13/cobra`, `gopkg.in/yaml.v3`). Dependencies are pinned via `go.sum` checksums.
- **Dart pub**: The ProbeAgent has zero external dependencies beyond the Flutter SDK itself. It uses only `flutter/widgets.dart` and `dart:io` from the standard library.
- **npm**: The documentation website uses Starlight (Astro) with a standard set of Astro plugins. Dependencies are locked via `package-lock.json`.

Dependabot version updates and security updates are configured to keep all dependencies current. See `.github/dependabot.yml` for the update schedule.

## Setup Checklist

The following items should be completed when the repository becomes public:

- [ ] Enable Dependabot alerts in Settings > Code security and analysis
- [ ] Enable Dependabot security updates in Settings > Code security and analysis
- [ ] Verify `.github/dependabot.yml` is active and creating PRs
- [ ] Enable CodeQL code scanning (add CodeQL workflow or enable default setup)
- [ ] Verify secret scanning is active with push protection enabled
- [ ] Enable private vulnerability reporting in Settings > Security > Advisories
- [ ] Create an initial security advisory template for coordinated disclosure
- [ ] Review and confirm SBOM export is available in the dependency graph
- [ ] Verify artifact attestations are generated by the release workflow
