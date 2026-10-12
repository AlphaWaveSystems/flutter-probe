---
title: Probe Cloud (preview)
description: An opt-in preview that uploads a scrubbed copy of a test report to a hosted service. What is sent, what is not, and how to see the exact payload first.
---

> **Preview.** This feature is opt-in and the hosted service is **not live yet**. The client is shipped so you can inspect
> exactly what it would send. Paid plans are listed as planned on the [pricing page](/pricing/); nothing here changes the
> free, local-first behaviour of the CLI. Nothing is ever uploaded unless you turn it on.

## Turn it on

```bash
probe test tests/ --upload-report
```

or in `probe.yaml`:

```yaml
hosted:
  upload_report: true
  # url: https://your-endpoint.example      # the default is a placeholder that is not live yet
  # timeout: 5s
```

The `hosted:` block is unrelated to the device-farm `cloud:` block and the `--cloud-*` flags.

Uploading needs a license with the `probe_cloud` plan (`probe license activate <key>`). Without a key the CLI prints a short
note that this is a hosted feature, nothing is sent, and the run is unaffected.

### In CI

`probe license activate` writes the key to a file in your home directory, which you may not want on a CI runner. Set the key at
run time instead, from your CI secret store:

```yaml
env:
  PROBE_LICENSE_KEY: ${{ secrets.PROBE_LICENSE_KEY }}
```

`PROBE_LICENSE_KEY` takes precedence over a stored key, is only read (nothing is written to the runner), and is never printed:
not in the job log, not in warnings, not in the `--upload-dry-run` output. If a server echoes it back in an error, it is
redacted before it is shown.

## See what would be sent

```bash
probe test tests/ --upload-dry-run
```

prints the exact payload, after scrubbing, and sends nothing. It needs no license key and no network.

## What is sent

- The run's JSON report, the same document `--format json` writes (test names, file paths relative to your project, pass/fail,
  durations, error messages, performance numbers, run metadata). The report format is unchanged.
- A small envelope next to it: the Probe version, the device class (`emulator`, `simulator`, `physical`, `cloud`), the retry
  outcome of tests that were re-run (`retry_failed_tests`), and, when running in GitHub Actions, the repository, branch, commit
  SHA, pull request number, run id and run attempt, with two optional branch fields: `ci.base_branch`, the branch a pull
  request targets (pull requests only), and `ci.default_branch`, the repository's default branch (when the event payload
  names it). These come from the standard GitHub Actions variables
  (`GITHUB_REPOSITORY`, `GITHUB_REF_NAME`, `GITHUB_HEAD_REF`, `GITHUB_BASE_REF`, `GITHUB_SHA`, `GITHUB_RUN_ID`,
  `GITHUB_RUN_ATTEMPT`) and from the event payload file that `GITHUB_EVENT_PATH` names, which is read for the default branch
  only. No other environment variable is read.

### Needed for a real upload

The hosted service identifies a run by its CI facts, so a real upload (not a dry run) needs a GitHub Actions run with the standard
variables set, and a report that names its platform (a connected device). Outside CI, or when the platform or commit hash is
missing, the CLI prints a clear warning saying why nothing was uploaded; `--upload-dry-run` still prints the payload and notes
that a real upload would be refused. Pull requests use the source branch (`GITHUB_HEAD_REF`, not `N/merge`) as the branch and
send the target branch (`GITHUB_BASE_REF`) as `base_branch` (only for the `pull_request` and `pull_request_target` events); the job id (`GITHUB_JOB`, the job_id of the current job) is sent as
`job`. At most 5,000 results are accepted.

### Several uploads in one job

Uploads are keyed by the CI run, the job and a label. Two uploads from the same job (a matrix, shards or two suites) **must**
carry different labels, or the second is refused. Set it with `--upload-label` or `hosted.label`, for example `shard-2-of-4` or the
matrix values:

```bash
probe test tests/ --upload-report --upload-label "ios-${{ matrix.os }}"
```

A label is 1 to 64 characters of letters, digits and `. _ : / -`; anything else is replaced by `-`. GitHub's documentation defines
`GITHUB_JOB` as the job_id of the current job, and a matrix is defined under that job_id, so matrix legs most likely share it;
the documentation does not say so in as many words, which is why the matrix values belong in the label.

## What is not sent

- Screenshots, videos and logs.
- Device ids (UDIDs, serial numbers).
- Environment variables.
- Absolute paths and home directories: paths are made relative to the working directory or shortened to `~`.
- Anything that looks like a credential (private keys, cloud, API and payment-provider keys, tokens, bearer headers, `password=` style values, Probe
  license keys) is replaced with `[redacted]`. This is a safety net, not a guarantee: check `--upload-dry-run` for your own
  data, and keep secrets out of test names and error messages.
- Reports over 1 MB: long error messages are shortened first, and if the report is still too large nothing is sent.

## Failure behaviour

An upload is one attempt with a short timeout (5 seconds by default) and no retries. Redirects are never followed, so the key cannot be forwarded to another address; a redirect is reported as an error. Text that comes back from the server is stripped of control characters and escape sequences before it is printed. A failure, a slow endpoint or a missing
key prints a warning and never changes the exit status of the run or delays it noticeably.

## Reference

| Setting | Meaning |
|---|---|
| `--upload-report` / `hosted.upload_report` | Upload the scrubbed report after the run |
| `--upload-dry-run` | Print the payload, send nothing |
| `hosted.url` | Service base URL (https required; plain http only for localhost). The built-in default is a placeholder host that is not live |
| `--upload-label` / `hosted.label` | Label for this upload (suite, shard or matrix leg); needed when one job uploads more than once |
| `hosted.timeout` | Upload time limit, default `5s` |

The wire format of the upload is a preview and may change.
