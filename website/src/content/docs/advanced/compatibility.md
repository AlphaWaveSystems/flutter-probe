---
title: Compatibility policy (1.x)
description: What stays stable across FlutterProbe 1.x releases - ProbeScript, probe.yaml, the agent protocol and the report JSON - what may grow, what is outside the promise, and how things are deprecated.
---

From **1.0** FlutterProbe follows [semantic versioning](https://semver.org/). This page says what that promises, so you can upgrade within 1.x without rewriting tests, configuration or tooling.

## Frozen in 1.x

Nothing in this list is removed or changes meaning within 1.x. A change that would break it waits for 2.0.

| Surface | What is frozen |
|---|---|
| **ProbeScript** | Every statement and form documented on the [grammar page](/probescript/grammar/) keeps parsing and keeps its meaning. Keywords are not removed or repurposed. The grammar page is checked against the parser by tests, so documentation and behaviour cannot drift apart. |
| **`probe.yaml`** | Every documented key keeps its name, type, meaning and default. A file that works in 1.0 works in any 1.x. |
| **Agent protocol** | The JSON-RPC methods between the CLI and the Dart agent keep their names, parameters and result fields. |
| **Report JSON** | Existing fields of the `--format json` report keep their names, types and meaning. JUnit output keeps following the JUnit XML format. |
| **CLI** | Documented commands and flags keep working. Exit codes keep their meaning (0 passed, non-zero failed). |

## Additive-only

These grow in minor releases, never in a way that changes what existing input does:

- **New statements and forms.** New words are recognised only in positions that were not valid before (the way `start measuring` or `within` were added), so no existing valid file changes meaning.
- **New `probe.yaml` keys, CLI flags and commands**, with defaults that keep the old behaviour.
- **New protocol methods and new fields** in requests and results. Readers ignore fields they do not know.
- **New report fields.** Consumers of the JSON report must ignore fields they do not know.
- **New MCP tools and tool arguments.**

A feature that needs a newer agent says so (`needs flutter_probe_agent >= x`) instead of failing obscurely. A CLI works with any agent of the same major version; it warns when versions differ, and features introduced after the agent's version are unavailable.

## Outside the promise

- **Probe Studio** has its own version line (0.x). Its screens, its automation endpoint and its method names can change in a minor release. Probe Studio never writes anything a 1.x CLI cannot read.
- **Experimental features**, marked as experimental in their documentation and `--help` (hidden or experimental flags included).
- **Human-readable output**: terminal text, log lines and error message wording. Use `--format json` for tooling.
- **The Go packages under `internal/`**, which are not an importable API, and the Dart agent's `src/` files; only the documented public entry points of the agent are API.
- **Prereleases** (`-next.N` canary builds): no guarantees.
- **Security fixes** take priority. If a fix cannot avoid a behaviour change, the change is called out in the CHANGELOG under Security.

## Deprecation

1. The change is announced in the CHANGELOG under **Deprecated**, with what to use instead.
2. The old form keeps working, and prints a warning once per run, for at least **two minor releases and 90 days**, whichever is longer.
3. It is removed only in the next major release, and the major release's CHANGELOG lists every removal.

## Version numbers

CLI, Dart agent, VS Code extension and docs share one version number. Patch releases are fixes; minor releases add things; a major release may break the frozen surfaces above. Pin the CLI and the `flutter_probe_agent` package to the same minor version in CI.
