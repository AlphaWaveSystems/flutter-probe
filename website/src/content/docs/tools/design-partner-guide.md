---
title: Probe Cloud design-partner guide
description: What a Probe Cloud design partner installs, what to expect while the hosted service is in preview, and how to send feedback.
---

This page is for the first teams trying Probe Cloud, the hosted report service that is in preview. If you have not applied yet, see [pricing](/pricing/#design-partners). Everything on this page is free to follow: the CLI and agent do not need Probe Cloud.

> **Status.** The hosted service is not live yet. Today you can install everything, run your tests, and inspect exactly what an upload would send with `--upload-dry-run`. When the service goes live, design partners get access first. The beta may change or end, and plan details are not final.

## What you install

1. **The CLI.** `brew tap AlphaWaveSystems/tap && brew install probe`, or a binary from the [releases page](https://github.com/AlphaWaveSystems/flutter-probe/releases/latest). Check it with `probe version`.
2. **The agent** in your Flutter app, as a dev dependency, started only when you build with `--dart-define=PROBE_AGENT=true`. See [installation](/getting-started/installation/).
3. **A first test.** `probe init`, then `probe lint tests`, then `probe test tests/example.probe --device <id>`. The [quick start](/getting-started/quick-start/) walks through it.

## Try the upload without a service

```bash
probe test tests/ --device <id> --upload-dry-run
```

prints the exact payload, after scrubbing, and sends nothing. It needs no license key and no network. Inside GitHub Actions the payload also carries the repository, branch, commit and run facts, and it says whether the job could send an identity token (`attestation: present` or `absent`). Read what is and is not sent on the [Probe Cloud preview page](/tools/probe-cloud-preview/).

## In GitHub Actions

When the service is live, an upload from a workflow needs:

- a license key with the Probe Cloud plan, from your CI secret store as `PROBE_LICENSE_KEY` (we send it to you when your access starts);
- `permissions: { id-token: write, contents: read }` on the job, so the CLI can send the job's identity token;
- `--upload-report` on `probe test` (or `hosted.upload_report: true` in `probe.yaml`).
- `--upload-local` on `probe test`, for one run on a local simulator, emulator or device. It is unverified evidence, and it needs a team that has enabled local uploads. See [Probe Cloud (preview)](/tools/probe-cloud-preview/).

Two uploads from one job need different `--upload-label` values. An upload never fails your run: any problem is a warning.

## What we ask of you

- Use it on a real project and tell us what is missing, confusing or wrong. Short notes are fine. A feedback call is optional: written notes are enough.
- Keep secrets out of test names and error messages. The upload scrubs known secret patterns, but local output is not scrubbed.

## How to send feedback

- **Bugs and feature requests:** open an issue on [GitHub](https://github.com/AlphaWaveSystems/flutter-probe/issues). Issues are public, so leave out private details.
- **Anything private, or questions about access:** email [support@alphawavesystems.com](mailto:support@alphawavesystems.com). We aim to reply within a few working days.

See the [terms](/terms/), [privacy policy](/privacy/) and [refund policy](/refunds/) drafts.
