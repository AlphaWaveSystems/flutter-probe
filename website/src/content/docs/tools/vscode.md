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

### Commands

Access via the Command Palette (`Cmd+Shift+P`):

| Command | Description |
|---------|-------------|
| FlutterProbe: Run Test | Run the test at cursor |
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

The extension looks for the `probe` binary on your PATH. If it is installed elsewhere, configure it in VS Code settings:

```json
{
  "flutterprobe.probePath": "/path/to/bin/probe"
}
```
