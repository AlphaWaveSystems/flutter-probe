---
title: Codemagic
description: Run FlutterProbe tests on an iOS simulator in Codemagic with a codemagic.yaml recipe, with the JUnit report shown in the build overview.
---

This recipe builds a Flutter app with the FlutterProbe agent, boots an iOS simulator, runs your `.probe` tests and shows the JUnit report in Codemagic's build overview. It is written from Codemagic's own documentation (read 2026-10-14) and has **not** been run on Codemagic yet: run it once and adjust the machine type and simulator for your project.

## The recipe

```yaml
workflows:
  probe-e2e:
    name: FlutterProbe E2E (iOS simulator)
    instance_type: mac_mini_m2
    max_build_duration: 60
    environment:
      flutter: stable
      vars:
        PROBE_VERSION: "0.25.2"            # pin a CLI version
        APP_ID: com.example.myapp          # your bundle id
    scripts:
      - name: Install the FlutterProbe CLI
        script: |
          mkdir -p "$HOME/probe-bin"
          curl --fail --silent --show-error --location \
            --output "$HOME/probe-bin/probe" \
            "https://github.com/AlphaWaveSystems/flutter-probe/releases/download/v${PROBE_VERSION}/probe-darwin-arm64"
          chmod +x "$HOME/probe-bin/probe"
          echo "PATH=$HOME/probe-bin:$PATH" >> $CM_ENV
          "$HOME/probe-bin/probe" version
      - name: Build the app with the agent
        script: |
          flutter pub get
          flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
      - name: Boot a simulator and start the app
        script: |
          UDID=$(xcrun simctl list devices available -j | python3 -c "import sys,json; d=json.load(sys.stdin)['devices']; print(next(x['udid'] for v in d.values() for x in v if 'iPhone' in x['name']))")
          xcrun simctl boot "$UDID"
          xcrun simctl install "$UDID" build/ios/iphonesimulator/Runner.app
          xcrun simctl launch "$UDID" "$APP_ID"
          echo "SIMULATOR_UDID=$UDID" >> $CM_ENV
      - name: Run the FlutterProbe tests
        script: |
          mkdir -p reports
          probe test tests/ --device "$SIMULATOR_UDID" --format junit -o reports/junit.xml -y
        test_report: reports/junit.xml
    artifacts:
      - reports/junit.xml
```

## Notes

- **Facts from Codemagic's docs:** `instance_type`, `max_build_duration` and `environment.flutter` (`stable`, `beta`, `master` or a version) are workflow keys; `CM_ENV` is the file that sets environment variables for later scripts; `test_report` takes a glob for the result file, and JUnit XML is among the supported formats. [Flutter apps](https://docs.codemagic.io/yaml-quick-start/building-a-flutter-app/), [Testing](https://docs.codemagic.io/yaml-testing/testing/), [environment variables](https://docs.codemagic.io/yaml-basic-configuration/environment-variables/).
- **The Apple Silicon machine** is why the recipe downloads `probe-darwin-arm64`. On an Intel machine use `probe-darwin-amd64`.
- **Android emulators.** Codemagic's testing page states that Apple's virtualization does not support the nested virtualization Android emulators need, and recommends third-party integrations for Android emulator testing in macOS workflows. This recipe therefore covers the iOS simulator only. A physical iOS or Android device is not available on a Codemagic build machine.
- **Probe Cloud (preview).** `--upload-report` is optional and the hosted service is not live yet. See [Probe Cloud (preview)](/tools/probe-cloud-preview/). An upload never fails the build.
- **No checksum.** The releases publish no checksum file, so pin `PROBE_VERSION` and review release notes before upgrading.

See also [GitHub Actions](/ci-cd/github-actions/) and the [cloud device farm status](/tools/cloud-farms/).
