# Dogfooding Studio against your own app

The suite is parameterised, so the same tests that run on the bundled fixture
app can run against any Flutter app that embeds `flutter_probe_agent`.
A dogfood run is deliberately short — open the workspace, connect, run one
test file, read the results, record a three-step flow — not the app's full suite.

## 1. Describe the app

```bash
cp studio/e2e/dogfood.local.yaml.example studio/e2e/dogfood.local.yaml
```

`dogfood.local.yaml` is gitignored; keep app names, device names, UDIDs and
ports there only. Fill in:

| Key | Meaning |
|---|---|
| `workspace` | Folder with the app's `probe.yaml`. Studio reads `agent.port`, `defaults.timeout` and device ids from it (0.24+). |
| `test_file` / `pass_name` | A short `.probe` file and one test in it that passes. |
| `fail_file` / `perf_file` | Optional: a failing file and a `start measuring` file; `""` skips those tests. |
| `app_bundle` / `bundle_id` | The simulator build (`flutter build ios --simulator --dart-define=PROBE_AGENT=true --dart-define=PROBE_PORT=<port>`), or `""` if the app is already installed and running. |
| `sim_name` | A **named** simulator. The suite creates it if missing and never uses an anonymous `booted` device. |
| `out` | Report folder, so fixture and dogfood reports do not overwrite each other. |

## 2. Build the app with the agent on the port its `probe.yaml` declares

```bash
flutter build ios --simulator --debug \
  --dart-define=PROBE_AGENT=true --dart-define=PROBE_PORT=48xxx
```

Studio connects on the port from the workspace `probe.yaml`; the app must
listen on the same one.

## 3. Run

```bash
STUDIO_E2E_LOCAL=studio/e2e/dogfood.local.yaml SKIP_BUILD=1 scripts/studio-e2e.sh
```

`SKIP_BUILD=1` reuses the Studio build; drop it to rebuild Studio too (the
fixture app is not rebuilt when `app_bundle` is set).

## Physical iOS device over WiFi (manual for now)

The suite runs on a named simulator. To dogfood Studio's WiFi path on a
physical iPhone on the same network: build the app with
`--dart-define=PROBE_AGENT=true --dart-define=PROBE_WIFI=true`, run it on the
phone, open Studio's 📡 discovery, pick the phone, paste the token from the app
log, then run `test_file` and record a flow by tapping the phone. Automating
that needs a hand on the device, so it is a manual checklist, not a test.

## Shared machines

The run boots the named simulator and takes the Studio window to the front
for screenshots. On a Mac shared with other test sessions, agree on a device
window first and never remove other sessions' port forwards.

## What a dogfood run proves

`Test01`–`Test04`, `Test07` on the real app: workspace opens, the device
picker lists the named simulator, Studio connects with the app's own port,
a real test file runs green in the results pane, and the recorder produces a
valid `.probe` file from three taps. Everything else (overlays, error paths)
is app-independent and runs the same as on the fixture.
