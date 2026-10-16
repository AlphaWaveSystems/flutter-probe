# PRD: Run Recording for ProbeStudio and the CLI

**Status:** Draft, for review
**Author:** Patrick Bertsch (drafted with Claude Code)
**Date:** 2026-10-16
**Related:** [AI visual assertions PRD](ai-visual-assertions-prd.md), [Gap analysis, August 2026](../roadmap/gap-analysis-2026-08.md)

---

## 1. Problem statement

When a FlutterProbe test fails, the report names the failing step and, at most, shows one
screenshot. To learn why, a developer re-runs the test, watches it, guesses which backend
response the app received, and rebuilds the device state by hand. This is slow. It is worst on CI,
where the device is gone before anyone looks.

FlutterProbe already sees most of what happened. The runner emits an event for every step, the
agent records the app's HTTP traffic, and the screen and widget tree can be read at any point. No
part of the product joins these into one record that can be shared, opened offline or replayed.

## 2. Goals

- Record a test run step by step. For each step: the action, its result and timing, the screenshot
  and widget tree around it, the HTTP exchanges it caused, and the element it acted on.
- Capture backend traffic. App traffic through `dart:io` comes from the agent. Native SDK and
  WebView traffic comes from a device proxy.
- Write a self-contained bundle that opens offline in a browser and can be attached to a bug
  report.
- Generate a replayable test: a `.probe` file that repeats the same steps with the recorded
  backend responses registered as mocks.
- Keep recording opt-in. Redact credentials before anything reaches disk.

## 3. Non-goals

- No hosted service. Bundles stay on the machine. Probe Cloud keeps uploading only the scrubbed
  report it already sends.
- No ProbeScript grammar change in v1. Replay uses the existing `mock` command.
- Recording is off by default. Normal runs keep their timing and output.
- Not a security testing tool. The proxy reads the test's own traffic. It does not probe the app.
- Physical devices are supported only where the device can be pointed at the proxy and trusts its
  CA. Those steps are documented per platform, not automated.

## 4. Users and use cases

- A developer whose test failed on CI wants the same run replayed locally, with the same backend
  responses.
- A developer chasing a flaky test wants the timeline of steps and responses for the failing
  attempt.
- A QA lead attaches a bundle to a bug report. Credentials are redacted. Personal data visible in
  screenshots is the test author's responsibility (section 7).
- A team wants to know which backend call a step depended on.

## 5. What exists today

These are the pieces the design builds on. Each was checked against `origin/main`.

| Piece | Where | What it provides |
|---|---|---|
| Step events | `internal/runner/step_event.go` | Started, passed, failed and skipped events per step, with file, line, attempt, depth and duration. Studio consumes them through `OnStep` (`studio/app.go`). |
| App HTTP capture | `probe_agent/lib/src/http_capture.dart`, installed at agent start (`probe_agent/lib/src/agent.dart`) | Every `dart:io` exchange: method, URL, status, duration, headers, and bodies capped at 64 KiB each. Credential headers and query values are redacted. A ring buffer holds the last 300 exchanges. |
| HTTP log RPC | `probe.http_log` (`probe_agent/lib/src/executor.dart`), `internal/probelink/protocol.go` | A `since` cursor on `seq`, an `epoch` that changes when the app restarts, and `latest`. Empty method and pattern match every exchange. |
| Screen state | `probe.screenshot`, `DumpWidgetTree`, `SelectorBounds` (`internal/probelink/http_client.go`) | Per-step screenshot, widget tree, and the bounds of the element a step targets. |
| Interaction recorder | `probe record` (`internal/cli/record.go`), Studio recording (`studio/recorder.go`) | Taps and text entry turned into ProbeScript lines. |
| Video | `VideoRecorder` (`internal/runner/recorder.go`) | Whole-run device video. |
| Redaction | `studio/ai_redact.go`, `internal/hosted/scrub.go`, `http_capture.dart` | Credential patterns, masking of obscured fields in the widget tree, and scrubbing of the hosted report. |
| Reports | `internal/report/html.go`, `internal/runner/reporter.go` | Self-contained HTML, JSON and JUnit output. |

The gaps:

1. HTTP exchanges are not tied to steps. The runner reads the HTTP log only for HTTP assertions and
   failure messages (`internal/runner/http_steps.go`).
2. `StepEvent` carries durations but no wall-clock time, so traffic from outside the agent cannot
   be placed in a step.
3. `OnStep` is documented as non-blocking ("must not block"). Recording needs work that waits for
   the device, so it needs its own synchronous hook.
4. There is no bundle format and no viewer.
5. Native SDK and WebView traffic is not captured at all.
6. Studio has no recording switch and no timeline view.

## 6. Proposed design

### 6.1 Bundle layout

One bundle per recorded test attempt:

```
reports/recordings/<run-id>/<device-name>/<test-slug>/
  manifest.json      run facts and capture settings (below)
  timeline.json      ordered steps, each linked to its evidence
  http/NNNN.json     one HTTP exchange, bodies redacted
  tree/NNNN.txt      widget tree dump, redacted
  screenshots/       one PNG per capture, redaction regions applied
  index.html         viewer, no external requests
  reproduce.probe    replayable test (added in PR 3)
```

`manifest.json` holds the schema version, run id, test name, file path and SHA-256 of the file,
attempt number, device name, platform, OS version, agent version, CLI version, start and end times,
final status, capture settings, and counters for anything that was truncated.

`timeline.json` holds one entry per step, in order. Each entry has the step index, depth, file,
line, description, status, error, start time, duration, the names of its before and after
screenshots and tree dump, the interaction (kind, target bounds, tap point), the ids of its HTTP
exchanges, and the source of each exchange (`dart_io` or `proxy`).

### 6.2 Capture pipeline

Recording adds two synchronous points to the executor, one before a step is dispatched and one
after it finishes. They run only when recording is on, so the normal path is unchanged. `OnStep`
stays non-blocking.

- Before a step: read the HTTP log cursor, and take the "before" screenshot and tree when
  `record.screenshots` asks for them.
- After a step: read the HTTP log from the cursor, attribute the exchanges between the two cursor
  positions to this step, and take the "after" screenshot and tree.

A device runs its steps one at a time, so the exchanges between two cursor positions belong to the
step that ran in between. Background requests are attributed to whichever step was running. The
bundle says so; it does not guess.

Edge cases:

- **App restart.** An `epoch` change means `seq` started over. The recorder resets its cursor,
  keeps the exchanges it already has, and marks the step `app_restarted`.
- **Buffer overflow.** The agent keeps 300 exchanges. A gap in `seq` larger than what the recorder
  saw means some exchanges were lost. The manifest and timeline record the gap as `truncated`, with
  its size.
- **Interaction target.** Each step's element is resolved with `SelectorBounds` before the action
  runs, so the timeline can show where a tap landed.

Overhead is measured in PR 1 and published with it. Per-step screenshots are the expensive part.

### 6.3 Native capture through a device proxy (PR 2)

- **Proxy.** One loopback proxy per device, started by the CLI or by Studio for the length of the
  run. It is off unless recording is on with `native: proxy`. It binds to loopback. It binds to a
  LAN address only when a physical device is part of the run.
- **Per-run CA.** Each run generates a new CA. Its key is written to a `0600` file in the run's
  directory, is never reused, and is removed when the run ends. A cleanup at the next start removes
  anything a crashed run left behind.
- **Trust, Android.** Apps targeting API 24 and up do not trust user-installed CAs by default. They
  trust them only when their network security config opts in with `debug-overrides`, which applies
  to debuggable builds only ([Android network security configuration](https://developer.android.com/privacy-and-security/security-config)).
  The CLI does not edit app code. The docs give the config snippet.
- **Trust, iOS simulator.** The CA is added to the simulator's trust store for the run and removed
  afterwards.
- **Trust, physical devices.** Manual steps, documented per platform, not automated in v1.
- **What is captured.** Traffic that honours the system proxy setting. WebView coverage differs by
  platform and must be verified before the docs claim it.
- **Pinned connections.** A pinned TLS connection fails at the proxy. The bundle records it as a
  `tls_failed` exchange with the host, so the gap is visible instead of silent.
- **Deduplication.** A request can reach both the `dart:io` capture and the proxy. When a
  `dart:io` exchange and a proxy exchange match on method, host, path and time window, the bundle
  keeps the `dart:io` one, which has app-side detail, and marks the proxy one as a duplicate. This
  rule is verified against the fixture apps in PR 2.
- **Attribution.** Proxy exchanges are attributed to steps by the `At` timestamp on `StepEvent`
  (added in PR 1).
- **Dependency.** The proxy needs a new Go dependency. It goes through license review first.

### 6.4 Redaction

Redaction runs before anything is written to disk. Unredacted data is never written.

- **Headers.** The credential headers list from `http_capture.dart` (authorization, cookies, API
  key headers). The same list applies to proxy traffic on the CLI side.
- **Query values.** The credential names list from `http_capture.dart` (`key`, `token`,
  `access_token`, `sig`, and similar).
- **Bodies.** The credential shapes from `studio/ai_redact.go` (API key prefixes, GitHub tokens,
  `Bearer` values, long hex and base64 runs), and JSON fields named like `password`, `token`,
  `secret`, `authorization`, `refresh_token` or `card_number`.
- **Widget tree.** The obscured-field masking `redactTree` already applies.
- **Screenshots.** The regions named in the existing `redact:` rules (`internal/config/config.go`),
  blacked out before the image is saved.

Redaction is best-effort. A credential shape the rules do not know is not caught. The docs say so
plainly and make no "PII-safe" claim. The two redaction implementations (Studio and the CLI) are
consolidated into one package in a later PR, not in v1.

### 6.5 Replay (PR 3)

- `reproduce.probe` is generated from `timeline.json`: the same steps in the same order, with a
  `mock` for each recorded exchange a step depended on. Mocks match on method and URL, not on the
  request body.
- A URL that contains a timestamp, a random id or a token is flagged in a comment. The recorded
  response is still used.
- Replay uses the agent's mock path for `dart:io` traffic and the proxy's mock mode for native
  traffic. A native call is replayed only if the recording captured it.
- Replay is best-effort. If a step depends on a value the mock does not reproduce, the step fails
  and the report names the exchange that had no match.
- The existing `mock` command is used. If it lacks a form replay needs, that is a grammar change,
  and the project rule requires an EBNF update and a conformance test with it.

### 6.6 Entry points

- **CLI.** `probe test --record`, with `--record-dir <path>` to choose the directory.
- **Config.** A `record:` block in `probe.yaml`: `screenshots` (`every_step`, `on_failure` or
  `never`), `native` (`off` or `proxy`), and `max_bytes`. Everything defaults to off.
- **Studio.** A "Record run" switch in the Results pane, and an "Open recording" link after a run.
- **MCP.** The new flag gets an updated tool description and examples, as the project rules
  require.

### 6.7 Viewer

- A single `index.html` with inline CSS and JavaScript. It makes no network requests. A test checks
  the asset for `http://` and `https://` loads.
- Shows a timeline of steps with file and line, status, duration and errors. Each step opens to
  its before and after screenshots, its widget tree, its HTTP exchanges with status and timing and
  redacted bodies, and its interaction target.
- Opens from disk with no server, including on a machine the bundle was copied to.

## 7. Security and privacy

- **Opt-in.** Recording is off in every default. A run records only when the flag or the switch
  asks for it.
- **Local only.** Bundles never leave the machine in v1. The docs tell users to keep `reports/` out
  of version control. The CLI warns when the recording directory is inside a git work tree and is
  not ignored.
- **Per-run CA.** It exists only for the run, is never reused, and is removed afterwards. Its key
  never leaves the run directory.
- **Proxy exposure.** Loopback only, unless a physical device is in the run.
- **Personal data in screenshots.** Redaction rules apply where the author names them. Other
  visible data is the test author's responsibility. The docs say this without a "PII-safe" claim.

## 8. Success metrics

- With recording off, terminal and JSON output are unchanged. A golden-file test enforces this.
- Per-step overhead for `screenshots: every_step` is measured and published in PR 1. The target is
  set from that measurement, not guessed here.
- A failed step's bundle opens offline and shows the request and response that came before the
  failure, under that step.
- In PR 3, at least one recorded failure replays to the same failing step on the project's
  end-to-end fixture apps.
- No bundle written by the test corpus contains a credential-shaped string the redaction tests
  cover.

## 9. Risks and open questions

- **Pinned apps.** Their traffic cannot be captured. This is a known limit and is shown in the
  bundle.
- **WebView coverage.** Unverified. It is not documented as supported until it is.
- **Deduplication.** The `dart:io` and proxy rule in 6.3 is a hypothesis. It is checked in PR 2.
- **The synchronous hook.** It must stay off the hot path when recording is off. Its cost is part of
  the PR 1 measurement.
- **Multi-device runs.** One proxy per device and one bundle per device per test. Port allocation
  details are open.
- **Serial execution per device.** Assumed, not yet confirmed. Attribution depends on it.
- **Replay fidelity.** Timestamps, random ids and server-side state can make a replay diverge.
  Replay is best-effort, as 6.5 says.
- **Size.** A 300-exchange buffer with 64 KiB bodies can hold about 37 MiB per device, before
  screenshots. `record.max_bytes` caps a bundle. At the cap, later steps are recorded without
  screenshots and the manifest says so.
- **Two redaction implementations** until they are consolidated.
- **Licensing.** The proxy dependency and the placement of the new code (CLI under BSL, agent under
  MIT) follow the same review as the AI assertions PRD.

## 10. Rollout

- **PR 0 (this PR).** This document.
- **PR 1.** `internal/record`: the bundle writer, the HTTP cursor reader, redaction and the viewer.
  Adds `StepEvent.At` and the synchronous recording hook. Adds `probe test --record` with `dart:io`
  capture, per-step screenshots and widget tree. Includes tests, docs, the CHANGELOG entry, and the
  MCP tool description.
- **PR 2.** The device proxy: per-run CA, iOS and Android trust setup, deduplication, and
  `tls_failed` markers. Platform trust docs are verified against the fixture apps.
- **PR 3.** `reproduce.probe` generation and the replay path, plus the Studio "Record run" switch,
  timeline view and "Open recording" link.

Every PR ships with `record:` defaulting to off.

## 11. Decisions

Decided by the maintainer on 2026-10-16:

- Native SDK and WebView traffic is captured through a device proxy with a per-run CA.
- Capture is opt-in and full: screenshot, widget tree and HTTP per step, when `--record` or the
  Studio switch is on.
- The output is a viewer plus a replayable test.
- Credentials are redacted. The rest of each body is kept.

Open for review: section 7 (privacy defaults), section 9 (risks), and the PR split in section 10.
