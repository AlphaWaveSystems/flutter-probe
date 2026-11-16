# `press <key>` on iOS (design note)

**Status:** Design note for review. No code, no behaviour change. It answers one question: what would it take for `press tab`, `press escape`, `press backspace`, `press space` and the four arrow keys to work on iOS, and what does XCUITest allow?

## Where it stands

- `press <key>` (PR #605) is **not** an operating-system key press. The Dart agent inside the app (`probe_agent/lib/src/executor.dart`, `_pressKey`) acts through Flutter's own machinery: `tab` and the arrows move the focus with the focus system (`nextFocus`, `focusInDirection`), `escape` unfocuses, `backspace` and `space` edit the focused field through its `EditableTextState` (the same entry point real text input uses, so `onChanged` runs). `press enter` is the same kind of thing (`performAction` on the field).
- On iOS the runner refuses it before sending anything (`internal/runner/executor.go`, the `VerbPressKey` case): *"press X is not supported on iOS yet: only press enter works there (not verified on a simulator)"*. The refusal is about **verification, not about a known incompatibility**: the Dart code does not branch on the platform, and `press enter` already runs through the same agent on iOS.
- The iOS driver (`ios-driver/`, an XCUITest runner with routes `/see /wait /tap /type /dismiss /dialogs /tree`) handles **system UI** only (permission dialogs, share sheets). It has no key route. Its `type` already uses `XCUIKeyboardKey.delete` inside a `typeText` string to clear a field.

So there are two separate questions: (A) does the existing in-app path already work on iOS, and (B) is there anything an in-app path cannot do that a native key press would.

## Option A: verify the in-app path on an iOS simulator, then lift the refusal (recommended first step)

What changes: nothing in the agent. One real run on a simulator, per key, against a fixture app with two text fields and a few focusable widgets, recording what each key does. Then the runner refusal is removed (or narrowed to the keys that did not pass), the agent's handshake keeps gating old agents, and the docs say "Android and iOS".

What to check on a simulator, per key:

| Key | Expected on iOS (same code as Android) | Risk to look for |
|---|---|---|
| `tab`, arrows | focus moves by Flutter's traversal | iOS apps often have no focus highlight; the step passes but nothing visible changes. The focus *does* change, which a following `type`/`press` can use. |
| `escape` | the field loses focus | the software keyboard may stay up: Flutter unfocusing closes the text input connection, which hides it, but this must be seen, not assumed |
| `backspace`, `space` | edits the focused field, `onChanged` runs | an IME composing region (marked text) is not handled by `_deleteBeforeCaret`; check with an accented input |
| `enter` | already works | none |

Cost: small. Risk: low. This does not need XCUITest at all.

## Option B: a native key route in the iOS driver (only if A shows a gap)

If a key cannot be reached in-app (a native text field inside a platform view, a `WKWebView` input, a system alert field), the iOS driver could press it with XCUITest. What XCUITest allows, as far as I know (**each statement to be confirmed on a simulator before any code is written**):

- `XCUIElement.typeText(_:)` types into the element that has keyboard focus, and the string may contain the special-key constants of `XCUIKeyboardKey` (`delete`, `return`, `escape`, `tab`, `space`, the four arrows, `home`, `end`, `pageUp`, `pageDown`, ...). The driver already relies on `delete` this way. The element must have keyboard focus, or the call fails/hangs.
- `XCUIElement.typeKey(_:modifierFlags:)` presses one `XCUIKeyboardKey` with optional modifiers (shift, control, option, command). It is documented for iOS 13.4 and later, and is meant for a hardware keyboard.
- **The hardware-keyboard setting matters on a simulator.** With "Connect Hardware Keyboard" on, the simulator takes key events from the host and the software keyboard may not show; with it off, key events may not reach the app the way a real keyboard's would. The two modes can behave differently for `tab` and the arrows, which iOS only turns into focus movement when the app handles keyboard navigation (full keyboard access, `UIFocusSystem`). A Flutter app reads them through its own key handling, not UIKit's.
- **No key press reaches a system-wide target.** `XCUIDevice.shared.press(_:)` takes buttons (home, and volume up/down on devices that support them), not keys.
- A physical device accepts the same calls; the simulator is where they were never checked here.

What B would add: a `POST /key` route with `{key}` limited to the same eight words (a closed set, as `pressKeys` is on the parser side), a `Driver.pressKey` that maps the word to an `XCUIKeyboardKey` (no free-text key names, no modifiers), the Dart agent staying the first choice and the driver the fallback for native fields, and a capability in the handshake (the way `press_key` is reported) so an older driver is refused clearly.

Cost: larger (Swift route, the runner choosing between two paths, a second real-device matrix). Risk: moderate. **Not recommended until A shows what is missing.**

## Rules that hold for either option

- No new key names and no modifiers: the eight words stay the whole vocabulary.
- A key with nothing to act on warns (as on Android), it does not pass silently.
- A real run per key on a named simulator is part of the PR, never a claim from reading the code.
- The refusal stays for any key not verified.

## Questions for the owner

1. Is Option A (one verification run, then lift the refusal) enough for now, with B only on demand?
2. Is a simulator run acceptable as the evidence, or should one physical iPhone also be used (the project's other iOS checks use simulators)?
3. Should `press` on a native text field (a platform view or a WebView input) be in scope at all, or stay "Flutter widgets only" as documented for Android?
