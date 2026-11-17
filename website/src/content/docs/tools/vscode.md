---
title: VS Code Extension
description: ProbeScript syntax highlighting, snippets, and integrated commands for VS Code.
---

FlutterProbe includes a VS Code extension that provides a first-class editing experience for ProbeScript files.

## Features

### Syntax Highlighting

Full syntax highlighting for `.probe` files including:
- Keywords (`test`, `recipe`, `use`, `before each`, `after each`, `on failure`)
- Selectors (text strings, `#key` references, `<Type>` references)
- Tags (`@smoke`, `@critical`)
- Comments
- Block structures (indentation-aware)

### Code Snippets

Quick-insert common patterns:

| Prefix | Expands to |
|--------|-----------|
| `test` | Full test block |
| `recipe` | Recipe with parameters |
| `before` | `before each` hook |
| `after` | `after each` hook |
| `onfail` | `on failure` hook |
| `if` | Conditional block |
| `repeat` | Loop block |
| `examples` | Data-driven examples block |

### Testing view

Tests in `.probe` files appear in VS Code's Testing view, with a run button next to each test and in the editor gutter. A run executes the file with `probe test --yes --format json` and reports each selected test's pass or fail from the CLI's report. The CLI has no per-test filter in this version, so a run executes the whole file. The Tests sidebar is unchanged.

### Commands

Access via the Command Palette (`Cmd+Shift+P`):

| Command | Description |
|---------|-------------|
| FlutterProbe: Run Test File at Cursor | Run the file that contains the cursor (the CLI has no per-test filter yet) |
| FlutterProbe: Run File | Run all tests in current file |
| FlutterProbe: Lint File | Validate current file syntax |
| FlutterProbe: Start Recording | Start recording interactions |
| FlutterProbe: Open Studio | Launch interactive test studio |

## Installation

### From VSIX (local build)

The extension is pre-built in the repository:

```bash
code --install-extension vscode/flutterprobe-0.1.0.vsix
```

### From source

```bash
cd vscode
npm install
npm run compile
```

Then press `F5` in VS Code to launch an Extension Development Host with the extension loaded.

## Configuration

The extension looks for the `probe` binary on your PATH. If it is installed elsewhere, configure it in your **user** settings (a workspace's `.vscode/settings.json` cannot set it):

```json
{
  "flutterprobe.probePath": "/path/to/bin/probe"
}
```

## Security

- **Secrets never go on a command line.** The cloud key, the cloud secret and the relay token reach `probe test` as environment variables of its terminal (`PROBE_CLOUD_KEY`, `PROBE_CLOUD_SECRET`, `PROBE_RELAY_CLI_TOKEN`). These settings, and the relay URL and cloud provider that decide where they are sent, can be set only in your user (or remote) settings.
- **A workspace cannot choose the binaries.** `probePath`, `convertPath`, `adbPath` and `flutterPath` are user (or remote) settings, the extension looks the binary up in the absolute folders of PATH itself and runs the path it found, never one from the workspace folder (when `probe` is on no absolute folder of PATH it says so, on every system), and a `bin/probe` in the workspace is used only with `flutterprobe.useWorkspaceBinaries`. The extension runs only in a trusted workspace.
- **What the programs the extension starts inherit.** Every program the extension starts (`probe`, and through it `adb` and `flutter`) inherits VS Code's environment, as programs do. The settings' secrets are given only to test runs. The device list, lint and the device view get VS Code's environment without the three secret variables (in any casing). The other terminals the extension opens (Studio, record, convert and the like) get no extra variables but still inherit VS Code's own environment, so if VS Code was started with one of the three set, they have it too. A `${NAME}` in a test reads any environment variable by design, so the environment is not reduced further. The boundary is the trust model above: which binaries run, and in which workspaces. A `probe.yaml` is read by `probe test` and its `tools:` section can name `adb` and `flutter`, so trust a folder only if you trust what it runs.
