---
title: Using FlutterProbe from AI Agents
description: A practical guide for LLM agents (Claude, Cursor, Copilot, local models) driving FlutterProbe through MCP or the CLI — the loop to follow, how to read failures, and the traps to avoid.
---

FlutterProbe is built to be driven by an AI agent as much as by a person. This page is the playbook. It assumes the
agent either has the [MCP server](/tools/mcp/) (`probe-mcp`, 21 tools) or can run the `probe` CLI in a shell. A
machine-readable summary lives at [`/llms.txt`](https://flutterprobe.dev/llms.txt).

## The loop

1. **Find a device** — `list_devices` (or `probe device list`). Start one with `start_device` if none is booted.
2. **Look before you write** — `get_widget_tree` (or `probe studio`'s inspector) shows the real widget keys and
   texts on screen. Prefer `#key` selectors over text: keys survive copy changes and translations.
3. **Write a small test** — `write_test` validates the syntax before writing. Start with one flow of 5–10 steps.
4. **Run it with detail** — `run_tests` with `flags: "-v --format json"`. Verbose output prints every step and its
   timing.
5. **Read the failure, fix, rerun** — see below. Change one thing at a time.
6. **Only then widen** — add tags (`@smoke`), more files, `--shard`, `--parallel`.
7. **Optional: explain failures** — `triage_failure` asks the configured AI model (a local one works) for a probable
   cause. It is advice only and never changes results.

## Reading a failure

Failures look like this:

```
line 14: wait until "Saved" appears: rpc error -32000: Timed out waiting for "Saved" to appear —
visible texts: ["Settings", "Daily Goal"], keys: ["nav_tab_settings", "goal_field"]
```

| Part | Meaning |
|---|---|
| `line 14` | the failing line of the `.probe` file |
| `wait until "Saved" appears` | the step (text typed into system fields is masked as `****`) |
| `rpc error -32000` | timeout. `-32001` widget not found, `-32002` assertion failed, `-32099` connection closed |
| `visible texts / keys` | what the user could actually see at that moment — compare it to what you expected |

Decide from that list: wrong selector (use a key that *is* listed), wrong screen (a navigation step is missing), or
timing (add `wait until "<next screen's text>" appears`, or `wait for idle` after closing a dialog).

## Rules that prevent most failures

- **Wait for the screen, then act.** After any navigation, `wait until "<text on the next screen>" appears` before
  tapping. A tap returns before the route has finished building.
- **Prefer positive assertions.** `see "Dashboard"` is reliable. `don't see "X"` can pass for the wrong reason when
  the widget simply is not built yet (lists build rows lazily).
- **Rows below the fold do not exist yet.** Use `scroll down until "X" appears`; `scroll down` reveals *later* content.
- **One test, one purpose, self-contained.** Log in at the start and out at the end; do not rely on another test's
  state. Do not use `clear app data` on physical iOS devices.
- **Optional dialogs:** `tap "Skip" if visible` is two lines; do not branch with `if "X" appears` to detect which
  screen is active.
- **Secrets never go in a test.** Use environment variables: `type "$PROBE_SANDBOX_PASSWORD" into system field "Password"`.
  Never put credentials in a `.probe` file, a prompt or a command line.

## Things the Flutter agent cannot see

Permission alerts, the StoreKit sign-in sheet, share sheets and other OS UI are outside the app. Use the
[system-dialog steps](/tools/system-dialogs/) (`tap "Allow" in system dialog`, `dismiss system dialog`) rather than
trying to find them with `see`. For Android native UI there are also `see native` / `tap native`.

## Choosing flags

| Goal | Flag |
|---|---|
| machine-readable results | `--format json -o reports/results.json` (or `junit`) |
| per-test streaming | `--format json --stream` |
| find slow/stuck steps | `-v` |
| subset | `--tag smoke`, a file list, `--shard 1/3` |
| pre-grant OS permissions | `--grant notifications,camera` |
| avoid port collisions | `--agent-port 48700` with the app built with `--dart-define=PROBE_PORT=48700` |
| physical device over WiFi | `--host <ip> --token <token>` |

## Using a small local model

The failure text above is deliberately self-contained, so even a small local model can act on it. Point `ai:` in
`probe.yaml` at an OpenAI-compatible server (`provider: local`) and run `probe ai doctor` to see whether the model
accepts images. A text-only model works with `ai.vision: false`. See [AI & local models](/tools/ai/).

## Do not

- Do not guess selectors: read the widget tree.
- Do not retry a failing run unchanged; read the message first.
- Do not loosen assertions to make a test pass.
- Do not run several agents against one simulator and one agent port: give each its own `PROBE_PORT`.
