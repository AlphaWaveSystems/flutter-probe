# Studio E2E suite

End-to-end tests for FlutterProbe Studio that drive the **real desktop app**
the way a person does — no test hooks compiled into Studio.

- **Launch**: the built `.app` binary, with `PROBE_STUDIO_WORKSPACE` pointing at a workspace.
- **Find**: the macOS accessibility tree (`System Events`) — buttons by label, texts by content, the device picker as a pop-up, result rows by their `pass:`/`fail:` accessible name.
- **Act**: clicks at element centres, keystrokes, pop-up selection.
- **Assert**: accessibility tree first; window screenshots on every test; an optional vision provider (`STUDIO_E2E_AI_PROVIDER=openai|anthropic|local`, same backends as `probe.yaml` `ai:`) judges a screenshot where structure is not enough.
- **Device**: a named iOS simulator (`StudioE2E-iPhone17` by default), created and booted by the suite; the fixture app under `native-test-apps/studio-fixture/` is installed and relaunched before every test.

## Run

```bash
make studio-e2e                 # builds Studio + fixture, runs, writes reports/studio-e2e/
SKIP_BUILD=1 scripts/studio-e2e.sh -run Test04   # reuse builds, one test
```

Reports: `reports/studio-e2e/report.json`, `report.html`, `screenshots/`, `logs/`.

## Tests

| # | Covers |
|---|---|
| 01 | Launch, workspace file list, toolbar, initial status |
| 02 | Device picker lists the named simulator with platform tag |
| 03 | Connect → live device frame + widget tree in the inspector; disconnect |
| 04 | Open a file, Run, pass rows and summary |
| 05 | Failing file → fail row, error row, failed summary |
| 06 | Performance lines under a result (`start measuring`) |
| 07 | Recorder: three taps on the device pane → saved `.probe` with a tap step |
| 08 | Workspace settings overlay (probe.yaml form) |
| 09 | AI chat pane toggle and API-key overlay |
| 10 | Connect with the app not running → error state |
| 11 | WiFi discovery overlay |

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
- The terminal running the suite needs **Accessibility** and **Screen Recording**
  permission (System Settings → Privacy & Security) — `osascript` drives the UI
  and `screencapture` takes the window shots.
- A self-hosted macOS runner for CI (`.github/workflows/studio-e2e.yml`); hosted
  runners cannot grant those permissions.

## Dogfooding on your own app

See [DOGFOOD.md](DOGFOOD.md).
