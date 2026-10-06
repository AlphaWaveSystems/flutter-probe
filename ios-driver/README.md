# ios-driver

The FlutterProbe **iOS system-dialog driver**: an XCUITest runner that lets `probe` see and tap OS-level dialogs on
the iOS Simulator (permission alerts, the StoreKit "Sign in to Apple Account" sheet, ...), which live outside the
Flutter app and are invisible to the on-device Dart agent.

It is optional. A normal `probe test` run never starts it; only system-dialog steps, `probe system-dialog` and
`--grant notifications` on iOS do.

## How it works

- `ProbeDriverHost` is an empty host app (UI-test bundles need one).
- `ProbeDriverUITests/ProbeDriverTests.testServe` never asserts anything: it starts a loopback-only HTTP server
  (`DriverServer.swift`) inside the test-runner process, the only process allowed to drive SpringBoard's accessibility
  tree, and keeps the test alive until it receives `POST /shutdown` (or a 3-hour safety timeout).
- `Driver.swift` finds dialogs (alerts, sheets, and the windows of system service apps) and exposes `/dialogs`, `/see`,
  `/tap`, `/type`, `/dismiss`, `/tree`. Text sent to `/type` is never echoed in a response.
- The CLI (`internal/sysdialog`) starts it with `xcodebuild test-without-building`, talks to it over
  `127.0.0.1:<port derived from the simulator UDID>`, and stops it when done.

## Build

Needs macOS, Xcode and [xcodegen](https://github.com/yonaskolb/XcodeGen):

```bash
brew install xcodegen
make ios-driver            # -> bin/probe-ios-driver.zip   (scripts/build-ios-driver.sh)
PROBE_IOS_DRIVER_DIR=<unzipped dir> probe system-dialog list --device <udid>
```

`project.yml` is the source of truth; `ProbeDriver.xcodeproj` is generated and not committed. CI builds the zip on
every release and attaches it; `probe ios-driver install` downloads it to `~/.probe/ios-driver/<version>/`.

## Self-test hooks (host app launch arguments)

| Argument | What it does |
|---|---|
| `-probeRequestNotifications` | requests notification permission, which raises the system alert |
| `-probeShowSignInAlert` | shows an alert shaped like the StoreKit sheet (account field, secure password field, Cancel/OK) and writes what was typed to `tmp/probe_signin.txt` |

Simulators only: a runner on a physical device needs code signing.
