---
title: Recording
description: Record user interactions on a device and generate ProbeScript automatically.
---

FlutterProbe can generate `.probe` files by recording your interactions on a real device or simulator. This is useful for creating initial test drafts without writing ProbeScript manually.

## Usage

```bash
# Start recording (interact with your app, then Ctrl+C to stop)
probe record --device <UDID-or-serial> --output tests/my_flow.probe

# Record with a timeout (auto-stops after 60s)
probe record --timeout 60s -o tests/my_flow.probe
```

## How It Works

1. The CLI sends a `probe.start_recording` RPC call to the Dart agent
2. The agent installs a global pointer route via `GestureBinding.instance.pointerRouter.addGlobalRoute()`
3. Each touch event is captured, classified, and streamed back to the CLI as JSON-RPC notifications

### Event Classification

| Interaction | Detection |
|------------|-----------|
| **Tap** | Pointer down + up with displacement < 20px |
| **Long press** | Pointer down + up with displacement < 20px and hold > threshold |
| **Swipe** | Pointer down + up with displacement >= 20px, direction detected |
| **Text input** | Controller listener on `EditableText` elements, debounced at 300ms |

### Widget Identification

For each touch event, the recorder:
1. Hit-tests at the touch position
2. Walks hit test entries looking for user-meaningful selectors
3. Prefers: `ValueKey` > Text content > Semantics label > widget type
4. Skips framework internals like `NotificationListener`, `Padding`, `Container`

### Text in recorded steps

`probe record` and Studio's recorder write the steps from the same code, so the same event gives the same step in both. Text is written as the ProbeScript lexer reads it: only `\"`, `\\`, newline and tab are escaped, and everything else (a no-break space, a soft hyphen, a zero-width or bidirectional character, emoji, any script) is written as it is, so it matches when the test runs. A line break in a label becomes a space, and spaces around the text are trimmed. A character a script line cannot hold (a control character, a line separator, text that is not valid UTF-8) is not written wrong: a lookup that has one falls back to the next one (the flat text or id of the event); when none is left, a tap, long press or see becomes a comment (`# could not record a selector for tap: add the step by hand.`), never a step that cannot match, and typed text becomes a comment that asks you to add the step by hand. The runner expands `${NAME}` and `<name>` placeholders in a step's text, and there is no way to escape them, so a step whose recorded text contains one is followed by a comment that says so.

### Wait Step Insertion

When the gap between two actions exceeds 2 seconds, a `wait N seconds` step is automatically inserted in the generated script.

## Real-Time Feedback

As you interact with the app, the CLI prints each detected event:

```
  ● Recording on A1B2C3D4... — interact with your app
  ✓ tap "Sign In"
  ✓ type "user@test.com" into "Email"
  ✓ swipe up
  ✓ tap "Submit"
  ✓ Recorded 4 events → tests/my_flow.probe
```

## Platform Differences

- **iOS**: Uses `ReadTokenIOS()` directly — no port forwarding needed (simulator shares host loopback)
- **Android**: Uses `ForwardPort()` + `ReadToken()` via ADB

## Limitations

- Icon-only buttons may resolve to framework widget types instead of meaningful selectors — manual cleanup may be needed
- Complex gestures (pinch, rotate) are not recorded
- Implicit scrolling from list view interactions may not be captured accurately

## Tips

- Use `ValueKey` on important widgets in your app to get cleaner recorded selectors
- Record short flows and combine them into recipes
- Treat recorded output as a starting point — review and refine the generated ProbeScript
