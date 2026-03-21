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
