# Studio UI E2E (browser mode)

Drives the **real Studio frontend and Go backend** with Playwright, without a native window, cursor or focus.

Studio started with `PROBE_STUDIO_HEADLESS=1` serves its built frontend over HTTP on `127.0.0.1` (random port, or
`PROBE_STUDIO_HEADLESS_PORT`), injects a small bridge that stands in for the Wails runtime (`window.go.main.App.*`
calls become HTTP calls to the same Go methods, `EventsOn` becomes a server-sent event stream) and prints
`PROBE_STUDIO_HEADLESS_URL=...`. It is for tests only: it needs a per-launch token (embedded in the bridge it serves),
refuses non-loopback hosts and cross-origin pages, and exposes every exported `App` method, so it is **not** the interface
for agents (that is the confined automation endpoint, `PROBE_STUDIO_AUTOMATION=1`).

## Run

```bash
make studio-e2e-web                       # builds Studio, prepares a workspace, a named simulator (macOS) and the fixture app
STUDIO_E2E_NO_DEVICE=1 make studio-e2e-web   # only the tests that need no device (any OS, CI)
SKIP_BUILD=1 scripts/studio-e2e-web.sh -g "12"   # reuse builds, one test
STUDIO_E2E_HEADED=1 make studio-e2e-web   # watch it in Chromium (never a Studio window)
```

The fixture app and the workspace use their own agent port (48690, `STUDIO_E2E_AGENT_PORT`), because simulators share the host network and another Probe-enabled app on the default port would otherwise answer.

Report: `reports/studio-e2e-web/html/index.html` (+ `report.json`, traces and screenshots of failures).

## Tests

| # | Needs a device | Covers |
|---|---|---|
| 01 | no | Launch: workspace listed, toolbar, disconnected status |
| 02 | no | Open a file: name and content in the editor |
| 03 | no | Device picker, Connect disabled until a booted device is picked |
| 04 | no | Settings overlay reads and saves probe.yaml |
| 05 | no | AI chat pane toggle and API key overlay |
| 06 | no | WiFi discovery overlay |
| 07 | no | Help overlay |
| 08 | no | Running without a device explains what to do |
| 09 | no | The bridge: event stream connected, Go methods answer |
| 10 | yes | Device picker lists the named simulator |
| 11 | yes | Connect: live screen and widget tree; disconnect |
| 12 | yes | Passing file: pass rows and summary |
| 13 | yes | Failing file: fail row, error row, summary |
| 14 | yes | Performance lines under a result |
| 15 | yes | Live steps: every step row passed; after the run the verdict heads its steps, the last step passes, no step is left running, and Cancel is hidden again |
| 16 | yes | Failing step row names its line |
| 17 | yes | Cancel stops a run |
| 18 | yes | Connect with the app not running reports an error |
| 19-21 | no | Manual WiFi entry: a malformed host is explained, an unreachable agent says what to check, the token is not kept |
| 22 | no | The picker shows physical phones (USB, WiFi only) with what each needs, and the platform tag of a simulator and an emulator |
| 23 | no | Manual WiFi entry with an empty host says a host is needed |
| 24 | no | Record and Save: disabled without a device or an edited file; Save enables once the file is edited |
| 25 | no | A device picked while the list is refreshing stays selected |
| 30-33 | **a physical phone** (opt-in) | Test connection, Connect with screen and widget tree, a probe file runs, a wrong token is refused; never in CI. See [PHYSICAL.md](PHYSICAL.md) |

Not covered here (they need the real window, so they stay in the native suite `make studio-e2e-native`): the native folder picker,
window chrome and keyboard focus, the recorder (it records taps made on the simulator itself).
