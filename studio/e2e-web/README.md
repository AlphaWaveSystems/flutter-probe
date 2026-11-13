# Studio UI E2E (browser mode)

Drives the **real Studio frontend and Go backend** with Playwright, without a native window, cursor or focus.

Studio started with `PROBE_STUDIO_HEADLESS=1` serves its built frontend over HTTP on `127.0.0.1` (random port, or
`PROBE_STUDIO_HEADLESS_PORT`), injects a small bridge that stands in for the Wails runtime (`window.go.main.App.*`
calls become HTTP calls to the same Go methods, `EventsOn` becomes a server-sent event stream) and prints
`PROBE_STUDIO_HEADLESS_URL=...`. It is for tests only: it needs a per-launch token (embedded in the bridge it serves),
refuses non-loopback hosts and cross-origin pages, and exposes every exported `App` method, so it is **not** the interface
for agents (that is the confined automation endpoint, `PROBE_STUDIO_AUTOMATION=1`).

The suite sets `PROBE_STUDIO_KEYCHAIN=memory` in `playwright.config.ts`. Studio then keeps API keys in memory, so no test reads or writes the real keychain. Only the headless build reads that variable; a normal Studio launch ignores it.

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
| 26 | no | The device settings panel opens and says to connect a simulator or emulator first |
| 27 | no | `ApplyDeviceSetting` without a connection is refused |
| 36 | yes, and `STUDIO_E2E_DEVICE_VIEW=1` | The shared device view: a canvas with the hub's frames (no screenshot loop image), Alt+click selects `#counter_button`, a click taps the List tab |
| 30-33 | **a physical phone** (opt-in) | Test connection, Connect with screen and widget tree, a probe file runs, a wrong token is refused; never in CI. See [PHYSICAL.md](PHYSICAL.md) |
| ai-01 to ai-10 | no | AI chat preview and form (`ai-preview.spec.ts`, fake providers on 127.0.0.1): the first message names its destination and nothing is sent until **Send**; a repeat to the same destination has no preview; the preview returns after the destination, the provider or the saved key changes; a second Enter sends once; an untouched AI form saves; a refused one shows its reason while the rest of Settings saves; a model change keeps a plain key |
| ex-01 to ex-08 | no | Explain (`ai-explain.spec.ts`, fake providers on 127.0.0.1, a seeded failed run): no button with AI off; the preview first and nothing sent until **Send**; the card says Advisory, the model and local or hosted, and leaves the results as they were; a destination on this machine is shown once per session and after a settings change; a hosted one before every Explain; hostile error text stays data and no credential or personal text leaves; a setting changed between the preview and Send sends nothing; a failing provider gives a fixed sentence; turning AI off hides the button |
| ai-s01 to ai-s07 | no | Settings AI section (`ai-settings.spec.ts`, fake provider on 127.0.0.1): Test connection with AI off makes no request; a local provider shows its checks; List models fills the suggestions; a plain key in `probe.yaml` survives Save; Save writes the `ai:` block and keeps the other keys; Off writes `provider: none`; a form that would not run is refused and not written |
| ai-h01 to ai-h09 | no | Chat header (`ai-chat-label.spec.ts`, fake provider on 127.0.0.1): AI off says so; a local model shows its model and that it runs on this machine; a remote endpoint says so when confirmed; the header follows a Settings save, a saved key or a removed key; a model id cannot change the header's text |

**Seeded run.** `playwright.config.ts` writes `.e2e-seed/run.json` in the workspace and starts Studio with `PROBE_STUDIO_SEED_RUN` pointing at it, so the screens that need a failed run (Explain) show without a device. The variable is read by the headless build only, and the file must be inside the workspace once links are resolved, a JSON list of tests in the shape of `RunState.results` with no unknown field, at most 1 MB and 50 tests; anything else is ignored with a note that never names the path. The seeded error text is hostile on purpose (markup, hidden characters, a credential shape, an email, a home folder).

Not covered here (they need the real window, so they stay in the native suite `make studio-e2e-native`): the native folder picker,
window chrome and keyboard focus, the recorder (it records taps made on the simulator itself).
