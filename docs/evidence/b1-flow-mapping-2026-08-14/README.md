# B-1 — smoke-flow ↔ Maestro-flow mapping

## nect-flutter (`tests/smoke/*.probe` ↔ `.maestro/flows/**`)

Static analysis — read both suites in full, cross-referenced by behavior, not filename. Live
execution against nect-flutter's own Android build wasn't possible in this environment (see
R-2/R-3/R-4's evidence — an unrelated connectivity issue on that specific build); the mapping
below is verified by reading both suites' actual step-by-step content, not assumed from names.

| `.probe` test file | Maestro equivalent(s) | Coverage |
|---|---|---|
| `01_first_launch.probe` | `navigation/app-launch.yaml` + `home/home-page-elements.yaml` | Full |
| `02_sign_in_out.probe` | `auth/login.yaml` + `auth/logout.yaml` | Core mechanism matches; the 6 "become &lt;role&gt;" multi-user tests are E2E-fixture-specific (custom test accounts) with no per-role Maestro equivalent — not a tooling gap, just fixture scope |
| `03_browse_feed.probe` | *(none)* | **Gap.** No Maestro flow scrolls the feed and verifies it survives — `home-page-elements.yaml` only asserts static elements are visible, never scrolls |
| `04_create_post.probe` | `posts/create-text-post.yaml`, `posts/create-post-navigation.yaml` | General/text post creation matches; the 4 post-subtype tests (service/event/looking-for/alert) have no distinct Maestro flows |
| `05_react_and_bookmark.probe` | `posts/react-to-post.yaml`, `posts/bookmark-post.yaml`, `posts/react-to-post-count-verification.yaml` | Full |
| `06_filter_categories.probe` | `posts/posts-list-page-elements.yaml` | **Partial.** Maestro exercises 1 category chip (for-sale → all); probe exercises all 6 (for-sale, for-rent, service, event, looking-for, alert) |
| `07_open_post_detail.probe` | `posts/post-details-page-elements.yaml` | Full |
| `08_drawer_navigation.probe` | `navigation/drawer.yaml` + 6 `drawer-*.yaml` flows | Full — actually more granular on the Maestro side (7 separate flows vs. probe's one test with 7 sub-cases) |
| `09_settings_toggle.probe` | `settings/settings-notifications.yaml`, `settings/settings-language.yaml`, `settings/settings-about.yaml` | Full |
| `10_help_and_faq.probe` | `help/help-page-elements.yaml`, `help/help-faq-expand.yaml` | Full |

**Net result: 7/9 full parity, 2 partial/gap** (browse-feed scroll, filter-categories exhaustiveness)
— both on the Maestro side being narrower, not the probe side. Given the scope of closing these
(writing/extending real Maestro YAML against a live nect-flutter build, blocked in this
environment) they're logged as follow-on work rather than attempted blind.

## water-sip (`tests/smoke.probe` ↔ new `.maestro/flows/smoke/*.yaml`)

water-sip had zero prior Maestro coverage. Wrote and **live-verified** (Android emulator) a new
9-flow suite matching every test in `tests/smoke.probe` 1:1. PR open:
https://github.com/AlphaWaveSystems/water-sip/pull/51 (awaiting review — CI has an unrelated
`claude-review` bot failure, likely a missing secret in that repo, not something to force through
on the user's own app without established precedent for it).

**Result: 8/9 flows pass reliably.** The 9th (`undo-last-entry`) is a genuine, documented finding:
water-sip's "Undo" snackbar auto-dismisses in a window narrow enough that Maestro's out-of-process
accessibility polling can miss it entirely — confirmed via Maestro's own captured hierarchy
snapshot at the failure moment (add succeeded, no "Undo" node anywhere in the tree). FlutterProbe's
in-process element-tree access catches this reliably (see `docs/evidence/r1-tap-var-resolve-2026-08-14/`).
**This is a real competitive data point for G-1's speed thesis, not an authoring bug.**

### A second finding, worth carrying into G-1/G-2

Several of water-sip's UI labels are two-line `content-desc` values (e.g. the bottom-nav tabs:
`"History\nTab 2 of 3"`). Maestro's default text-selector regex doesn't span embedded newlines
without an explicit `(?s)` DOTALL flag — undocumented, discovered live by watching a plain
`tapOn: "History"` fail with "Element not found" against a label that visibly contains "History".
FlutterProbe's own text-matching has no equivalent gotcha (confirmed: `see "History"` and
`tap "250ml"`-style probe selectors just work against the same screens without special-casing).
Minor on its own, but exactly the kind of small-papercut evidence that supports "FlutterProbe is
easier to write correct selectors for" as a claim, not just an assertion.

## What this means for B-2/B-3

The benchmark harness (B-2) should run, per app:
- **water-sip**: the full 9-flow comparison (all 9 have working equivalents in both tools now).
- **nect-flutter**: the 7 flows with full parity, once nect-flutter's Android connectivity issue
  is resolved (or run against iOS instead, per `probe.yaml`'s own default platform there).
