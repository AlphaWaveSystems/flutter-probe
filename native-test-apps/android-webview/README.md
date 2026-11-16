# android-webview: a WebView to run the WebView steps against

One Android activity with one WebView (debugging on) that shows `assets/page.html`. It is the target of probe's WebView steps
(`see`, `tap`, `type`, `wait until ... appears`, `store text of ... in webview`) and of the three opt-in real-device tests that
check them. It is plain Java built with the Android SDK command-line tools: no Gradle, no dependencies, no APK in the repository.

The app is **not** a ProbeAgent host. The WebView steps reach the page through the WebView's own DevTools socket (`adb forward`),
so the tests below drive the executor directly and need no agent.

## Build and install

```bash
native-test-apps/android-webview/build.sh        # -> native-test-apps/android-webview/out/probe-webview-fixture.apk
adb -s <serial> install -r native-test-apps/android-webview/out/probe-webview-fixture.apk
adb -s <serial> shell am start -n dev.probe.webviewfixture/.MainActivity
```

`build.sh` needs the Android SDK (`ANDROID_HOME`, or `~/Library/Android/sdk` or `~/Android/Sdk`) with build-tools and a platform of
API 26 or newer, and a JDK 11 or newer. It signs with a throwaway debug key it makes in `out/` on every build; nothing built is
committed (`out/` is ignored). Package: `dev.probe.webviewfixture`.

## Run the real-device tests

Each test needs the app started fresh (the first one taps a button, so a second run on the same page already finds "Message sent").
Use one device and name it with `-s`/the serial; the tests only talk to that device.

```bash
adb -s <serial> shell am force-stop dev.probe.webviewfixture && adb -s <serial> shell am start -n dev.probe.webviewfixture/.MainActivity
PROBE_WEBVIEW_SERIAL=<serial> PROBE_WEBVIEW_APP=dev.probe.webviewfixture \
  go test -count=1 -run TestWebViewStepsOnARealDevice -v ./internal/runner/
# the same with TestWebViewTypeKindsOnARealDevice and TestStoreTextOnARealDevice, restarting the app before each
```

## The page

| Id | What it is | Used for |
|---|---|---|
| (title `h1`) | "Getting Started" | `see "Getting Started" in webview` |
| `message` | text input | `type "hello" into css:"#message" in webview` |
| `contact` | button "Contact support" | `tap "Contact support" in webview` |
| `result` | empty until the button is tapped, then "Message sent" | `wait until "Message sent" appears in webview`, `see css:"#result" in webview` |
| `total` | "Total: 42.00   USD" (runs of spaces) | `store text of css:"#total" in webview as total` (whitespace collapses) |
| `pw` | "my-device-pass-77" | a value whose variable name looks secret |
| `hidden` | a, a zero-width space, b, a right-to-left override, c, a byte order mark, d | hidden characters are dropped; the page shows the byte order mark as a space, so the text reads "abc d" |
| `empty` | an empty paragraph 20 px high | an element with no text stores an empty string |
| `long` | 3000 characters | the 1000-character cap |
| `ok`, `ro`, `dis`, `fs`, `cb`, `dt`, `num`, `ta`, `ce`, `sel`, `btn` | a text input, a read-only input (value "locked"), a disabled input, an input in a disabled fieldset, a checkbox, a date, a number, a textarea, a content-editable div, a select and a button | what `type ... in webview` types into, and what it refuses with the reason |

There is no `#nope`: the first test checks that a selector that matches nothing is not seen.
