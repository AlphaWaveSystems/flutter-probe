# N-1 evidence — native UI bridging on Android (`tap native` / `see native` / `type native`)

Reaches outside the Flutter widget tree into native (OS-owned) UI — pickers, share sheets, and
anything else the Dart agent can never see — via `uiautomator dump` + `input tap`/`input text`,
dispatched through `DeviceContext` exactly like `AllowPermission`/`SetLocation` already are.
Android only, per `docs/proposals/pt13-native-ui-bridging.md`'s Option C recommendation (iOS has
no `uiautomator` equivalent — that's N-2's own proposal). Competitive roadmap Phase 2, N-1.

## Automated regression coverage

`internal/device/uiautomator_test.go`: `TestFindNativeElement_MatchesByText`,
`TestFindNativeElement_MatchesByResourceID` (icon-only buttons often have no visible text — only
a resource-id to match against), `TestFindNativeElement_CaseInsensitive`,
`TestFindNativeElement_NoMatchReturnsFoundFalse`, `TestFindNativeElement_InvalidXMLErrors`,
`TestFindNativeElement_MalformedBoundsErrors` — all against a real, structurally accurate
uiautomator XML fixture (`sampleDump`), not a synthetic shortcut.

`internal/parser/parser_test.go`: `TestParser_TapNative`, `TestParser_TapNotNative` (regression
guard: plain `tap "X"` must still route to the ordinary Flutter-agent verb),
`TestParser_TypeNative`, `TestParser_TypeNotNative`, `TestParser_SeeNative` (covers both
`see native` and `don't see native`), `TestParser_SeeNotNative`.

`internal/runner/retry_optional_test.go`: `TestRunStep_TapNative_CloudModeSkips`,
`TestRunStep_TypeNative_CloudModeSkips`, `TestRunStep_SeeNative_CloudModeSkips` — all CLI-side
verbs (uiautomator via adb, no Dart RPC) skip gracefully in cloud mode instead of panicking on a
nil `DeviceContext`.

Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip, Android emulator)

Neither water-sip nor nect-flutter has a share-sheet or system-picker flow of their own reachable
without deeper app exploration, so the evidence targets two Android-provided native UI surfaces
directly instead — the system Share sheet and Settings' search screen. Both are genuine,
unmodified OS UI with zero Flutter involvement, which is exactly the surface this feature targets;
which app happens to be in the foreground when a native UI is triggered is incidental to what's
being proven.

### `see native` / `tap native` — the Android Share sheet

Triggered via `adb shell am start -a android.intent.action.SEND` (an external, real
`ACTION_SEND` intent — the same mechanism any app's "Share" button fires) while water-sip's
ProbeAgent stayed connected in the background.

```
✓  see native and tap native locate real elements in the Android share sheet (10.202s)
```

`n1_share_sheet_evidence.probe`: `see native "Chrome"`, `see native "Gmail"`, and a negative case
`don't see native "Nonexistent App XYZ 12345"` all passed, then `tap native "Bluetooth"` — the
result, captured via an independent raw `adb exec-out screencap` (not `take a screenshot`, see
below), shows the **Bluetooth row visibly highlighted/selected** (grey background, distinct from
the other rows), confirming the tap landed precisely on the intended element, not just somewhere
plausible. See `n1_share_sheet_before_tap.png` / `n1_after_tap_native_bluetooth.png`.

### `type native` — Settings' search field

`n1_type_native_evidence.probe`: `tap native "Search settings"` (navigates from Settings' home
screen to a dedicated search screen — a real Activity transition), `wait 2 seconds`, then
`type native "wifi" into "Search settings"`.

```
✓  type native focuses and types into a real native text field (8.724s)
```

Independently verified via raw screencap: the field shows **"wifi"** with real, live search
results (Wi-Fi, Wi-Fi hotspot, Wi-Fi Direct, Wi-Fi calling, Wi-Fi scanning, Wi-Fi control, Wi-Fi
certificate, Wi-Fi MAC address) — not a static screenshot, an actual query that ran against
Settings' real search index. See `n1_after_type_native_wifi.png`.

## Bugs found and fixed along the way

**`type native` lost keystrokes without a settle delay.** The first evidence attempt tapped the
search field, then immediately fired `input text "wifi"` — the tap visibly focused the field
(cursor appeared) but the typed text never arrived; the field stayed empty. Independently
reproduced with raw `adb shell input tap` + `input text` (no delay: same failure; with a manual
`sleep 1` in between: succeeded). The IME hadn't finished attaching by the time the text arrived.
Fixed in `DeviceContext.TypeNative` by adding a 500ms settle delay between the focus tap and
`input text` — the same default this codebase already uses for other tap-then-settle sequences
(`restartDelay`, `restart_delay: 500ms` in `probe.yaml`).

**`take a screenshot` (the Dart-agent verb) doesn't work while native UI covers the app.** The
very first evidence run used the existing `take a screenshot` step after `tap native`, expecting
it to work the same way it does everywhere else in this test suite. It didn't — the saved file was
corrupted (contained an error string, not PNG bytes). This makes sense once native UI is in the
foreground: `take a screenshot` can only capture Flutter's own render tree, and there's nothing
Flutter-rendered on screen at that point — the native UI is a separate Activity drawn entirely
outside Flutter's surface. **Not a bug** — a real, documented limitation worth calling out for
future test authors: to visually verify native UI state, use `adb exec-out screencap` (or
equivalent) externally, not `take a screenshot`, exactly as this evidence run did from that point
on.

## Known limitation (unchanged from the PT-13 proposal, restated for clarity)

Android only. iOS has no `uiautomator` equivalent — `tap native`/`see native`/`type native` are
Android-only verbs; `tap`/`type native` return a clear error on iOS rather than silently no-op'ing
(a native-UI tap is typically load-bearing mid-flow, so failing loudly beats a confusing failure
several steps later), while `see native` returns "not found" (the honest answer, and lets
`don't see native "..."` pass correctly there). iOS native bridging is N-2's own proposal.
