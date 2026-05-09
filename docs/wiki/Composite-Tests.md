# Composite Tests

Composite tests let a single `.probe` file coordinate steps across **multiple devices simultaneously**. They are designed for testing real-time features — chat, multiplayer, sync, push notifications, and anything else that requires more than one device acting in concert.

---

## Syntax

```
composite test "alice sends bob a message"
  devices
    A: iPhone 15 Simulator
    B: Pixel 9 Emulator

  A:
    open app
    tap "Login"
    type "alice@example.com" in "email"
    tap "Continue"

  B:
    open app
    tap "Login"
    type "bob@example.com" in "email"
    tap "Continue"

  sync "both logged in"

  A:
    tap "New Message"
    type "Hello Bob" in "compose"
    tap "Send"

  B:
    wait until "Hello Bob" appears
    see "Hello Bob"
```

### Keywords

| Keyword | Where | Purpose |
|---------|-------|---------|
| `composite test "name"` | top-level | Declares a composite test |
| `devices` | inside composite | Optional block declaring alias-to-device mappings |
| `A:` / `B:` / `Phone:` | inside composite | Steps tagged for that device alias |
| `sync "label"` | inside composite | Cross-device barrier |

### Device blocks

Steps indented under `A:` execute only on device `A`. Multiple device blocks for the same alias are fine — steps accumulate in order:

```
A:
  tap "step 1"
A:
  tap "step 2"
```

### Sync barriers

`sync "label"` is a cross-device rendezvous point. **All** devices in the test must reach that label before any of them proceed. Labels are strings — use descriptive names:

```
sync "both at home screen"
sync "message delivered"
sync "session ended"
```

The same label may not appear more than once in a test (each barrier is single-use per run).

---

## Execution model

FlutterProbe launches **one goroutine per device**. Goroutines execute their respective steps sequentially. At each `sync` barrier all goroutines block until the last one arrives, then all proceed together.

```
goroutine A: [tap, type, tap] ────────────────── sync ── [tap] ──▶
goroutine B: [tap, type, tap] ────────────────── sync ── [see] ──▶
                               ← concurrent →   barrier  ← concurrent →
```

### Failure semantics

If device A fails mid-test:

1. The **root-cause device** (A) records the error.
2. The **shared context** is cancelled immediately.
3. All **sync barriers** are aborted so no other goroutine hangs.
4. Other goroutines (B, C…) unblock from any barrier or step they are waiting at and receive a `context.Canceled` error.
5. The final result marks A as `FAIL` and B/C as `CANCELLED`.

The composite test is marked **FAIL** if any device fails (other than with `context.Canceled`).

---

## Configuring devices

Devices must be mapped to real serials / host:port at runtime. FlutterProbe resolves them in this order:

1. **CLI flags** (highest priority)
2. **`probe.yaml` composite.devices** section
3. Alias declared in the `devices` block (used only as a name hint, not for auto-resolution)

If any declared alias cannot be resolved, the test is **SKIPPED** (not failed).

### Connection spec formats

| Format | Example | When to use |
|--------|---------|-------------|
| `host:port/token` | `192.168.1.10:48686/abc123` | WiFi-connected physical device or cross-machine |
| iOS simulator UDID | `A1B2C3D4-E5F6-...` | Locally-attached iOS simulator |
| Android serial | `emulator-5554` | Locally-attached Android emulator |

### CLI flags

```bash
probe test tests/chat.probe \
  --composite-device "A=192.168.1.10:48686/my-token" \
  --composite-device "B=00008030-001A34E40258002E"
```

Repeat `--composite-device` once per alias. Each value is `ALIAS=spec`.

### probe.yaml

```yaml
composite:
  devices:
    A: "192.168.1.10:48686/my-token"
    B: "00008030-001A34E40258002E"
```

CLI flags override probe.yaml values for the same alias.

---

## Coexistence with regular tests

A `.probe` file may contain both `test` and `composite test` definitions. Regular tests run sequentially on the primary device; composite tests run multi-device with their own runner. They share the same file's recipes and `use` imports.

```
# tests/realtime.probe

test "single device smoke"
  open app
  see "Home"

composite test "push notification delivery"
  devices
    Sender: iPhone 15 Simulator
    Receiver: iPad Air Simulator
  Sender:
    tap "Send Notification"
  sync "sent"
  Receiver:
    wait until "New message" appears
    see "New message"
```

---

## Skipped tests

If no composite devices are configured (no CLI flags, no probe.yaml section), composite tests are reported as **SKIPPED** with a message:

```
  ⟳  push notification delivery (skipped: no composite devices configured — use --composite-device)
```

This lets you run the same file on a single-device CI pipeline without failures. Add the `--composite-device` flags on the multi-device pipeline.

---

## N-device tests

Composite tests support any number of devices. Add more aliases to the `devices` block and write corresponding step blocks:

```
composite test "three-way conference call"
  devices
    Alice: iPhone 15 Simulator
    Bob: Pixel 9 Emulator
    Carol: iPad Pro Simulator

  Alice:
    tap "Start Call"
  Bob:
    tap "Join Call"
  Carol:
    tap "Join Call"

  sync "all in call"

  Alice:
    tap "Mute"
  Bob:
    see "Alice (muted)"
  Carol:
    see "Alice (muted)"

  sync "verified mute"
```

Each sync barrier is initialized with **N** (the total device count), so all three goroutines must arrive before any proceeds.

---

## Tips

- **WiFi mode is recommended** for physical devices — no USB fragility, no iproxy tunnels.
- **Keep tests self-contained**: each composite test should log in and log out explicitly. There is no equivalent of `clear app data` across composite devices.
- **Name sync points descriptively** — `sync "both at chat screen"` is far easier to debug than `sync "s1"`.
- **Failure messages include the alias** — `[A] assertion failed: "Send" button not found` tells you exactly which device and which step failed.
