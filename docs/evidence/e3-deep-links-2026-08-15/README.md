# E-3 evidence — deep links: `open link "url" in the app`

`open link "url" in the app` / `... into the app` — routes a URL through the OS's own intent/URL
handling (`adb shell am start -a android.intent.action.VIEW`, `xcrun simctl openurl`) instead of
the Dart agent's `url_launcher` (which always opens an external browser). Matches Maestro's
`openLink` with app-targeting. Competitive roadmap Phase 1, E-3.

The existing `open link "url"` (no suffix) is unchanged — still goes through
`client.OpenLink` / `url_launcher`, still opens an external browser. `DeepLink` is CLI-side only
(`adb`/`simctl`), so cloud mode (no `DeviceContext`) skips it with a warning rather than silently
falling back to the external-browser path, which would run the wrong behavior without telling the
test author.

## Automated regression coverage

`internal/parser/parser_test.go`: `TestParser_OpenLink` (extended to assert `DeepLink` defaults
`false` for the pre-existing form), `TestParser_OpenLink_IntoApp` (table test over `"in the app"`,
`"into the app"`, `"in app"`).

`internal/runner/retry_optional_test.go`: `TestRunStep_OpenLink_ExternalBrowser` (no suffix still
calls `client.OpenLink`, unchanged), `TestRunStep_OpenLink_DeepLink_CloudModeSkips` (cloud mode
skips gracefully and — critically — does *not* fall through to the external-browser path).

Full `go test ./...` passes with no regressions.

## Real-device evidence (water-sip)

water-sip has a genuine registered custom scheme (`watersip://`) on both platforms —
`CFBundleURLSchemes` in `ios/Runner/Info.plist` and `android:scheme="watersip"` in
`android/app/src/main/AndroidManifest.xml` — so the deep link actually round-trips through real OS
intent handling, not a scheme that's merely declared but unhandled.

### Android emulator

`e3_android_deep_link_evidence.probe`, 1 test, proof strategy: open an `https://` link *without*
`in the app` first (existing, unchanged behavior — external browser, backgrounding water-sip),
then use the new `in the app` form with `watersip://` and confirm water-sip is back in the
foreground. This proves the OS genuinely routed the second URL to the app rather than the app just
happening to still be there.

```
✓  open link in the app brings water-sip back to foreground from a browser (11.173s)
```

### iOS simulator

`e3_ios_deep_link_evidence.probe`, 1 test (simpler than the Android version — this build's agent
predates `probe.open_link`, the pre-existing external-browser RPC, which is unrelated to E-3's own
work; E-3's `DeepLink` path is entirely CLI-side and calls no Dart RPC at all). Restarts water-sip
first (app running, not in foreground), then opens `watersip://...` `in the app` and confirms the
app is showing a real screen afterward.

```
✓  open link in the app on iOS simulator lands back on a real screen (7.607s)
```

## Finding: `simctl openurl` does not cold-launch a terminated app (Android does)

While trying to strengthen the iOS evidence to match Android's rigor — proving the deep link can
bring the app up from a fully-terminated state, not just background-to-foreground — a real
platform asymmetry surfaced:

- **Android**: `adb shell am start -a android.intent.action.VIEW -d <url>` cold-launches the app
  from fully terminated with no prompt, confirmed by killing the app first
  (`am force-stop`) and then observing it start.
- **iOS Simulator**: with water-sip fully terminated (confirmed absent from
  `xcrun simctl spawn <udid> launchctl list`), `xcrun simctl openurl <udid> watersip://...`
  returns exit 0, but the app does **not** relaunch. A screenshot taken immediately after shows
  the Home Screen with a system confirmation dialog — **"Open in 'WaterSip'? Cancel / Open"** —
  sitting on top, waiting for a manual tap. See `ios_cold_launch_confirmation_dialog.png`.

This is a genuine iOS Simulator behavior, not a bug in this feature's Go code (`SimCtl.OpenURL`
just shells out to `simctl openurl` exactly as documented and returns whatever it returns) — the
OS itself gates cold-launch-via-custom-scheme behind a user-confirmation prompt that `simctl`
cannot dismiss non-interactively. `simctl` has no touch-injection primitive to tap "Open"
programmatically, and driving it via host-screen coordinate clicks (e.g. `cliclick` against the
Simulator.app window) was deliberately avoided here — it's fragile and risks clicking outside the
sandboxed simulator on the real screen.

**Practical implication for test authors:** on iOS, `open link "..." in the app` reliably brings
a *running* (foreground or backgrounded) app to the front, exactly as demonstrated above. It is
**not** currently a reliable way to cold-launch a fully-terminated app via deep link in a test —
put a `restart the app` (or equivalent warm-up) before it if the app might not already be running.
No such caveat applies on Android.
