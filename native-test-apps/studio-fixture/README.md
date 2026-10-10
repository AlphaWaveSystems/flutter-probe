# studio-fixture — target app for the Studio E2E suite

A small, deterministic Flutter app with `flutter_probe_agent` (path dependency
on `../../probe_agent`) so FlutterProbe Studio has real content to open, run,
record and inspect. Driven by `studio/e2e/` (see `make studio-e2e`).

| Screen | Keys | Used by |
|---|---|---|
| Counter | `counter_value`, `counter_button` ("Tap Me"), `counter_reset` | `probe-tests/counter.probe`, the recorder test |
| Form | `name_field`, `submit_button`, `dialog_ok`, `dialog_cancel`, `greeting` | `probe-tests/form.probe` |
| List | `item_list`, `item_<n>` (200 rows) | `probe-tests/list.probe` (scroll + `start measuring`) |

Bottom navigation keys: `nav_counter`, `nav_form`, `nav_list`.
`probe-tests/failing.probe` fails on purpose so the suite can assert Studio's failure UI.

Build for the simulator with the agent on:

```bash
flutter build ios --simulator --debug --dart-define=PROBE_AGENT=true
```

Bundle id: `com.alphawavesystems.studioFixture` (see `probe.yaml`).
