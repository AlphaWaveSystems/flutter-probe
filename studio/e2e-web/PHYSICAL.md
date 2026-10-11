# Studio with a physical phone: opt-in E2E and manual checklist

A person with a phone can verify the whole path (build, install, find, connect, see the screen, run a test) in about ten
minutes. Nothing here runs in CI: the automated half is gated by `STUDIO_E2E_PHYSICAL=1` and refuses to run when `CI` is set.

## What you need

- A Mac with Xcode 15 or later, Flutter, and the `probe` CLI; an iPhone paired with it, Developer Mode on, on the **same
  network** as the Mac (Wi-Fi or Ethernet on the same router, no client isolation).
- Your Apple developer team (signing is your step; the script only prints it).

## 1. Put the fixture on the phone (about 5 minutes)

```bash
scripts/physical-ios-fixture.sh --ip <phone-ip>      # prints the exact commands for the connected iPhone
```

Follow its steps 1 and 2 (sign in Xcode once, then `flutter run --profile -d <id> --dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`).
Keep the app in the foreground with the screen unlocked, allow the local network prompt, and copy the value after
`PROBE_TOKEN=` from the console.

| Check | Expected |
|---|---|
| `scripts/physical-ios-fixture.sh --ip <phone-ip> --check` | `ping` answers, "the agent answers on <ip>:48686" with `{"agent":"flutter_probe",...}`, and the Bonjour service `_flutterprobe._tcp` is advertised (if it is not, "Connect manually" still works) |

## 2. The automated half (about 3 minutes)

```bash
scripts/studio-e2e-physical.sh --host <phone-ip> --token <token>
```

It builds Studio, serves it in a browser (no native window) and runs four tests against the phone:

| # | Expected |
|---|---|
| 30 | **Test connection** says "Agent reachable at ... and the token is accepted", and Studio stays disconnected |
| 31 | **Connect** shows the phone's live screen and the widget tree contains `Native Test App` |
| 32 | A one-line probe file (`see "Native Test App"`) runs through Studio against the phone and passes |
| 33 | A wrong token is refused with a message that says what to check and does not echo the token |

Reports: `reports/studio-e2e-web/html/index.html`. With a one-line failure, the message in Studio (or the test's `error-context.md`)
names the thing to check.

## 3. The manual half in the real Studio window (about 2 minutes)

Start Studio normally, then:

| Step | Expected |
|---|---|
| Click the picker's refresh | The phone is listed with its name and model, tagged `physical` and `USB` or `WiFi only`; selecting it shows a line saying the app must be running with the agent |
| Click the 📡 button, **Connect manually**, enter host, port, token, **Test connection** | A green message: "Agent reachable ..." |
| **Connect** | The status shows the phone; the device pane shows its screen and updates when you touch the phone |
| Open a `.probe` file and **Run** | Step rows turn green; results list passes |
| Put the app in the background, then **Test connection** | A clear message that the phone did not answer, listing what to check |
| Enter a public address such as `8.8.8.8` | Refused: Studio only dials local addresses unless started with `PROBE_STUDIO_ALLOW_PUBLIC_TARGETS=1` |

Anything that differs from the "Expected" column is a bug: file it with the message text (never the token).
