---
title: License and use
description: What the FlutterProbe license allows for internal and production use, CI, consultants and hosted services, and when each version becomes Apache 2.0.
---

FlutterProbe's command-line tools and the rest of the repository are licensed under the [Business Source License 1.1](https://github.com/AlphaWaveSystems/flutter-probe/blob/main/LICENSE), except the MIT-licensed packages below. The Dart agent that runs inside your app, the `probe_annotation` and `probe_gen` packages, the GitHub Action, the Bitrise step and the VS Code extension are licensed under the MIT license.

This page explains how we read the license for common situations. It is not legal advice, and it does not change the license. If this page and the license text ever differ, the license text governs.

## What you may do

Use FlutterProbe for any purpose, including production testing, at no charge, except the one restriction below. You may also copy it, change it and redistribute it, as the license allows.

## What you may not do

Provide a commercial hosted testing service that competes with FlutterProbe. If your use is not covered by the license, the license requires you to buy a commercial license from the licensor, its affiliates or an authorized reseller, or to stop using FlutterProbe for that use.

## How we read "competes with FlutterProbe"

We read it as a hosted service, offered to others, that runs FlutterProbe tests for paying customers or gives them access to FlutterProbe itself, in place of FlutterProbe. Running your own tests with FlutterProbe, on your own machines, your CI or your own infrastructure, is not such a service. Neither is a consultancy using FlutterProbe on its own machines and CI to test its clients' apps, as long as it does not give clients access to FlutterProbe.

## Examples

| Situation | Allowed? |
|---|---|
| Developers run `probe` on their own machines | Yes |
| Your CI runs `probe test` on every change | Yes |
| You run FlutterProbe on shared test machines for your own apps | Yes |
| A contractor uses FlutterProbe to test your apps | Yes |
| A consultancy uses FlutterProbe on its own machines and CI to test its clients' apps as part of its work, without giving clients access to FlutterProbe | Yes, in our reading |
| You change FlutterProbe for your own use | Yes |
| You ship the Dart agent (MIT) in an app for your users | Yes |
| You run FlutterProbe as a hosted service that other companies pay to use for their tests | Not without a commercial license, when it competes with FlutterProbe |

## When each version becomes Apache 2.0

Each version becomes available under the Apache License 2.0 four years after its release. The license applies to each version separately.

## Open source status

The license is source-available, not open source: its notice says it "is not an Open Source license". The MIT-licensed Dart agent is open source. If your organization accepts only OSI-approved licenses for the tools it runs, write to us (see below) for written terms that cover your use.

## Copies and warranties

If you copy or redistribute FlutterProbe, include the license with each copy, as the license requires. The license provides the software "as is" and disclaims warranties, to the extent applicable law allows.

## Trademarks

The license does not give you rights in FlutterProbe's name or logo, except as the license requires.

## Questions and written terms

For a question about your organization's use that this page does not answer, or for a commercial license for a use the license does not cover, write to support@alphawavesystems.com. The [contact page](/contact/) says where to send each matter.
