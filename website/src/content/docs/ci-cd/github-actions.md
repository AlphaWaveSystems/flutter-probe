---
title: GitHub Actions
description: CI/CD workflow examples for running FlutterProbe tests in GitHub Actions.
---

## iOS Simulator Workflow

```yaml
name: E2E Tests (iOS)
on: [push, pull_request]

jobs:
  test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - uses: subosito/flutter-action@v2
        with:
          flutter-version: '3.19.0'

      - name: Build probe CLI
        run: make build

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
          bin/probe test tests/ \
            --device $DEVICE_UDID \
            --timeout 60s -v -y \
            --video \
            --format json -o reports/results.json

      - name: Generate HTML report
        if: always()
        run: bin/probe report --input reports/results.json -o reports/report.html

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
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Build probe CLI
        run: make build

      - name: Start Android emulator and run tests
        uses: reactivecircus/android-emulator-runner@v2
        with:
          api-level: 34
          script: |
            adb install -r your-app-debug.apk
            adb shell am start -n com.example.yourapp/.MainActivity
            sleep 10
            bin/probe test tests/ \
              --device emulator-5554 \
              --timeout 60s -v -y \
              --video \
              --format json -o reports/results.json
            bin/probe report --input reports/results.json -o reports/report.html

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
          bin/probe test tests/ \
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
      - run: bin/probe test tests/ --config probe.ios.yaml --device $IOS_UDID -v -y

  android-tests:
    runs-on: ubuntu-latest
    steps:
      # ... (Android workflow above)
      - run: bin/probe test tests/ --config probe.android.yaml --device emulator-5554 -v -y
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
      - run: bin/probe test tests/ --shard ${{ matrix.shard }} --device $DEVICE_UDID -v -y
```

## Docker (Self-Hosted)

A Docker setup is available in `docker/` for self-hosted CI with Android emulators. This is useful for teams running their own CI infrastructure.
