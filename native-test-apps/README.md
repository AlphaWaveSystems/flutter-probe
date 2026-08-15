# Native test apps — purpose-built targets for probe's native UI verbs

Two minimal, dependency-free native apps exposing an identical set of stable, test-oriented
elements (a tap counter, a live-echo text field, a toggle, and a hidden-until-revealed message):

| Element | Android resource-id | iOS accessibility id | Visible text |
|---|---|---|---|
| Title | `native_title` | `native_title` | "Native Test App" |
| Counter button | `native_button` | `native_button` | "Tap Me" |
| Counter | `native_counter` | `native_counter` | "Taps: 0" → "Taps: N" |
| Text field | `native_input` | `native_input` | hint "Enter text here" |
| Echo mirror | `native_echo` | `native_echo` | "Echo: (empty)" → "Echo: <typed>" |
| Switch | `native_switch` | `native_switch` | "Native Switch" |
| Reveal button | `native_message_button` | `native_message_button` | "Show Message" |
| Hidden message | `native_message` | `native_message` | "Message Revealed" — absent from the accessibility/uiautomator tree until revealed |

## android/ — live target for `tap native` / `see native` / `type native` (N-1)

Kotlin, classic XML Views (`android:id` values surface directly as uiautomator resource-ids),
zero external dependencies. Build: `cd android && JAVA_HOME=$(/usr/libexec/java_home -v 21)
./gradlew assembleDebug`.

**Run pattern** (`android/probe-tests/native_suite.probe`, verified 5/5 on a real emulator):
probe's session lives in any ProbeAgent host app (e.g. water-sip); the native verbs are CLI-side
and never touch the Dart agent. Foreground this app over the host before the run:

```
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n com.alphawavesystems.probenativetest/.MainActivity
probe test android/probe-tests/native_suite.probe --config <host probe.yaml> --device <serial>
```

**Real-world pitfall captured here**: on API 35, edge-to-edge is forced — without
`android:fitsSystemWindows="true"` on the root layout, top-of-screen elements report uiautomator
bounds whose center sits under the app bar, which silently eats `input tap` (probe's tap lands,
nothing happens). This app shipped with that bug, was diagnosed via probe itself, and carries the
fix — kept documented because any adopter's native screens can hit the same thing.

## ios/ — target for the N-2 WebDriverAgent bridging spike

SwiftUI, generated via `xcodegen` (`cd ios && xcodegen generate`, then build with
`xcodebuild -scheme ProbeNativeTest -sdk iphonesimulator -configuration Debug build
CODE_SIGNING_ALLOWED=NO`). iOS native verbs are not implemented yet — `tap native`/`type native`
error clearly by design, `see native` reports "not found" (see
`docs/proposals/n2-ios-native-ui-bridging.md`). This app exists so that spike has a controlled,
identifier-stable target from day one; `native_message`'s conditional rendering makes "element
appears in the tree" the observable behavior WDA matching needs.
