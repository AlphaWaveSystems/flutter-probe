# Getting probe's hands on native UI

**Status:** Proposal — not yet implemented
**Origin:** `IMPROVEMENT_TASKS.md`, PT-13
**Touches:** `internal/device`, `internal/runner`, ProbeScript grammar

Pickers, share sheets, and one or two permission prompts sit outside the
Flutter widget tree entirely — invisible to every verb probe has today. This
proposes how far to reach outside that tree, and where to draw the line.

## TL;DR

- **Ask:** Let ProbeScript reach into native (non-Flutter) UI — pickers,
  share sheets, unbypassable dialogs.
- **Finding:** The gap is narrower than reported: permission dialogs are
  already solved by an OS-level bypass. What's actually missing is pickers
  and share sheets specifically.
- **Recommendation:** Ship an Android path now — it needs no new
  dependencies and reuses probe's existing device-command architecture.
  Treat iOS as a separate, larger proposal once there's a concrete flow to
  justify it.

## What's actually missing

The original report frames this as one gap — "native UI is invisible" —
covering pickers, share sheets, and permission dialogs together. Reading
through how permissions are handled today changes the shape of the problem.

| Surface | Today | Status |
|---|---|---|
| Permission grant/deny | `allow permission "camera"` calls `pm grant` / `simctl privacy grant` directly — the dialog never appears | Solved |
| iOS notification prompt | No OS-level bypass exists for this one prompt; apps work around it by checking `PROBE_AGENT` and skipping the request | App-side workaround |
| Image / file pickers | Fully native, no bypass, no selector reaches it | Gap |
| Share sheets | Fully native, no bypass, no selector reaches it | Gap |
| Screenshot / video of native UI | `take screenshot` and recording already capture the full physical screen, pickers included | Already works |

**Scope correction:** the real gap is pickers and share sheets — surfaces
with no OS-level bypass at all. Permission dialogs are already a solved
problem; this proposal doesn't need to touch that code path.

## What we already have to build on

probe's Go CLI already shells out to `simctl` and `adb` for a set of
device-level operations that never touch the Dart agent — installing,
launching, force-stopping, granting permissions, clearing app data. Each of
those is dispatched through `DeviceContext`, a path that sits entirely
outside the Flutter widget-tree / WebSocket round-trip. A native-UI verb is
architecturally the same kind of thing: it wants to run before or after the
Flutter app has focus, doesn't involve the Dart agent, and already has a
home to plug into.

That matters because it means this doesn't need the "bridging mode with
handoff" the original report imagined — no session state to hand back and
forth between two drivers. A native-UI verb is just another command
dispatched through `DeviceContext`, sitting in the same test file as
ordinary Flutter verbs:

```
# tap "Upload Photo" runs through the Flutter agent, as usual —
# tap native "..." runs through DeviceContext instead, no agent involved
tap "Upload Photo"
wait 1 seconds
tap native "Camera Roll"
tap native "IMG_0001"
```

No mode switch, no state handoff — just a second verb namespace dispatched
through the existing device-command path.

## Two platforms, two very different costs

Android and iOS diverge sharply here, and the proposal has to follow that
divergence rather than design one abstraction that fits neither well.

### Android — low cost

Every Android device and emulator already ships `uiautomator` — no install
step, no compiled test target. `adb shell uiautomator dump` returns the
current native UI hierarchy as XML (text, resource-id, bounds); `adb shell
input tap x y` drives it. Both are plain shell commands, the same shape as
everything already in `internal/device/adb.go`.

- New dependencies: **none**
- New infra for adopters: **none**

### iOS — high cost

There's no simulator-side equivalent. `simctl` has no element-inspection or
tap-by-accessibility-id subcommand — the real path is XCUITest, which means
a compiled UI test target added to the app's Xcode project, launched via
`xcodebuild test-without-building`, talking back to the CLI over a local
socket or file for the one interaction, then torn down. That's real
Xcode-project surgery for every adopter, not a probe-side change alone.

- New dependencies: **XCTest target + IPC**
- New infra for adopters: **Xcode project change**

## Options considered

### A — Full XCUITest / UiAutomator bridging on both platforms at once

**Not now.** The original report's own suggested fix. Matches the shape of
what Maestro and Appium already do, but pays iOS's full integration cost
before a single test can drive a single picker anywhere — including on
Android, where none of that cost is actually necessary. Symmetric scope,
asymmetric platforms; the wrong shape for a first release.

### B — Raw coordinate tap, no element lookup

**Not now.** `tap native at 200, 480` against a screenshot a test author
eyeballed by hand. Cheapest possible implementation on both platforms, and
brittle in exactly the way probe's whole design has avoided so far — a
layout shift or OS version bump silently breaks every coordinate.
Selector-based, not coordinate-based, is the one design principle probe
hasn't compromised on anywhere else; this would be the first exception.

### C — Android first, via `uiautomator` — iOS deferred to its own proposal

**Recommended.** Ships real selector-based native-UI support (`tap native
"text or resource-id"`, `see native "..."`) on the platform where it costs
nothing beyond code already written for this repo's existing patterns. iOS
gets the honest treatment it needs — its own RFC, once there's a concrete
adopter flow to size the Xcode-integration cost against, rather than a
rushed implementation forcing parity for its own sake.

## What Android support would look like

```
test "share a photo from the gallery"
  tap "Upload Photo"
  wait until "Choose from Gallery" appears
  tap native "Choose from Gallery"
  wait until "IMG_0001.jpg" appears
  tap native "IMG_0001.jpg"
  see "Photo uploaded"
```

Same file, same flow — `native` only marks which verbs cross into
OS-owned UI.

**Implementation shape:** a new `runner.VerbTapNative` / `runner.VerbSeeNative`
pair, dispatched through `DeviceContext` exactly like `AllowPermission` is
today. `tap native "X"` runs `uiautomator dump`, parses the XML for a node
whose `text` or `resource-id` contains `X`, computes the center of its
`bounds`, and issues `input tap`. No new package, no new binary to ship —
`adb` is already a hard dependency.

## Recommendation

Build Option C. It closes the highest-value part of the gap — pickers and
share sheets on Android — for the cost of two new verbs over commands the
platform already ships. iOS stays honestly unsolved rather than solved
badly; when a real flow demands it, that's its own proposal, sized against
the Xcode-integration cost this one deliberately didn't pay.
