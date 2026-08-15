# Getting probe's hands on native UI — iOS

**Status:** Proposal — not yet implemented
**Origin:** competitive roadmap Phase 2, N-2 (deferred by
`docs/proposals/pt13-native-ui-bridging.md`, which shipped Android via
`uiautomator` — see N-1, merged in #232 — and explicitly punted iOS to its
own proposal, this one)
**Touches:** `internal/device`, `internal/runner`, ProbeScript grammar
(reuses N-1's existing `tap native` / `see native` / `type native` syntax —
no new DSL surface)

## TL;DR

- **Ask:** the same thing PT-13 asked for on Android — `tap native "…"` /
  `see native "…"` / `type native "…"` reaching pickers and share sheets —
  now for iOS.
- **Finding:** PT-13's iOS cost estimate ("XCTest target + IPC... Xcode
  project change" for every adopter) was the right shape but the wrong
  granularity. The real integration point is **WebDriverAgent (WDA)** —
  Facebook's XCTest-based automation server, the same one Appium has driven
  iOS through since ~2016 — and it is a *separate* app, built and signed
  once, not a change to every adopter's own Xcode project. On the
  Simulator specifically, it needs no code-signing from the adopter at all.
- **Recommendation:** ship it, Simulator-first, using WDA as the automation
  backend — reusing N-1's exact verb surface so ProbeScript itself doesn't
  change at all, only what runs underneath `tap native` on iOS. Defer
  physical-device support (WDA's provisioning-profile requirement there is
  real, and matches probe's existing physical-vs-simulator cost asymmetry
  elsewhere — see `AddMedia`, `OpenDeepLink`, `SetLocation`, all of which
  already have narrower physical-device support than simulator).

## The concrete justifying flow

nect-flutter's post-creation screen has a real, unavoidable native-UI gap:
an "Add Images" button (`identifier: 'post_form_image_button'`,
`lib/features/posts/presentation/pages/post_form_page.dart:561`) calls
`image_picker`'s `pickMultiImage()`
(`post_form_page.dart:848`). On iOS this launches `PHPickerViewController`
— Apple's system photo picker, declared via `NSPhotoLibraryUsageDescription`
in `ios/Runner/Info.plist`. Since iOS 14, `PHPickerViewController` runs in
a **separate, out-of-process extension** — not even part of the host app's
own view hierarchy or process. It has no Flutter involvement whatsoever and
no OS-level bypass (unlike permission dialogs, already solved). A test
covering nect-flutter's actual post-creation flow cannot proceed past this
button today.

This mirrors N-1's Android evidence almost exactly: same DSL gap, same
"real feature is blocked mid-flow" shape, different platform underneath.

## Why this is harder than Android, and by how much

Android ships `uiautomator` as a standing OS service on every device and
emulator — N-1 called it directly, zero new dependencies, zero new
processes. iOS has no equivalent system service. `xcrun simctl` — the tool
every other iOS operation in this codebase already shells out to — has no
element-inspection or tap-by-accessibility-id subcommand at all; it can
boot, install, launch, set location, open URLs, and add media, but it
cannot look at or drive arbitrary on-screen UI.

The one real path to iOS UI automation without Apple's official native
inspector is **XCUITest**, and the one real way to *drive* XCUITest from an
external process (rather than compiling assertions into the app itself) is
**WebDriverAgent**: a standalone XCTest test target — already built and
open-sourced, no code to write — that, once launched on a simulator or
device via `xcodebuild test-without-building`, starts an HTTP server
(conventionally port 8100) implementing a WebDriver-like API: find element
by accessibility id/label/type, get its frame, tap it, type into it. This
is the exact mechanism Appium's iOS driver has used for essentially the
entire time Appium has supported iOS — it is mature, well-documented, and
not something this proposal would be inventing.

The critical distinction PT-13 didn't have visibility into: **WDA is not
part of the app under test's Xcode project.** It's a separate app,
installed and launched independently, that uses Apple's accessibility
APIs to inspect and drive *whatever else is running* on the same
simulator/device — the same relationship `uiautomator` has to the apps it
drives on Android. Building and signing WDA itself is a one-time cost this
repo would absorb (vendoring a prebuilt WDA `.app`/`.ipa`, or building it
from source as part of CI), not a per-adopter Xcode change.

| | Android (`uiautomator`, N-1) | iOS (WDA, this proposal) |
|---|---|---|
| Standing OS service | Yes, on every device/emulator | No — WDA is a separate app probe installs |
| Per-adopter Xcode change | None | None (Simulator) |
| New dependency for probe itself | None (`adb` already required) | WDA build/vendor + a running HTTP client |
| Signing required | No | **Simulator: no. Physical device: yes** — provisioning profile |
| Maturity of the mechanism | Google's own tool | Same one Appium has used for iOS since ~2016 |

The physical-device row is the one real asymmetry worth taking seriously.
WDA on a physical device needs to be code-signed with a valid development
team, the same class of requirement Xcode already imposes on *any* app
running on a physical iPhone — not novel to this proposal, but real
adopter-side setup that doesn't exist on Simulator. This is the same shape
of cost this codebase already accepts elsewhere: `AddMedia`, `OpenDeepLink`,
and `SetLocation` are all already narrower on physical iOS than on
Simulator (see `docs/evidence/e3-deep-links-2026-08-15/`,
`docs/evidence/e4-add-media-2026-08-15/` for two recent, concrete
instances of that exact pattern). Scoping this proposal to Simulator first
follows the precedent already set, rather than inventing a new one.

## What iOS support would look like

No new ProbeScript syntax. N-1 already shipped the verb surface this needs:

```
test "add images to a post"
  tap "Add Images"
  wait 1 second
  tap native "Camera Roll"
  tap native "IMG_0001"
  see "1 photo selected"
```

Same file, same flow, same verbs as the Android example in PT-13 and the
real N-1 evidence — the only thing that changes is what `DeviceContext`
does underneath `tap native` when `dc.Platform == device.PlatformIOS`,
mirroring how N-1's `TapNative`/`SeeNative`/`TypeNative` already branch on
platform and currently just error out (`tap`/`type`) or report "not found"
(`see`) for iOS. This proposal fills in that branch instead of leaving it
unimplemented.

**Implementation shape:**
- A `WDAClient` (new, `internal/device/wda.go` or similar) — a thin HTTP
  client against WDA's REST API: `POST /session` (start), a element-lookup
  endpoint by accessibility label/type, `POST /wda/tap`, `POST
  /wda/keys` (type). Same shape as `internal/probelink`'s existing
  JSON-over-HTTP/WS client, not a new architectural pattern for this repo.
- `Manager` gains a `WDA()` accessor alongside the existing `ADB()` /
  `SimCtl()`, and `probe.yaml`/CLI flags gain a way to point at a prebuilt
  WDA bundle (or a documented default install path this repo manages).
- `DeviceContext.TapNative`/`SeeNative`/`TypeNative` (in
  `internal/runner/device_context.go`) gain an iOS branch that calls
  through `WDAClient` instead of erroring/reporting-not-found — the
  Android branch (`ADB()` + `uiautomator`) is untouched.
- Lifecycle: probe launches WDA against the target simulator once per test
  run (or reuses an already-running instance — WDA's own startup is the
  slow part, on the order of several seconds, so a per-step launch would be
  a real regression against N-1's Android latency), and tears it down at
  run end.

## Options considered

### A — Compile a dedicated XCTest target into each adopter's own app

**Not now.** This is what PT-13's original framing assumed the iOS cost
would be, and it would genuinely have been the wrong shape: real
Xcode-project surgery for every adopter, before a single test could drive
a single picker. WDA avoids this entirely by being a separate app instead
of an addition to the app under test.

### B — WebDriverAgent, Simulator-first

**Recommended.** Closes the same gap N-1 closed on Android — pickers and
share sheets — for a cost that, on Simulator, is genuinely comparable to
what N-1 already paid: no per-adopter project change, reusing an existing,
mature, widely-deployed mechanism rather than inventing a new one.
Physical-device signing is real but scoped out of v1, matching this
codebase's existing physical-vs-simulator cost pattern.

### C — Raw coordinate taps against a simulator screenshot

**Not now**, for the same reason PT-13 rejected this on Android:
probe hasn't compromised on selector-based (not coordinate-based)
targeting anywhere else, and this would be the first exception. Also
concretely worse on iOS than it would have been on Android, since there's
no `uiautomator`-equivalent bounds data to fall back on if a coordinate
approach ever needed a sanity check — it would be coordinates and nothing
else.

### D — Wait for Apple to ship an official, external UI-automation API

**Not now.** No public indication this is coming, no timeline to plan
against. WDA is the mechanism the broader ecosystem (Appium, and by
extension a large fraction of production iOS test automation) has already
converged on in its absence.

## Recommended next step

Before writing `WDAClient` against production code: a scoped spike to
validate the specific claims above against this repo's actual toolchain —
building WDA from source (or vendoring a release build), launching it
against a booted simulator via `xcodebuild test-without-building`, and
confirming its HTTP API can find and tap an element in a real app (ideally
nect-flutter's own `PHPickerViewController` flow, to close the loop on the
concrete justifying case this proposal opened with). That spike is what
should turn this proposal's cost estimates from "well-documented elsewhere
in the ecosystem" into "verified against this codebase," the same bar N-1's
real-device evidence held Android to.
