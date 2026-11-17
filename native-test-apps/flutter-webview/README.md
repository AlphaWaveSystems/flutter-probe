# flutter-webview: a Flutter app with a WebView, for probe's WebView steps

A Flutter app (Android) that hosts the ProbeAgent and shows a `webview_flutter` page from its own assets, so ProbeScript can run the WebView steps (`see`, `don't see`, `tap`, `type`, `wait until ... appears`,
`store text of ...`, each with `in webview`) through `probe test`, next to the ordinary steps on the Flutter widgets around the page. The Android-only fixture in `../android-webview` cannot do that: it has no agent.

| On screen | Id | What it is for |
|---|---|---|
| status text | `page_status` | `Page loading`, then `Page loaded (N)` (N counts the loads): `wait until "Page loaded" appears` |
| button "Reload page" | `reload_page` | loads the page again, so its state starts over |
| text field + echo | `echo_input`, `echo_text` | type a value you stored from the page into the field; the echo (`Echo: ...`) shows exactly what it was |
| the WebView | | `assets/page.html`, the page of `../android-webview` (a test keeps the two files equal; its ids are in that README) |

## Build, install, run

```bash
cd native-test-apps/flutter-webview
flutter build apk --debug --dart-define=PROBE_AGENT=true
adb -s <serial> install -r build/app/outputs/flutter-apk/app-debug.apk
adb -s <serial> shell am start -n com.alphawavesystems.probe_flutter_webview/.MainActivity
# a probe.yaml next to the test with:  project: { app: com.alphawavesystems.probe_flutter_webview }
probe test probe-tests/webview.probe --device <serial> -y
```

`probe-tests/webview.probe` has 6 tests and restarts the app before each. `probe-tests/negative/webview.probe.expect-fail` holds the 9 variants that must fail (copy it to a `.probe` file to run it; its header says how). What each shows, and what it does not, is in `docs/compat/MATRIX.md` ("WebView content").

## Versions

`webview_flutter` and `webview_flutter_android` are pinned to exact versions in `pubspec.yaml`, and `pubspec.lock` is committed; a test (`scripts/webview_fixture_test.go`) keeps the pins and the lock equal. What was read in their release notes is in
`docs/compat/RELEASES.md`. `webview_flutter_android` 4.12 and later need Flutter 3.44 or newer. The app builds on Flutter 3.44.8 and 3.50.0.

## What is on purpose

- WebView debugging is turned on (`AndroidWebViewController.enableDebugging(true)`): the WebView steps reach the page through its DevTools socket, which exists only then. A real app that wants them has to do the same.
- The app is a fixture and is never shipped. It has no iOS project: the WebView steps are Android only.
