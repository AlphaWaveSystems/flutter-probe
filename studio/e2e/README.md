# Studio E2E suite

End-to-end tests for FlutterProbe Studio that drive the **real desktop app**
the way a person does — no test hooks compiled into Studio.

This is the native suite. It needs macOS, a second display and an unlocked
session with Accessibility and Screen Recording permission. The browser suite
covers the UI without a window; see [Modes](#modes).

- **Launch**: the built `.app` binary, with `PROBE_STUDIO_WORKSPACE` pointing at a workspace.
- **Find**: the macOS accessibility tree (`System Events`) — buttons by label, texts by content, the device picker as a pop-up, result rows by their `pass:`/`fail:` accessible name.
- **Act**: clicks at element centres, keystrokes, pop-up selection.
- **Assert**: accessibility tree first; window screenshots on every test; an optional vision provider (`STUDIO_E2E_AI_PROVIDER=openai|anthropic|local`, same backends as `probe.yaml` `ai:`) judges a screenshot where structure is not enough.
- **Device**: a named iOS simulator (`StudioE2E-iPhone17` by default), created and booted by the suite; the fixture app under `native-test-apps/studio-fixture/` is installed and relaunched before every test.

## Run

```bash
STUDIO_E2E_DISPLAY=2 make studio-e2e-native                      # the smoke set; builds Studio + fixture, writes reports/studio-e2e/
STUDIO_E2E_DISPLAY=2 STUDIO_E2E_SET=full make studio-e2e-native  # also the tests still to be ported
SKIP_BUILD=1 STUDIO_E2E_DISPLAY=2 scripts/studio-e2e.sh -run Test07   # reuse builds, one test
```

`make studio-e2e` still works for one release as a deprecated alias of
`make studio-e2e-native`.

Reports: `reports/studio-e2e/report.json`, `report.html`, `screenshots/`, `logs/`.

## The display rule

The suite never runs on the main display. It needs `STUDIO_E2E_DISPLAY=n`, with
n 2 or more, naming a display that is attached:

- **Unset**: the script refuses (exit 2) and says how many displays are attached.
  There is no default display.
- **1, zero, a negative number, or anything that is not a number**: refused. 1 is the main display.
- **Above the number of attached displays**: refused, with the count.
- **In CI** (`CI` is set, as on GitHub Actions) an unset variable skips the run
  with exit 0 and prints `studio-e2e: skipped: no second display configured`.
  A variable that is set but wrong still refuses.

The suite checks the same rule again before its first test. After moving a
window, the driver reads its position back and fails the test if the window did
not land on that display. A window is created on the main display for a moment
before it is moved.

The suite needs the machine's keyboard focus: typing into another window while a
test runs makes it fail.

## Modes

| Mode | Command | Window | Input |
|---|---|---|---|
| Headless browser (CI default) | `make studio-e2e-web`, or `STUDIO_E2E_NO_DEVICE=1 make studio-e2e-web` for the device-free tests | none | none |
| Headed browser (watching a run) | `STUDIO_E2E_HEADED=1 make studio-e2e-web` | a Chromium window, never a Studio window | Playwright |
| Native smoke set (opt-in) | `STUDIO_E2E_DISPLAY=2 make studio-e2e-native` | the real Studio and Simulator windows on display 2 | macOS accessibility and clicks |

A headed run of the real Studio window, driven by the automation endpoint, is
not implemented yet. It is tracked in #389.

## Tests

### Smoke set (run by default)

| # | Covers |
|---|---|
| 01 | Launch, workspace file list, toolbar, initial status |
| 07 | Recorder: three taps on the device pane → saved `.probe` with a tap step |

### To be ported (run with `STUDIO_E2E_SET=full`)

Each of these is ported to browser mode in #331 phase 2 and removed then. The
browser suite already covers the same behaviour for most of them:

| # | Covers | Browser suite |
|---|---|---|
| 02 | Device picker lists the named simulator with platform tag | browser 10 and 22 (the platform tag) |
| 03 | Connect → live device frame + widget tree in the inspector; disconnect | browser 11 |
| 04 | Open a file, Run, pass rows and summary | browser 12 |
| 05 | Failing file → fail row, error row, failed summary | browser 13 |
| 06 | Performance lines under a result (`start measuring`) | browser 14 |
| 08 | Workspace settings overlay (probe.yaml form) | browser 04 |
| 09 | AI chat pane toggle and API-key overlay | browser 05 |
| 10 | Connect with the app not running → error state | browser 18 |
| 11 | WiFi discovery overlay | browser 06 |
| 12 | Live steps: progress text, Cancel visible, every step row ends passed, verdict row heads the steps | browser 15 and 17; gaps below |
| 13 | Failing step keeps a `step fail` row naming its line, with the error | browser 13 and 16 |
| 14 | Manual WiFi entry explains an empty host | browser 19 (malformed host) and 23 (empty host) |

Still not covered by browser mode, to port before the native copies are removed (the empty host, the platform tag and the Record and Save states are covered by browser tests 22-24):

1. After a passing run, the last step row is `step pass`, no step is left running, and Cancel is hidden again (12).

Each test launches its own Studio, closes it, and relaunches the fixture app
(self-contained; explicit cleanup).

## Findings the suite documents (as of the first run)

- Studio's device pane does **not** forward clicks to the app (README still lists tap
  forwarding as a target). The recorder test therefore taps the Simulator window
  directly; `FindDevicePane` is kept for the live-frame assertion.
- `Stop` replaces the editor with the agent's assembled steps only — the
  `test "recorded flow"` header typed at `Record` is dropped. The test asserts the
  recorded tap lines, not the header.
- Studio did not honour the workspace `probe.yaml` (fixed in this change).

## Requirements

- macOS with Xcode (simulators), Flutter, Go, Wails CLI.
- A second display, named with `STUDIO_E2E_DISPLAY` (see [The display rule](#the-display-rule)),
  and an unlocked session.
- The terminal running the suite needs **Accessibility** and **Screen Recording**
  permission (System Settings → Privacy & Security) — `osascript` drives the UI
  and `screencapture` takes the window shots.
- A self-hosted macOS runner for CI (`.github/workflows/studio-e2e.yml`); hosted
  runners cannot grant those permissions.

## Dogfooding on your own app

See [DOGFOOD.md](DOGFOOD.md).
