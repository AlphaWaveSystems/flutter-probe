# 60-second demo script

Goal: show a plain-English test running against a real Flutter app, from nothing to a green result. Record the terminal and the simulator side by side. Everything shown is in the released CLI.

Before recording: a Flutter app with `flutter_probe_agent` added, running on a booted simulator or emulator with `--dart-define=PROBE_AGENT=true`, and a login screen with the texts used below.

| Time | Screen | Say | Do |
|---|---|---|---|
| 0:00 | Title card | "Flutter end-to-end tests, written in plain English." | none |
| 0:05 | Terminal | "Install and scaffold in two commands." | `brew install probe` (already installed: show `probe version`), then `probe init` |
| 0:15 | Editor | "A test reads like a manual test case." | Show `tests/login.probe` (below) |
| 0:25 | Terminal | "Lint catches mistakes before a device is involved." | `probe lint tests` |
| 0:30 | Terminal + simulator | "Run it. It drives the live widget tree, not screenshots." | `probe test tests/login.probe --device <id> -v` |
| 0:45 | Terminal | "Each step is reported; a failure names the line and saves a screenshot." | Point at the pass rows; optionally show one failing run |
| 0:55 | End card | "Free CLI. Tutorials and docs at flutterprobe.dev." | none |

`tests/login.probe`:

```
test "a user can log in"
  open the app
  wait until "Log in" appears
  type "user@example.com" into #email
  type "secret123" into #password
  tap #loginButton
  wait until "Dashboard" appears
```

Notes:
- Do not say anything that is not on screen: no speed numbers, no comparisons to other tools in this video.
- If you mention hosted reports, say that Probe Cloud is a design-partner preview and is not live yet.
- Use a device name that is not a personal device, and a demo app with no real account data.
