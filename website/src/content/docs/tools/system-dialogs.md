---
title: System Dialogs
description: Drive OS-level dialogs — permission alerts, the StoreKit "Sign in to Apple Account" sheet, Android permission dialogs — from ProbeScript or the command line.
---

Some UI is not part of your Flutter app: iOS permission alerts, the StoreKit **Sign in to Apple Account** sheet,
Android's permission dialogs. They are drawn by the operating system, so the on-device Dart agent can neither see
nor tap them. FlutterProbe drives them from outside the app:

| Platform | How | Needs |
|---|---|---|
| iOS simulator | a small XCUITest runner ("iOS driver") that reads SpringBoard's accessibility tree | macOS with Xcode |
| Android emulator / device | `uiautomator dump` + `adb input` | `adb`, and no other UI-automation tool running |

Physical iOS devices are not supported (an XCUITest runner there needs code signing). The feature is optional:
a normal test run never starts the driver.

## ProbeScript steps

```
tap "Allow" in system dialog
tap "OK" in system dialog "Apple Account"                       # optional title filter
type "$PROBE_SANDBOX_PASSWORD" into system field "Password"
see system dialog "Sign in to Apple Account"
don't see system dialog "Notifications"
wait for system dialog "Notifications" appears                  # or: disappears
dismiss system dialog
sign in sandbox tester
tap "Allow" in system dialog optional
```

- **Button and field matching** is case-insensitive, exact first and then "contains", and treats `’` and `'` as the
  same, so `tap "Don't Allow"` matches iOS's "Don’t Allow".
- **Title filter**: an optional second string narrows to a dialog whose text contains it. Without one, the first
  system dialog is used.
- **`dismiss system dialog`** taps the first of Cancel, Don't Allow, Not Now, Close, Dismiss, Later, No Thanks. It
  does nothing (and passes) when no dialog is showing.
- **`optional`** on any step turns "no dialog showed up" into a warning instead of a failure, which keeps flows
  idempotent.
- Failures say what *is* showing: `... (showing: "Allow Notifications?" [Allow | Don’t Allow])`.

## Secrets

`type "..." into system field "..."` takes the text from an environment variable (`$NAME` or `${NAME}`) or a
literal. Because a literal could be a password too, **everything typed into a system field is masked**: the step is
shown as `type "****" into system field "Password"` in output and reports, and the value is scrubbed from any error
text. A missing or empty variable fails the step by *name*, never by value. Values are sent only to the local driver
on loopback.

:::note
The failure screenshot is taken from the Flutter app, which never contains system-dialog fields. If you add your
own screenshots around a sign-in step, remember that a visible (non-secure) field shows its text.
:::

## StoreKit sandbox sign-in

A fresh simulator asks to sign in to the App Store the first time your app touches StoreKit. Provision a sandbox
tester once; the account lives in the simulator's data and survives app reinstalls.

```
sign in sandbox tester
```

It waits briefly (about 5 s) for the sheet, types `PROBE_SANDBOX_USER` and `PROBE_SANDBOX_PASSWORD`, taps OK and
waits for the sheet to close. **If no sheet appears it does nothing**, so it is safe at the start of every test. If
the sheet stays open, it fails with a hint to check the tester credentials.

The same thing, with no test file and no Flutter app (handy for provisioning a CI simulator):

```bash
export PROBE_SANDBOX_USER='tester@example.com'
export PROBE_SANDBOX_PASSWORD='...'            # from your secret store; never on the command line
probe system-dialog sign-in-sandbox --device <simulator-udid>
```

## Notification permission

iOS has no `simctl` service for notifications, so `allow permission "notifications"` and
`probe test --grant notifications` answer the system alert instead: the first taps **Allow** if the alert is up
within a few seconds (a no-op if the permission was already decided), the second keeps a watcher running that taps
Allow whenever the alert appears during the run. `deny permission "notifications"` taps **Don't Allow**.

## The `probe system-dialog` command

```bash
probe system-dialog list                       # titles, buttons and fields of what is showing
probe system-dialog see --title Notifications  # exit 0 if showing, 1 if not
probe system-dialog tap "Allow"
probe system-dialog wait --title Notifications --gone --timeout 10s
probe system-dialog dismiss
probe system-dialog type --field Password --env PROBE_SANDBOX_PASSWORD   # or --stdin
probe system-dialog sign-in-sandbox
```

`--device <udid-or-serial>` picks the target (default: the only booted/connected device). The value for `type` is
accepted from `--env NAME` or `--stdin` only, never as an argument, so it cannot show up in `ps` or shell history.

## The iOS driver

The runner is built in CI on every release and attached as `probe-ios-driver.zip`. The first use on a machine
downloads it into `~/.probe/ios-driver/<version>/`; you can do it ahead of time:

```bash
probe ios-driver install
probe ios-driver status --device <udid>
probe ios-driver stop --device <udid>     # normally not needed
```

`probe` starts it with `xcodebuild test-without-building` on the simulator (about 5 seconds, once per run), talks
to it over a loopback-only HTTP port derived from the simulator's UDID, and stops it when the run ends. If the CLI
dies, the runner stops itself after a safety timeout.

| Variable | Purpose |
|---|---|
| `PROBE_IOS_DRIVER_DIR` | use a runner build from this directory (e.g. one you built with `make ios-driver`) |
| `PROBE_HOME` | move `~/.probe` |
| `PROBE_DRIVER_APPS` | extra bundle ids whose UI counts as a system dialog (comma-separated) |
| `PROBE_ANDROID_DIALOG_PACKAGES` | extra Android packages to treat as system dialogs |

To build the runner yourself: `brew install xcodegen && make ios-driver` (produces `bin/probe-ios-driver.zip`; the
source is in `ios-driver/`).

## Troubleshooting

- **"no system dialog found"** — run `probe system-dialog list` while the dialog is up. If it is empty, the dialog
  belongs to a process the driver does not look at: find its bundle id and set `PROBE_DRIVER_APPS`.
- **"needs Xcode"** — install Xcode and run `sudo xcode-select -s /Applications/Xcode.app`.
- **Android: "uiautomator ... killed"** — only one UI-automation client can run per device. Stop Maestro's driver or
  another uiautomator session on that device.
- **Android typing** — `adb shell input text` types ASCII only. Spaces and shell characters (`& ; $ ' "`) in the
  value are handled; emoji and non-Latin text are not.
- **Cloud devices and physical iOS** — not supported; the steps fail with a message saying so.

## Compared with Maestro

Maestro drives the same dialogs through its own on-device driver. FlutterProbe's version is deliberately narrower:
dialogs only (not arbitrary native UI on iOS), simulators only on iOS, and an explicit secrets model. For Flutter
UI you keep using the normal ProbeScript steps, which are far faster than accessibility-tree driving.
