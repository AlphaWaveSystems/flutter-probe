# tests

`.probe` files used to test FlutterProbe itself (not the Go unit tests, which live next to the code in
`internal/`, and not the Dart agent tests in `probe_agent/test/`).

| Path | What |
|---|---|
| `*.probe` (top level) | Small example suites (`login`, `cart`, `onboarding`) |
| `e2e/` | Feature suites that need a running app: lifecycle, clipboard, CSV-driven, hooks, HTTP calls, location, random data |
| `e2e_cli_params/` | Shell scripts exercising CLI flags end to end (`run_all.sh`, `health_check.sh`) |
| `regression/` | Visual regression sample |
| `recipes/`, `fixtures/` | Shared recipes and data |

The reference test app is `e2e-test-workspace` (Flutter plus native twins). Run a suite with
`probe test tests/e2e --config probe.yaml`.
