# Maestro-competitive roadmap — completion report

**Status: all 19 items complete** (2026-08-15). This is the consolidated summary; per-item detail,
caveats, and honest limitations live in each item's entry in
[`maestro-competitive-roadmap.md`](maestro-competitive-roadmap.md) and its linked
`docs/evidence/` folder. Shipped across releases **v0.11.0 → v0.13.0**.

## Phase 0 — Correctness (shipped in v0.11.0)

| Item | What shipped |
|---|---|
| R-1 | Tap-family verbs now resolve `<var>` placeholders in their selectors (previously only `type`/`see` did) |
| R-3 / PT-27 | Android reconnection token survives OS cache clearing — the periodic re-print now also re-writes the token file |
| R-4 | `see #field contains "…"` reads `EditableText` controllers, not just `Text`/`RichText` |
| V-1 / PT-03 | Scroll targeting re-verified holding on real devices (stacked-route Scrollables) |
| R-5 | `dump tree` / `save device logs` parse correctly (dangling-token misparse fixed) |

## Phase 0.5 — Docs + benchmark groundwork (v0.11.0)

| Item | What shipped |
|---|---|
| D-1 | `dictionary.md` completeness pass — all missing verb rows added |
| D-2 | `ai:` config block documented in `configuration.md` (provider/api_key/model/endpoint/timeout/redact) |
| B-1 | water-sip's 9-flow suite written twice — ProbeScript and equivalent Maestro YAML — as the benchmark corpus |
| B-2 | Reusable benchmark harness: `scripts/bench/run-comparison.sh` + `summarize.py` (later gained `--order` and `--android-adb-port` in G-1) |
| B-3 | N=10 baseline: FlutterProbe 59.6s median / 0% flake vs Maestro 122.6s / 100% flake (~2.1×); device-connectivity finding flagged for replication, later retired in G-1 |

## Phase 1 — Ecosystem parity (v0.12.0, PRs #227–#231)

| Item | What shipped |
|---|---|
| E-1 (#228) | `retry N times` block (re-run-on-failure, stop at first success) + trailing `optional` modifier on tap/type/long press/double tap/clear/see |
| E-2 (#229) | Element-scoped visual regression: `compare screenshot "x" of <selector>` crops both baseline and actual to the widget's bounds |
| E-3 (#230) | Deep links: `open link "…" in the app` via OS intent handling (`am start -a VIEW` / `simctl openurl`). Documented caveat: iOS Simulator cannot cold-launch a terminated app |
| E-4 (#231) | `add media "path"` seeds the camera roll/gallery (`adb push` + media-scan broadcast / `simctl addmedia`), MediaStore-indexed and picker-visible |

## Phase 2 — Native UI bridging (v0.12.0, PRs #232–#233)

| Item | What shipped |
|---|---|
| N-1 (#232) | `tap native` / `see native` / `type native` on Android via uiautomator — pickers, share sheets, anything outside the Flutter tree. Zero new dependencies. Dedicated fixture app added later in #241 |
| N-2 (#233) | iOS native bridging proposal (`docs/proposals/n2-ios-native-ui-bridging.md`): WebDriverAgent, Simulator-first, reusing N-1's syntax unchanged. Proposal only — iOS verbs error clearly by design today. Swift fixture app for the spike shipped in #241 |

## Phase 3 — Compound the lead (v0.12.0, PRs #234–#236)

| Item | What shipped |
|---|---|
| G-1 (#234) | Benchmark re-run with current build, **both orderings** (probe-first + maestro-first): ~2.0–2.1× median wall-clock advantage held both ways, 0% flake across 20 FlutterProbe runs. B-3's device-connectivity finding **did not replicate — retired**, said so publicly. Blog post: `/blog/flutterprobe-vs-maestro-benchmark/` |
| G-2 (#235) | vs-Maestro comparison refreshed with 2026 facts: no physical iOS, ~$250/device/mo cloud, AI assertions at parity with a stronger local-first privacy story, native-UI gap narrowed |
| G-3 (#236) | `probe migrate maestro` hardened: 2.x syntax (setPermissions/retry/assertScreenshot + the corpus's real gaps extendedWaitUntil/scrollUntilVisible/eraseText), recursive directory discovery, relativePoint corruption fixed. 76/76 real flows convert and parse |

## Shipped beyond the roadmap in the same cycle

- **v0.12.0/v0.12.1 recovery + campaign fixes** (#238, #242): issue-#237 reconnect-retry dispatch
  drift, conditional connection-error misrouting, `toggle` double no-op, filler-word recipe names,
  Android `set location`, iOS permission relaunch, CLI-side duration waits — eight real bugs found
  by dogfooding probe against real apps, all with regression tests and live device verification.
- **adb transport forensics** (#239): host adb server flush-timeout socket closes identified as
  the likely #237 drop mechanism, with the raw server log preserved; guest-adbd wedge reproduced
  without Maestro.
- **Full-feature campaign** (#243): every ProbeScript feature exercised on both platforms with a
  local Gemma 4 model for the AI family — Android 32/32, iOS 31/32 (the "failure" being the AI
  correctly catching a real app-layout defect).
- **Native fixture apps** (#241) and the **`migrate_maestro` MCP tool** (#245, v0.13.0) closing
  the MCP surface gap.
- **Docs surface sync** (#240, #244, platform-pages PR): dictionary, syntax, visual-regression,
  README, landing page (version badge now build-time-generated from `VERSION`), VS Code grammar,
  MCP tool descriptions, wiki, and both platform pages.
