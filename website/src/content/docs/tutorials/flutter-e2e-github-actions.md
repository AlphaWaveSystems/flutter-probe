---
title: "Run Flutter end-to-end tests in GitHub Actions"
description: "Run FlutterProbe tests on an Android emulator in GitHub Actions: install the CLI from a release, build the app with the agent, run the suite, publish JUnit results and keep reports as artifacts."
---

This workflow builds a Flutter app with the FlutterProbe agent, starts an Android emulator, runs your `.probe` tests and publishes JUnit results. It uses the released CLI binary, so you do not build FlutterProbe from source. Action versions and the emulator API level change over time: check them against the action's own documentation.

## The workflow

```yaml
name: E2E
on: [pull_request]

jobs:
  android:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: subosito/flutter-action@v2
        with:
          channel: stable

      - name: Install the probe CLI
        run: |
          curl -fsSL -o probe https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v0.25.1/probe-linux-amd64
          chmod +x probe
          ./probe version

      - name: Build the app with the agent
        run: |
          flutter pub get
          flutter build apk --debug --dart-define=PROBE_AGENT=true

      - name: Run the tests on an emulator
        uses: reactivecircus/android-emulator-runner@v2
        with:
          api-level: 34
          script: |
            adb install -r build/app/outputs/flutter-apk/app-debug.apk
            adb shell am start -n com.example.myapp/.MainActivity
            ./probe test tests/ --device emulator-5554 -y --format junit -o reports/results.xml

      - name: Keep the reports
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: e2e-reports
          path: reports/
```

Change `com.example.myapp` to your app id. Pin the CLI version in the download URL, and keep the CLI and the `flutter_probe_agent` package on the same version (see [Compatibility](/advanced/compatibility/)).

## What each step does

- **Install the CLI.** One binary from the [release page](https://github.com/AlphaWaveSystems/flutter-probe/releases). Use `probe-darwin-arm64` on a macOS runner.
- **Build with the flag.** `--dart-define=PROBE_AGENT=true` is what starts the agent. Without it the CLI cannot connect.
- **Run on the emulator.** `probe test` waits for the agent to come up and connects. `-y` accepts permission prompts automatically.
- **JUnit output.** Most CI systems show JUnit results natively. Use a test-reporter action to show them in the pull request.

## Make it faster and steadier

- Split the suite across jobs with `--shard 1/3`, `--shard 2/3` and `--shard 3/3`.
- If the first connection after install times out on a busy runner, raise `agent.dial_timeout` in `probe.yaml` or pass `--dial-timeout 90s`. The default is 30 seconds.
- Keep failed-step screenshots by uploading `reports/` with `if: always()`.

## Next

[Find and fix flaky tests](/tutorials/triage-flaky-flutter-tests/), or see all options on the [GitHub Actions](/ci-cd/github-actions/) page.

---

The CLI and everything on this page is free. Probe Cloud, hosted report storage, is a [design-partner preview](/probe-cloud/) and is not live yet.
