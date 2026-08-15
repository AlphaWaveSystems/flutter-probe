# Full-feature test campaign — every ProbeScript feature, Android + iOS, local AI

One suite exercising **every ProbeScript feature** against a real app (water-sip, freshly built
with agent 0.12.x), run on a fresh Android emulator (`pixel_7_e2e_secondary`) and a
freshly-created iOS 26.5 simulator, with the entire AI family running against a **local LM Studio
`google/gemma-4-31b`** (validated up front to return probe's exact JSON verdict shape, vision
included). Suite files in `suite/`; raw per-run JSON results alongside.

## Final scores (after the fix round, v0.12.1)

| Platform | Result | Notes |
|---|---|---|
| Android | **32/32** | 27/27 core+device+flow (kill-reopen verified separately post-fix), 5/5 AI, plus **5/5 native-verb suite** against the purpose-built Kotlin app (`native-test-apps/`, #241) |
| iOS | **31/32** | 26/27 core (grant-all fixed post-run, verified live), 4/5 AI — see the true positive below |

**The one "failure" worth celebrating**: `assert no visual defects with ai` on iOS failed
because Gemma correctly identified a **real water-sip layout defect** — the bottom card row is
clipped behind the tab bar on the iOS 26.5 simulator (`ios_ai_flagged_clipped_cards.png`, verdict:
"The buttons at the bottom of the 'Quick Add' section are partially cut off and overlapped by the
navigation bar"). The feature did exactly its job; the finding belongs to water-sip.

## Six real CLI bugs found and fixed (shipped as 0.12.1, PR #242)

1. **`toggle` was a double no-op** — agent-side `toggle` was `break;`, and `toggle #id` didn't
   even parse (dangling-token misparse). Now a full selector dispatched as a tap.
2. **Filler-word recipe names unreachable** — `recipe "add and verify"` never matched its own
   call site; normalization now applies to both sides.
3. **Android `set location` never worked** — emulator-console `emu` command run inside the device
   shell. Now `adb emu geo fix`.
4. **iOS permission verbs hung the session** — `simctl privacy grant` silently kills the app
   (confirmed via launchctl); all four permission verbs now relaunch + reconnect on iOS sims.
5. **`wait N seconds` was an agent RPC** — `kill the app` → `wait` burned the step timeout in
   doomed reconnects (through adb forward, dead ports dial as EOF, not ECONNREFUSED, so PT-18's
   relaunch heuristic never fired). Duration waits now sleep CLI-side.
6. **`close keyboard` printed "close the app"** (cosmetic).

Also fixed en route (#241): the native Kotlin fixture app initially shipped with an API-35
edge-to-edge bug — top-of-screen uiautomator bounds centered under the app bar, silently eating
taps — diagnosed *with probe itself* and kept documented in the fixture README as an
adopter-relevant pitfall.

## Honest suite-side corrections (mistakes that were mine, not probe's)

- `see exactly 1 "Quick Add"` — the text legitimately appears twice (header + nav label).
- A `retry` body that tapped-then-asserted without waiting was non-idempotent by construction.
- `wait for animations to end` can never settle on water-sip — it runs an infinite looping
  progress-ring animation, so "no scheduled frames" never becomes true. Working as documented;
  noted in the suite as a real caveat for apps with perpetual animations.
- `wait until "Undo" disappears` passes on Android but times out on iOS (snackbar text lingers in
  the iOS tree — same Navigator/offstage semantics gotcha as `don't see`). Positive assertions only.
- AI tests need `--timeout 300s` headroom for a 31B local model's cold start; warm inference is
  fast (the whole 5-test AI file passes in normal time once loaded).

## Coverage map

Core verbs (see/don't-see/see-exactly/tap/double-tap/waits), gestures (swipe/scroll),
toggle, navigation, conditionals + otherwise, repeat, retry, optional, store/log/variables,
recipes (args + nested + filler-word names), data-driven `with examples`, visual regression
(full-screen + element-scoped `of`), AI (`see with ai`, `assert no visual defects with ai`,
`read with ai into` + variable round-trip), add media, deep links (`open link in the app`),
external browser + `verify external browser opened`, clipboard round-trip, set location,
permissions (grant all), HTTP mock rule + real `call GET`, dump tree, save device logs,
kill/open, clear app data, restart, shake/pause/close-keyboard, native verbs
(`tap/see/type native`, Android, vs. the dedicated fixture app).

Not exercisable on this app (honestly declared): Flutter-side `type`/`clear` — water-sip has
zero text fields; typing coverage came via `type native` (Android). Biometrics and composite
multi-device tests were out of scope for this campaign.
