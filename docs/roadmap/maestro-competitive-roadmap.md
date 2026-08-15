# FlutterProbe vs. Maestro — Competitive Roadmap

**Status:** Living document — update checkboxes and evidence links as work lands. Do not let this
drift from reality; if a task's scope changes, edit the task, don't leave stale text.

**Baseline:** `main` @ v0.11.0 (synced 2026-08-14). Full competitive analysis:
`docs/roadmap/gap-analysis-2026-08.md` (archived snapshot — the artifact this doc supersedes for
day-to-day tracking).

**Test targets:** two real Flutter apps outside this repo, used for reproduction and comparison
evidence:
- `~/dev/water-sip` — Firebase/IAP consumer app. Has an existing `.probe` suite
  (`tests/*.probe`) but **no Maestro suite** — Maestro flows must be written from scratch here.
- `~/dev/nect-flutter` — Firebase/Firestore social app. Has both a `.probe` suite
  (`tests/smoke/*.probe`, 10 flows) **and a real, git-tracked Maestro suite**
  (`.maestro/flows/**`, 66+ flows across auth/groups/home/posts/navigation/settings/profile/
  messaging/help/invitations/subscription) — this is the primary comparison base.

## Working process (every task, no exceptions)

1. `git worktree add .worktrees/<branch> -b <type>/<slug>` off current `main`.
2. Implement + add/update a regression test in the same worktree.
3. Run the relevant test suite locally; capture the run (pass/fail output, and for
   device-dependent fixes, a screenshot/video from the real device run) into
   `docs/evidence/<task-id>-<yyyy-mm-dd>/`.
4. Update `CHANGELOG.md` (root, and `probe_agent/CHANGELOG.md` if the Dart agent changed).
5. Commit specific files only (never `-A`/`.`). No `Co-Authored-By` line (project convention).
6. Push, open a PR referencing this doc's task ID, wait for CI, merge (squash), delete the branch
   and worktree.
7. Check the box below and link the evidence folder + PR number.

Status legend: `[ ]` not started · `[~]` in progress · `[x]` done (linked) · `[!]` blocked (note why).

---

## Phase 0 — Reliability (close what's already open in `IMPROVEMENT_TASKS.md`)

Per `IMPROVEMENT_TASKS.md`'s own "Suggested working order" (revised 2026-07-05) and `DONE.md`'s
"Deferred / open items", exactly this is left from the hardening effort. Nothing here is
speculative — every item has a documented repro or root cause already.

- [x] **R-1 — Tap-family verbs never resolve `<var>` placeholders in their selector.**
  Still-open half of the PT-02 addendum. Scope grew during implementation: the same missing
  resolution also affected `clear`, `swipe`/`scroll` targets, `drag ... to ...`, and the
  `if visible` pre-check — not just tap/doubleTap/longPress. Fixed with a single
  `resolveSelector()` helper in `internal/runner/executor.go`, applied at every selector-taking
  call site (including `runAssert`, for consistency with the rest). Regression tests:
  `TestResolveSelector_ResolvesVariablePlaceholder`, `TestResolveSelector_LeavesPlainTextUnchanged`.
  Real-device evidence (water-sip, Android emulator): `docs/evidence/r1-tap-var-resolve-2026-08-14/`.
  PR: #217

- [!] **R-2 / PT-25 — Android WebSocket drop (`close 1006`) root cause, still open.** Attempted
  with real background load against water-sip (Firebase Crashlytics/Analytics/RemoteConfig/
  Performance/InAppMessaging + IAP billing calls): a ~100s continuous-foreground stress session
  did **not** reproduce the drop — clean negative result, logcat-confirmed. A first attempt with
  deliberate app backgrounding conflated an expected lifecycle disconnect with the actual bug and
  wasn't a real test of the hypothesis. Rules out "background SDK chatter alone, ~100s" as
  sufficient; next attempt should try longer duration (10+ min), genuine network-condition changes
  (needs `toggle airplane mode`, tracked as Phase 1 E-6), or a physical device instead of an
  emulator. See `docs/evidence/r2-pt25-ws-drop-2026-08-14/` for full writeup and reusable repro
  scripts.
  PR: —

- [x] **R-3 / PT-27 — Android reconnection token lives in an OS-clearable cache dir.**
  Fixed per suggested fix (c) from `IMPROVEMENT_TASKS.md`: the existing every-3-second token
  re-print in `server.dart` now also re-attempts `_writeTokenFile()`. Regression test:
  `token_reprint_test.dart`, using a new `@visibleForTesting tokenFileWriteAttempts` counter
  (the only thing observable from a host test, since the real file write no-ops off-device).
  Real-device attempt against nect-flutter surfaced a *pre-existing, unrelated* environment issue
  — `cache/probe/` was never created at all on that build, independent of this fix — documented
  honestly rather than forced; likely the same root cause as R-2/PT-25's WS-drop investigation.
  See `docs/evidence/r3-android-token-cache-clear-2026-08-14/`.
  PR: —

- [x] **R-4 — `_textOf` doesn't read `EditableText`, so `see #field contains "…"` returns empty
  for any `TextField`/`TextFormField`.** Actually in `probe_agent/lib/src/executor.dart` (not
  `finder.dart` as originally noted). Fixed by reusing the existing `_findTextController` up/down
  search rather than duplicating a tree walk. Regression tests:
  `see_contains_editabletext_test.dart` (3 widget tests, confirmed to fail pre-fix). Real-device
  attempt against nect-flutter hit an unrelated emulator/connectivity issue (`unexpected EOF` on
  WS dial, likely same class as R-2/PT-25) — documented honestly rather than forced; see
  `docs/evidence/r4-textof-editabletext-2026-08-14/`.
  PR: —

- [x] **V-1 / PT-03 — Re-verify scroll targeting holds.** Confirmed holding: a real-device test
  against water-sip (18 quick-adds overflow one screen of History's lazy `ListView.builder`, then
  `scroll down`) passes cleanly, consistent with the existing `scroll_scrollposition_test.dart`
  widget-test evidence. No code change needed. nect-flutter's Android build has an unrelated
  connectivity issue in this environment (see R-2/R-3/R-4 evidence) so water-sip stood in.
  See `docs/evidence/v1-scroll-pt03-2026-08-14/`.
  PR: —

- [x] **R-5 — `dump tree` / `dump the widget tree` / `save device logs` all misparse.** Fixed by
  explicitly consuming `TOKEN_WIDGET`/`TOKEN_TREE`/`TOKEN_DEVICE`/`TOKEN_LOGS` when present,
  mirroring `parseActionClose`'s existing pattern. Regression tests: `TestParser_DumpTree_Short`,
  `TestParser_DumpTree_Long`, `TestParser_SaveDeviceLogs` (all confirmed to fail pre-fix with
  "step count: got 2, want 1"). Real-device evidence against water-sip:
  `docs/evidence/r5-dump-tree-save-logs-2026-08-14/`.
  PR: —
  PR: —

- [x] **D-1 — `dictionary.md` hygiene.** Added all missing rows: `store`, `open link`, `log`,
  `close keyboard`, `save device logs`, `see ... is focused` (+ note that state suffixes compose),
  `wait for animations to end`, relational selectors, ordinal+key composition, filler words `on`/
  `into`, and three new sections (AI-Powered Assertions, Biometrics, Signals, Composite Tests).
  Verified with `npm run build` in `website/` — builds clean, `/probescript/dictionary/` renders.
  **Follow-on gap found, not fixed here (new task, out of D-1's scope):** `probe.yaml`'s `ai:`
  block has no entry in `advanced/configuration.md` at all — the dictionary intentionally doesn't
  link to a guide page for it since none exists yet.
  PR: —

- [x] **D-2 — document the `ai:` config block.** Added a `### ai` section (provider/api_key/model/
  endpoint/timeout/redact, all pulled from `AIConfig`'s real field docs and `validateAIConfig`'s
  actual fail-fast behavior, not guessed) plus an `ai:` block in the Full Example YAML, and a
  cross-reference to the dictionary's "AI-Powered Assertions" section. Verified with `npm run
  build` — the `#ai-powered-assertions` anchor exists in the built dictionary page and the link
  resolves to it exactly.
  PR: —

---

## Phase 0.5 — Benchmark harness (build once, reuse every phase after)

Moves G-1 from "phase 3" up front: we now have a real, extensive Maestro suite
(`nect-flutter/.maestro/flows/`) and a `.probe` suite for the same app
(`nect-flutter/tests/smoke/`), so the comparison harness can be built immediately instead of
waiting.

- [x] **B-1 — Pick a representative flow slice and write the missing half.** water-sip: wrote and
  live-verified a full 9-flow Maestro suite matching `tests/smoke.probe` 1:1 (8/9 pass reliably;
  the 9th is a genuine, documented tool-speed finding, not an authoring bug — see below). PR open:
  water-sip#51. nect-flutter: static-analysis mapping of all 9 `tests/smoke/*.probe` files against
  the real `.maestro/flows/**` suite (live execution blocked by the same connectivity issue as
  R-2/R-3/R-4) — 7/9 full parity, 2 partial/gap (browse-feed scroll, filter-categories
  exhaustiveness), both narrower on the Maestro side. Two reusable findings surfaced along the
  way: (1) Maestro's out-of-process accessibility polling can miss a narrow-window UI element
  (water-sip's Undo snackbar) that FlutterProbe's in-process tree access catches reliably — real
  evidence for G-1's speed thesis; (2) Maestro's text selector needs an explicit `(?s)` DOTALL flag
  to match two-line `content-desc` labels, undocumented, no FlutterProbe equivalent gotcha.
  Full writeup: `docs/evidence/b1-flow-mapping-2026-08-14/`.
  PR: —

- [x] **B-2 — Harness script.** `scripts/bench/run-comparison.sh` + `scripts/bench/summarize.py`
  (stdlib-only) run both suites N times against the same device, capture wall-clock + probe's JSON
  report + Maestro's JUnit XML per run, and print a median/P90/flake-rate comparison table.
  End-to-end smoke-tested (N=2, water-sip) — correctly parsed both output formats and correctly
  flagged the known `undo-last-entry` flake rather than reporting a false pass. See
  `docs/evidence/b2-harness-smoketest-2026-08-14/`. A real N≥10 baseline is B-3.
  PR: —

- [x] **B-3 — Baseline run + published numbers.** N=10 per tool against water-sip's 9-flow suite.
  **FlutterProbe: 10/10 clean, 59.6s median, 0% flake.** Maestro: 0/10 fully clean — runs 1–6 hit
  only the known `undo-last-entry` timing flake (~122s median across those); run 7 added two more
  failures and logged an `IOException: device offline` from Maestro's own driver-uninstall step;
  runs 8–10 couldn't connect to the device at all, which **remained offline even after the
  benchmark session ended** (`adb devices` confirmed empty independently). ~2.1x slower median
  wall-clock on the runs that executed, before even counting the connectivity collapse. The
  device-connectivity finding needs a reversed-order re-run before being cited as a general
  Maestro-stability claim (documented honestly as a replication candidate, not overclaimed).
  Full data + per-run raw logs: `docs/evidence/benchmark-baseline-2026-08-14/`.
  PR: —

---

## Phase 1 — Flow ergonomics (AI assertions already shipped in v0.11.0 — removed from scope)

- [x] **E-1 — `retry N times` block + `optional` step modifier.** New `RetryStep` AST node
  (parser + executor, mirrors `LoopStep`/`repeat` but stops at first success); `Optional bool` on
  `ActionStep`/`AssertStep`, handled generically in `runStep` (attempt the step, swallow non-
  connection errors with a warning). Matches Maestro's `retry` block and `optional: true`.
  Regression tests: 5 parser tests + 5 executor tests (scripted client failing N times then
  succeeding, for `retry`; always-failing for `optional`'s swallow + the non-optional regression
  guard). Real-device evidence against water-sip (Android emulator): all 3 designed outcomes
  confirmed exactly (retry executes cleanly, optional swallows a genuine failure with the expected
  warning, the non-optional identical case still fails). `dictionary.md` updated.
  See `docs/evidence/e1-retry-optional-2026-08-15/`.
  PR: —

- [x] **E-2 — Element-scoped visual regression (`compare screenshot "x" of "Widget"`).** New
  `visual.CropToBounds`, using the existing `probe.selector_bounds` RPC (already powers AI
  redaction) rather than new geometry plumbing. Regression: 3 `internal/visual` tests (correct
  region extracted — not just correct size — plus clamping and out-of-bounds error handling) + 3
  parser tests. Real-device evidence against water-sip: baseline-vs-itself passes at 0% diff,
  baseline-vs-genuinely-changed-content fails at a precise 37.61% diff, confirmed cropped to
  68×44px (not full-screen). `dictionary.md` updated.
  See `docs/evidence/e2-element-visual-regression-2026-08-15/`.
  PR: —

- [x] **E-3 — Deep links: `open link` into the app (not just external browser).** `simctl openurl`
  / `am start -a android.intent.action.VIEW` via `DeviceContext`, alongside the existing
  external-browser `open link`. New `open link "url" in the app` / `into the app` / `in app`
  suffix; CLI-side dispatch, cloud mode skips with a warning. Parser + executor unit tests.
  Real-device evidence against water-sip's genuine `watersip://` scheme on both Android emulator
  and iOS simulator — Android proves a full foreground-from-browser round trip; iOS proves the
  running-app case and documents a real platform limitation: `simctl openurl` cannot cold-launch a
  fully-terminated app (it raises an unactionable "Open in App?" confirmation dialog), unlike
  Android's `am start`. `CHANGELOG.md` and `dictionary.md` updated.
  See `docs/evidence/e3-deep-links-2026-08-15/`.
  PR: —

- [x] **E-4 — `add media` — seed camera roll / gallery.** `simctl addmedia` / `adb push` + media
  scan broadcast. Unblocks image-picker-adjacent flows (relevant to R-3/PT-27's repro too).
  Parser + executor unit tests, including the same PT-23-style recipe-name collision guard `open`
  needed. Real-device evidence against water-sip on both Android emulator (confirmed
  MediaStore-indexed via `content query`, not just written to disk) and iOS simulator (confirmed
  visually via the Photos app). Documents a real scope limitation found along the way: seeding the
  media store doesn't drive the app's own native image-picker UI to select the photo — that's
  Phase 2's N-1/N-2 gap, not this feature's. `CHANGELOG.md` and `dictionary.md` updated.
  See `docs/evidence/e4-add-media-2026-08-15/`.
  PR: —

---

## Phase 2 — Native UI bridging (per the existing PT-13 proposal, Android first)

`docs/proposals/pt13-native-ui-bridging.md` already scoped this: permission dialogs are already
solved via OS-level bypass; the real gap is image/file pickers and share sheets, which have no
bypass. Recommended path from the proposal: Android via `uiautomator`, iOS deferred to its own
proposal.

- [ ] **N-1 — `tap native "…"` / `see native "…"` / `type native "…"` on Android via
  `uiautomator`.** Dispatch through `DeviceContext` (same path as install/launch/force-stop/grant),
  entirely outside the Dart agent. Regression test: a fixture app or water-sip/nect-flutter's
  native image-picker/share-sheet surface.
  PR: —

- [ ] **N-2 — iOS native bridging proposal.** Write the proposal (own doc, per PT-13's
  recommendation) before any code — needs a concrete justifying flow (e.g. nect-flutter's photo
  picker or share sheet on iOS).
  PR: —

---

## Phase 3 — Compound the lead

- [ ] **G-1 — Publish the benchmark.** Once Phase 0/0.5/1 land, re-run B-2's harness, write up the
  comparison (blog post + flutterprobe.dev numbers), using the now-current data instead of the
  Phase 0.5 baseline. Before publishing, replicate B-3's device-connectivity finding with reversed
  run order (Maestro first, then probe) — as recorded it can't distinguish "Maestro's own
  driver/device management is less stable under sustained runs" from "the emulator happened to be
  the one dying regardless of which tool was running." The ~2.1x median wall-clock gap on runs
  that did execute doesn't have this confound and is safe to cite as-is.
  PR: —

- [ ] **G-2 — Refresh `vs-maestro` comparison pages** with 2026 facts gathered in the gap analysis
  (no physical iOS, semantics-only Flutter, $250/device/mo cloud, AI assertions now at parity
  with a stronger privacy story, native-UI gap narrowed).
  PR: —

- [ ] **G-3 — Harden `probe migrate maestro`** against 2.x Maestro syntax
  (`setPermissions`, `relativePoint`, `retry`, `assertScreenshot`) using nect-flutter's real
  66-flow suite as the test corpus — a uniquely good migration-fidelity testbed we now have
  on hand.
  PR: —

---

## Change log for this document

- 2026-08-14 — Created. Superseded the original artifact's Phase 1 (AI assertions) entirely —
  confirmed already shipped in v0.11.0 (`see ... with ai`, `assert no visual defects with ai`,
  `ai.provider: local`, `read ... with ai into <var>`). Reduced Phase 0 from a speculative list to
  the exact remaining items in `IMPROVEMENT_TASKS.md`/`DONE.md`. Added Phase 0.5 after discovering
  nect-flutter's real 66-flow Maestro suite.
