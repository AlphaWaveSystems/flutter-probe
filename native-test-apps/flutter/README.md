# probe_flutter_fixture

Flutter twin of the native fixture apps in `../android` and `../ios` — see the parent
`native-test-apps/README.md` for the shared element table and cross-framework story.

## The widget family screens

One screen per widget family of the compatibility catalog (`docs/compat/MATRIX.md`), chosen at build time. Every element has a semantics identifier and a key of the same name (`wf_...`) and shows its state as text.

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_WIDGETS=selection   # or textinput, buttons, navigation, display, lists
probe test probe-tests/widgets/selection.probe --config probe.yaml --device <serial>
```

Families so far: `selection` (checkbox, switch, radio group, slider, Cupertino switch), `textinput` (plain, form, obscured, multi-line and Cupertino fields), `buttons` (elevated, filled, outlined, text, icon, floating action, Cupertino, with an icon, disabled, with a long press) `navigation` (an app bar with a title and a search action, a pushed route, `go back`) `display` (a plain `Text`, a `Text` cut with an ellipsis, `Text.rich` and `RichText`; an `Icon` and an `Image` with a semantic label, a `CircleAvatar`, a `FlutterLogo` that counts taps) and `lists` (a lazy `ListView.builder` of 200 rows with a page each for a `GridView.count`, a `CustomScrollView` with a `SliverList`, a `SingleChildScrollView` and tiles: `Card` with `ListTile`, `CupertinoListTile`, and a list inside a `Scrollbar`). The negative variants of a suite (every test must fail) are in `probe-tests/negative/`, named `.probe.expect-fail` so a directory run never picks them up; the header of each says how to run it. A screen is something to test against: it is not a claim that
FlutterProbe supports a widget, and the matrix cell for it changes only from a test that ran. On a shared emulator give the agent its own port, as for the other screens.

## The list screen (scroll checks)

The default app is the 8-element surface the native fixtures share, and it stays that way. The scroll checks in
`probe-tests/list/` run against a second screen, a full-height list (`#fixture_list`, rows `#fixture_row_N`) with no other
scrollable around it, whose offset is shown in the app bar. Build it with a flag and install it over the default build:

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_LIST=true
probe test probe-tests/list/list_scroll.probe --config probe.yaml --device <serial>
```

`probe-tests/list/slider_drag.probe` swipes across the slider above the list (`#fixture_slider`, its value shown as `Value N`, starting at 50) and checks the value moved.

The list has to stand alone. The agent's `scroll ... on <target>` moves the nearest scrollable above the target: inside the
default screen's scroll view that is the outer page, so the list's own offset stayed at 0.0 when the list was placed there.

## The crash screen (forced crash)

`probe-tests/crash/forced_crash.probe` runs against a third screen: one button, `#fixture_crash` ("Crash the app"), that aborts the
process from native code through `dart:ffi` (the C library's `abort()`, found in the process itself, so no plugin). The default
build and the list build do not have it. Build it with a flag and install it over the default build:

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_CRASH=true
probe test probe-tests/crash/forced_crash.probe --config probe.yaml --device <serial> --crash-evidence
```

The test fails on purpose: the tap ends the app, so the step that loses its connection to the agent fails. With `--crash-evidence`
(Android only) the failed test keeps why: one line for the test (`the app died during this test: native_crash (pid …): Fatal signal 6
(SIGABRT)`), a `crash` object in its JSON report, and the exit reason and the crash text under `reports/crash/`. Without the flag the
run reports only the lost connection (`connection closed`). Not in any CI run: it is run by hand, like the other scenarios here.

On a shared emulator give the agent its own port: `--dart-define=PROBE_PORT=<n>` in the build, and `agent: port: <n>` in
`probe.yaml`, the same number in both, since probe forwards that port to the device.

## The load screen (sustained-load reproduction, PT-25)

`probe-tests/load/sustained_load.probe` is the reproduction for the agent connection that drops (close 1006) when an app does sustained
background work. The load screen (`PROBE_FIXTURE_LOAD=true`, key `load_ticks`, title "Load running") runs work that never stops while it is up:

- `PROBE_FIXTURE_LOAD_MODE=ui` (the default): about 40 ms of every 50 ms of CPU work on the UI isolate, where the agent's socket is also served.
- `PROBE_FIXTURE_LOAD_MODE=isolate`: the same work on a second isolate, leaving the UI isolate free.

```bash
flutter build apk --debug --dart-define=PROBE_AGENT=true --dart-define=PROBE_FIXTURE_LOAD=true --dart-define=PROBE_FIXTURE_LOAD_MODE=ui
probe test probe-tests/load/sustained_load.probe --config probe.yaml --device <serial>
```

The test takes about five minutes. A reproduction is a `Connection lost` line in the run output with `close 1006` (or `unexpected EOF`)
within that time. Run it once per mode to tell whether the UI isolate has to be busy for the drop. This is a reproduction only: it does not change the agent.

## On a physical iPhone, over WiFi

The app passes a `bonsoir`-based `ProbeAdvertiser` (`lib/probe_advertiser.dart`) to the agent when it is built with
`--dart-define=PROBE_WIFI=true`, and `ios/Runner/Info.plist` has the `NSLocalNetworkUsageDescription` and `NSBonjourServices`
(`_flutterprobe._tcp`) entries the advertisement needs on iOS 14+. Build in profile mode, never release.

```bash
scripts/physical-ios-fixture.sh --ip <phone-ip>    # prints the exact steps for your phone; --check tests reachability
```

Signing the app is the owner's step (an Apple developer team in Xcode). Full conditions:
https://flutterprobe.dev/platform/ios/ ("Physical iPhone over WiFi").
