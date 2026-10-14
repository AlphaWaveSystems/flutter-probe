# FlutterProbe tests (GitHub Action)

Installs the [FlutterProbe](https://flutterprobe.dev) CLI from its GitHub release and runs your `.probe` tests on a device that your own job provides, with a JUnit report. It runs only on your CI, with your devices; it needs no secret of ours.

This directory is self-contained: copy it to the root of a new public repository (GitHub Marketplace needs one action per repository, with `action.yml` at the root) and it works unchanged. Replace `<owner>/<repo>` in the examples with that repository.

> **Status.** Written against GitHub's documentation (read 2026-10-14) and tested locally (asset mapping and argument assembly). It has **not** run on a hosted runner yet; run the example once before relying on it.

## Usage

```yaml
- uses: <owner>/<repo>@v1
  with:
    version: '0.25.2'            # pin a version in CI
    device: ${{ steps.sim.outputs.udid }}
    tests: tests/
```

The job must already have a device: an iOS simulator on a macOS runner, or an Android emulator (see `examples/flutter-android.yml`, which installs the CLI with `run-tests: 'false'` and runs `probe test` inside the emulator step). Build the app with `--dart-define=PROBE_AGENT=true` first. See `examples/` for both.

## Inputs

| Input | Default | Meaning |
|---|---|---|
| `version` | `latest` | CLI version, like `0.25.2`. Pin one in CI. |
| `run-tests` | `true` | `false` only installs the CLI. |
| `tests` | `tests/` | Files or folders, separated by spaces. |
| `device` | empty | Device id; empty if one device is connected. |
| `working-directory` | `.` | Folder with `probe.yaml` and the tests. |
| `junit-path` | `reports/junit.xml` | JUnit report path; empty turns it off. |
| `timeout` | empty | Per-step timeout, like `60s`. |
| `upload-report` | `false` | Send the scrubbed report to Probe Cloud (preview). |
| `upload-artifact` | `true` | Attach the JUnit report to the run. |
| `artifact-name` | `probe-junit` | Artifact name; set a different one per matrix leg. |
| `skip-verify` | `false` | `true` skips the SHA-256 check (off by default). |
| `github-token` | the workflow token | Reads the release digest from the GitHub API. |
| `extra-args` | empty | More `probe test` arguments. |

Outputs: `exit-code`, `junit-path`. The action uploads the report and then fails the job if `probe test` failed.

## Runners

Linux x64, macOS (Intel and Apple Silicon) and Windows x64 are mapped to the release assets `probe-linux-amd64`, `probe-darwin-amd64`, `probe-darwin-arm64` and `probe-windows-amd64.exe`. Other combinations (for example Linux ARM64) stop with a message, because no release asset exists for them.

## Verification

The release publishes no checksum file, but GitHub records a SHA-256 digest for every release asset (the `digest` field of the [Releases API](https://docs.github.com/en/rest/releases/assets)). The install step downloads the asset, compares its SHA-256 with that digest, and installs nothing if they differ or if the release has no digest. `skip-verify: 'true'` turns the check off; it is off by default, and you should not use it in CI you rely on. Pin `version` too, so a new release is a change you review. The action pins `actions/upload-artifact` by commit SHA; keep it current with Dependabot.

## Optional: Probe Cloud upload (preview)

`upload-report: 'true'` adds `--upload-report`. Probe Cloud is a design-partner preview and the hosted service is **not live yet**, so this does nothing useful until it is. When it is, the job needs a license key in the environment and permission to request the job's identity token:

```yaml
permissions:
  contents: read
  id-token: write
steps:
  - uses: <owner>/<repo>@v1
    env:
      PROBE_LICENSE_KEY: ${{ secrets.PROBE_LICENSE_KEY }}
    with:
      upload-report: 'true'
```

An upload never fails the run. Details: [Probe Cloud preview](https://flutterprobe.dev/tools/probe-cloud-preview/).

## Test the scripts locally

```bash
bash scripts/test-install.sh     # asset mapping, and the checksum path with a good, a tampered and an unpublished download (no network)
```

## License

MIT, see `LICENSE`. The FlutterProbe CLI that this action installs is under the Business Source License 1.1 (free to use, except offering a competing hosted testing service); see the [repository](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/LICENSE).
