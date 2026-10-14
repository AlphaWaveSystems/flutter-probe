---
title: GitHub Actions
description: CI/CD workflow examples for running FlutterProbe tests in GitHub Actions.
---

The `actions/probe-test` directory in the repository is a ready-made composite action that installs the CLI from a release, runs `probe test` and attaches the JUnit report; copy it to its own repository to use or publish it (see its README). The workflows below show the same steps written out. The CLI is installed from a release asset (pin the version); the releases publish no checksum file, but GitHub records a SHA-256 digest for every asset, which the `actions/probe-test` action checks (the workflows below skip the check for brevity; add it before relying on them). These workflows were adapted from the project's own and have not been re-run on hosted runners as written: check the action versions against their documentation. The macOS asset below is for Apple Silicon runners; use `probe-darwin-amd64` on Intel.

## iOS Simulator Workflow

```yaml
name: E2E Tests (iOS)
on: [push, pull_request]

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v6

      - uses: subosito/flutter-action@v2
        with:
          flutter-version: '3.41.4'

      - name: Install the probe CLI
        run: |
          sudo curl --fail --silent --show-error --location -o /usr/local/probe \
            https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v0.25.2/probe-darwin-arm64
          sudo chmod +x /usr/local/bin/probe

      - name: Boot iOS simulator
        run: |
          DEVICE=$(xcrun simctl list devices available -j | \
            jq -r '.devices | to_entries[] | .value[] | select(.name | contains("iPhone")) | .udid' | head -1)
          xcrun simctl boot "$DEVICE"
          echo "DEVICE_UDID=$DEVICE" >> $GITHUB_ENV

      - name: Build & launch app
        run: |
          cd your-flutter-app
          flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
          xcrun simctl install $DEVICE_UDID build/ios/iphonesimulator/YourApp.app
          xcrun simctl launch $DEVICE_UDID com.example.yourapp

      - name: Run E2E tests
        run: |
          probe test tests/ \
            --device $DEVICE_UDID \
            --timeout 60s -v -y \
            --video \
            --format json -o reports/results.json

      - name: Generate HTML report
        if: always()
        run: probe report --input reports/results.json -o reports/report.html

      - name: Upload artifacts
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: test-reports
          path: reports/
```

## Android Emulator Workflow

```yaml
  android-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - name: Install the probe CLI
        run: |
          sudo curl --fail --silent --show-error --location -o /usr/local/probe \
            https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v0.25.2/probe-linux-amd64
          sudo chmod +x /usr/local/bin/probe

      - name: Enable KVM
        run: |
          echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666", OPTIONS+="static_node=kvm"' | sudo tee /etc/udev/rules.d/99-kvm4all.rules
          sudo udevadm control --reload-rules
          sudo udevadm trigger --name-match=kvm

      - name: Start Android emulator and run tests
        uses: reactivecircus/android-emulator-runner@v2
        with:
          api-level: 34
          script: |
            adb install -r your-app-debug.apk
            adb shell am start -n com.example.yourapp/.MainActivity
            sleep 10
            probe test tests/ \
              --device emulator-5554 \
              --timeout 60s -v -y \
              --video \
              --format json -o reports/results.json
            probe report --input reports/results.json -o reports/report.html

      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: android-reports
          path: reports/
```

## JUnit Integration

Produce JUnit XML for GitHub's test summary UI:

```yaml
      - name: Run tests (JUnit)
        if: always()
        run: |
          probe test tests/ \
            --device $DEVICE_UDID \
            --timeout 60s -y \
            --format junit -o reports/results.xml
        continue-on-error: true

      - name: Publish test results
        if: always()
        uses: dorny/test-reporter@v1
        with:
          name: E2E Test Results
          path: reports/results.xml
          reporter: java-junit
```

## Parallel Testing

Run iOS and Android tests simultaneously using platform-specific configs:

```yaml
jobs:
  ios-tests:
    runs-on: macos-latest
    steps:
      # ... (iOS workflow above)
      - run: probe test tests/ --config probe.ios.yaml --device $IOS_UDID -v -y

  android-tests:
    runs-on: ubuntu-latest
    steps:
      # ... (Android workflow above)
      - run: probe test tests/ --config probe.android.yaml --device emulator-5554 -v -y
```

## Test Sharding

Split tests across multiple runners for faster execution:

```yaml
jobs:
  test:
    runs-on: macos-latest
    strategy:
      matrix:
        shard: [1/3, 2/3, 3/3]
    steps:
      # ... setup steps
      - run: probe test tests/ --shard ${{ matrix.shard }} --device $DEVICE_UDID -v -y
```

## Docker (Self-Hosted)

A Docker setup is available in `docker/` for self-hosted CI with Android emulators. This is useful for teams running their own CI infrastructure.
