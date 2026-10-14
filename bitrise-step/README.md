# FlutterProbe tests (Bitrise Step)

Installs the [FlutterProbe](https://flutterprobe.dev) CLI from its GitHub release and runs your `.probe` tests on a device that your workflow provides (for example an iOS simulator on a macOS stack), writing a JUnit report into the deploy directory. It runs only on your build machine and needs no secret of ours.

This directory is self-contained: copy it to the root of its own public repository and it works unchanged.

> **Status.** Written against Bitrise's step template and documentation (read 2026-10-14); the script was tested locally with stubs. It has **not** run on Bitrise. The `host_os_tags` and `project_type_tags` in `step.yml` follow the template's examples: run `bitrise` against the step before relying on it.

## Use it without the Step Library

This Step is used straight from its git repository, through Bitrise's `git::` source (documented under "Adding Steps to a Workflow"). It is not published in the Bitrise Step Library:

```yaml
workflows:
  e2e:
    steps:
    - git-clone: {}
    - flutter-installer: {}
    - script:
        title: Build the app with the agent and start it on a simulator
        inputs:
        - content: |
            #!/bin/bash
            set -ex
            flutter build ios --debug --simulator --dart-define=PROBE_AGENT=true
            # boot a simulator, install and launch the app (see the FlutterProbe docs)
    - git::https://github.com/<owner>/<repo>.git@main:
        title: FlutterProbe tests
        inputs:
        - probe_version: "0.25.2"
        - tests: tests/
        - device: "$SIMULATOR_UDID"
```

`<owner>/<repo>` is the repository you copy this directory to. Build the app with `--dart-define=PROBE_AGENT=true` and start it before the Step.

## Inputs

| Input | Default | Meaning |
|---|---|---|
| `probe_version` | `latest` | CLI version, like `0.25.2`. Pin one in CI. |
| `tests` | `tests/` | Files or folders, separated by spaces. |
| `device` | empty | Device id; empty if one device is connected. |
| `junit_path` | `$BITRISE_DEPLOY_DIR/probe-junit.xml` | Where the JUnit report is written. |
| `timeout` | empty | Per-step timeout, like `60s`. |
| `upload_report` | `no` | `yes` sends the scrubbed report to Probe Cloud (preview; the hosted service is **not live yet**; needs `PROBE_LICENSE_KEY` as a secret). |
| `skip_verify` | `no` | `yes` skips the SHA-256 check of the download (keep `no` in CI you rely on). |
| `extra_args` | empty | More `probe test` arguments. |

Output: `PROBE_EXIT_CODE`. The Step fails when `probe test` fails.

## Stacks

macOS (Apple Silicon and Intel) and Linux x64 map to the release assets `probe-darwin-arm64`, `probe-darwin-amd64` and `probe-linux-amd64`; other combinations stop with a message. The releases publish no checksum file, but GitHub records a SHA-256 digest for every release asset (the `digest` field of the [Releases API](https://docs.github.com/en/rest/releases/assets)). The Step compares the download with it and installs nothing on a mismatch or when no digest is published. Pin `probe_version` too.

## License

MIT, see `LICENSE`. The FlutterProbe CLI it installs is under the Business Source License 1.1 (free to use, except offering a competing hosted testing service); see the [repository](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/LICENSE).
