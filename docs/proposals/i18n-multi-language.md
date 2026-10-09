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

## Status

- P3 loose matching: implemented (0.20.0).
- P1 system-dialog roles: Android + role mechanics implemented; the iOS label table is being measured per language on real simulators.
- P2, P5, P4: next.

## Releases (pub.dev allows 12 publishes a day, so batch)

- R1: P3 + P1 (0.20.0)
- R2: P2 + P5 (0.21.0)
- R3: P4 (0.22.0)

## Principles

- Opt-in where behaviour changes, no silent changes to existing suites.
- Measured data over guessed translations (every iOS label comes from a simulator run).
- Every new construct lands in the EBNF with a conformance-test example in the same PR.
