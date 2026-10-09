# Multi-language support for FlutterProbe

Goal: one test suite that runs correctly in any app language and on any device language, so developers can test
localized apps without rewriting tests per language.

## Where we are (0.19.5)

| Area | State |
|---|---|
| Locale-independent selectors (`#key`, Semantics identifier, widget type) | works |
| Alternatives (`see any of`, `wait until any of`), regex (`matching`), data-driven strings | works |
| Unicode / RTL text in selectors | works (substring match) |
| Text matching | case-sensitive, literal; no accent/Unicode folding |
| System dialogs (permission alerts, "Open in app?") | English only: `deny permission`, `--grant`, `dismiss` hard-code English labels |
| Device / app locale control from a test | none |
| Localization-key lookup (ARB) | none |
| Running one suite across locales | manual (data-driven) |

## Phases, in order of importance

1. **System dialogs by role (P1).** Developers should write `tap "Don't Allow" in system dialog` once and have it work on
   a German or Japanese device. The button is resolved to a *role* (allow, deny, allow once, allow while using, ok,
   cancel, open) and the dialog's button with the same role is tapped.
   - Android: language-independent resource ids (`permission_allow_button`, `permission_deny_button`, ...) first, then text.
   - iOS: no stable ids, so a table of labels *measured on a real simulator per language* (docs/evidence), extendable in probe.yaml.
   - `deny permission`, `--grant`, `dismiss` and the system-dialog steps all use roles.
2. **Locale control (P2).** `set language "de"` (and `set locale "de_DE"`) in a test, `--locale` on the CLI. Per-app locale so no
   device restart: iOS `AppleLanguages` for the bundle, Android 13+ `cmd locale set-app-locales`; the app is relaunched.
   RTL is exercised by choosing an RTL language (`ar`, `he`, `fa`).
3. **Loose text matching (P3).** Opt-in (`matching: loose` in probe.yaml / `--match-loose`): case, accents/diacritics,
   typographic apostrophes and dashes, Unicode forms and whitespace are folded in text selectors (agent and CLI).
4. **Locale matrix (P5).** `probe test --locales de,ja,ar` runs each test once per locale with the locale set, per-locale results
   and failure screenshots, grouped in the report.
5. **Localization lookup (P4).** `probe.yaml: l10n: {dir: lib/l10n}`; a selector that names an ARB key (`tap l10n "saveButton"`) resolves to the
   text of the current locale, so tests stay readable and follow translation changes. Placeholders/ICU plurals match on
   their static parts.
6. **Docs and surfaces, in every phase:** grammar (EBNF) + conformance test, dictionary/syntax, MCP guide, VS Code
   snippets/grammar, a "Testing localized apps" guide, README, CHANGELOG.

## Also planned: backend awareness (P6, requested 2026-10-09)

React to what backend services and BFFs return, to test behaviour that depends on data the UI does not show.

Finding: the `mock` block is advertised ("when the app calls POST ... respond with 503") but the agent only stores the mock in a
map (`ProbeExecutor._mocks`, read by `mockFor`, which nothing calls and the package does not export); nothing intercepts the app's
traffic. P6 starts by making interception real.

- Agent: install an `HttpOverrides` wrapper at `ProbeAgent.start()` for `dart:io` `HttpClient` (covers `http`, dio's default adapter,
  most Dart clients; not native SDK calls, WebViews or `dart:html`). It (1) records method, url, status, selected headers
  (Authorization/Cookie/Set-Cookie redacted) and a capped body (default 64 KB) per exchange in a ring buffer, (2) applies
  mocks (status, body, headers, delay, connection failure), (3) feeds the existing in-flight tracking used by `wait for idle`.
  Capture can be turned off (`PROBE_HTTP_CAPTURE=false`); it never exists in release builds.
- RPCs: `probe.http_log` (filtered, since a cursor), `probe.http_clear`, `probe.mock` (now effective).
- ProbeScript (all in the EBNF + conformance test):
  - `wait for response GET "/api/orders" [status 200]`
  - `see response "/api/me" status 200` / `contains "premium"` / `json "data.plan" equals "pro"`
  - `store response "/api/me" json "data.plan" as plan` (then `<plan>` in later steps)
  - `if response "/api/me" json "data.plan" equals "pro"` ... `otherwise` (same block rules as `if "X" appears`)
  - `see exactly 2 requests GET "/api/orders"`, `clear recorded requests`
  - `mock GET "/api/orders" status 503 [delay 2 seconds | fail]` (modernised form of the old mock block)
- Docs: a "Testing against backend data" guide, plus the surfaces required by the docs rule.
- Release: R4 (0.23.0), after R1-R3, unless the user reprioritises.

## Status

- P3 loose matching: implemented (0.20.0).
- P1 system-dialog roles: Android + role mechanics implemented; the iOS label table is being measured per language on real simulators.
- Named devices (hard rule, 2026-10-09): implemented (0.20.0). Every simulator/emulator probe opens is named (`device start --name`, AVD names, MCP `name`), every result records device name + id. The locale matrix (P5) reports per device + locale.
- P2 `set language` / `--locale` and P5 `--locales` matrix: implemented (0.21.0), verified on an iOS simulator and an Android 14 emulator.
- P4 `l10n "key"` from ARB: implemented (0.21.0), verified with real gen_l10n files on an iOS simulator.
- P6 backend awareness: next (0.22.0).

## Releases (pub.dev allows 12 publishes a day, so batch)

- R1: P3 + P1 (0.20.0)
- R2: P2 + P5 (0.21.0)
- R3: P4 (0.22.0)

## Principles

- Opt-in where behaviour changes, no silent changes to existing suites.
- Measured data over guessed translations (every iOS label comes from a simulator run).
- Every new construct lands in the EBNF with a conformance-test example in the same PR.
