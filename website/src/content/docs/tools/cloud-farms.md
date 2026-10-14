---
title: Cloud device farms
description: What FlutterProbe's cloud device farm providers do today, what was checked against each vendor's documentation, what is not verified, and what the relay requirement means.
---

FlutterProbe has code for five cloud device farms. This page says what that code does, what was checked against each vendor's own documentation on 2026-10-14, and what has **not** been shown to work. Everything here is "implemented, not verified on a live account": no farm run has been recorded for this project, so no provider is described as supported beyond that.

The farms are optional. Local emulators, simulators and physical devices are the tested path.

## Support level

| Provider | `--cloud-provider` | Platforms in the code | Support level |
|---|---|---|---|
| BrowserStack App Automate | `browserstack` | Android, iOS | Implemented, not verified on a live account |
| AWS Device Farm | `aws` | Android, iOS | Implemented, not verified on a live account |
| Sauce Labs Real Device Cloud | `saucelabs` | Android, iOS | Implemented, not verified on a live account |
| LambdaTest (now TestMu AI) real devices | `lambdatest` | Android, iOS | Implemented, not verified on a live account |
| Firebase Test Lab | `firebase` | Android only (Robo test) | Implemented, not verified, **deprecated by Google** |

## What a cloud run needs: a relay

No farm lets your computer connect to a port inside the app on a rented device. FlutterProbe's cloud mode therefore works the other way round: the agent in the app connects **out** to a relay, and the CLI connects to the same relay.

- The relay is a service that implements `POST /api/v1/relay/sessions` at `cloud.url` (see `internal/cloud/relay.go`). **The relay server is not part of this repository, and the hosted Probe Cloud service is not live.** Until you operate a compatible relay, an end-to-end farm run is not possible. Without a relay the providers' direct port forwarding returns an error that says so.
- The agent must be built with `PROBE_RELAY_URL` and `PROBE_RELAY_TOKEN`, so the relay session is created before the build (`--relay-url`, `--relay-token`).
- Whether each farm allows the device to make that outbound connection, and how real iOS devices re-sign the app, was not tested. Check your plan with the vendor.

## What was checked against the vendors' documentation

Read 2026-10-14. "Matches" means the request our code sends is the one the page describes; it is not a test against a live account.

### BrowserStack App Automate
- **Upload.** Matches: `POST https://api-cloud.browserstack.com/app-automate/upload`, basic auth, multipart `file`, response `app_url` (`bs://...`). [Apps API](https://www.browserstack.com/docs/app-automate/api-reference/appium/apps)
- **Devices.** Matches: `GET /app-automate/devices.json`, fields `device`, `os`, `os_version`. [Devices API](https://www.browserstack.com/docs/app-automate/api-reference/appium/devices)
- **Session details.** Matches: `GET /app-automate/sessions/{id}.json` with `video_url`. [Sessions API](https://www.browserstack.com/docs/app-automate/api-reference/appium/sessions)
- **Mismatch.** Our stop call sends `PUT .../sessions/{id}.json` with status `completed`; the page lists `passed` and `failed`. The stop may be refused with a warning. Tracked in [#435](https://github.com/AlphaWaveSystems/flutter-probe/issues/435).
- **Not confirmed.** The Appium hub URL and capability names were not stated on the pages read.

### AWS Device Farm
- **Mismatch (fixed).** `CreateRemoteAccessSession` was sent without `appArn`, so the uploaded app was never installed; and `remoteDebugEnabled` is not in the documented request. The call now sends `appArn` when the upload returned an ARN. [CreateRemoteAccessSession](https://docs.aws.amazon.com/devicefarm/latest/APIReference/API_CreateRemoteAccessSession.html)
- **Note.** The API documents a managed Appium endpoint (`endpoints.remoteDriverEndpoint`); FlutterProbe does not use it. Device Farm is available in `us-west-2` only. [What is AWS Device Farm](https://docs.aws.amazon.com/devicefarm/latest/developerguide/welcome.html)
- **Not confirmed.** Upload, device listing and artifact calls were not compared line by line.

### Sauce Labs Real Device Cloud
- **Upload.** Matches: `POST /v1/storage/upload`, form field `payload`; the page also lists a `name` field that our request does not send ([#436](https://github.com/AlphaWaveSystems/flutter-probe/issues/436)). Data centers `api.us-west-1.saucelabs.com` and `api.eu-central-1.saucelabs.com`. [Storage API](https://docs.saucelabs.com/dev/api/storage/)
- **Devices and jobs.** Matches: `GET /v1/rdc/devices`, `/v1/rdc/jobs`, `/v1/rdc/jobs/{id}`. The page also lists `PUT /v1/rdc/jobs/{id}/stop`, which we do not use (we end the Appium session instead). [RDC API](https://docs.saucelabs.com/dev/api/rdc/)
- **Not confirmed.** The Appium endpoint and capability names.

### LambdaTest (TestMu AI)
- The vendor has renamed itself TestMu AI. The API index names `/app/upload/realDevice`, `/v1/device` and the hub `mobile-hub.lambdatest.com/wd/hub`, which are the ones our code calls. Request and response shapes were **not** verified ([#437](https://github.com/AlphaWaveSystems/flutter-probe/issues/437)). [API documentation](https://www.lambdatest.com/support/api-doc/)

### Firebase Test Lab (deprecated)
Google's page says: "Firebase Test Lab is deprecated and will be fully shut down on September 30, 2027." It points to the Developer Device Platform on Google Cloud. [Test Lab](https://firebase.google.com/docs/test-lab); [migration guide](https://docs.cloud.google.com/developer-device-platform/device-run/migrate)

`probe test --cloud-provider firebase` now prints this notice. What to know before migrating:

- The Developer Device Platform is driven by `gcloud beta device-run`. Its migration guide lists Android instrumentation tests and iOS XCTest, and does not mention Robo tests, Appium, real iOS devices or a REST API.
- FlutterProbe's provider starts a **Robo** test on Android devices and keeps the app alive while the agent talks to the relay. A replacement therefore needs a design of its own; it is not a drop-in change. A replacement provider is tracked in [#434](https://github.com/AlphaWaveSystems/flutter-probe/issues/434) and is not built.
- Until you migrate, keep local devices or another farm in your plan.

## Maestro Cloud

Maestro Cloud runs **Maestro flows**: its documentation describes uploading "your Maestro tests" with an app build (an Android APK, or an iOS simulator `.app`) and the flow files. [Maestro Cloud](https://docs.maestro.dev/maestro-cloud/run-tests-on-maestro-cloud.md) ProbeScript tests cannot run there, and FlutterProbe has no Maestro Cloud provider. If you use Maestro Cloud, keep your Maestro flows for it. If you want your suite in ProbeScript, `probe-convert` translates Maestro tests (see [probe-convert](/tools/probe-convert/)) so you can run them with FlutterProbe on your own devices and CI.

## Where this goes next

Verifying a provider needs a live account on that farm. Until a run is recorded, the support level stays "implemented, not verified". See [#438](https://github.com/AlphaWaveSystems/flutter-probe/issues/438) and the open issues labelled `area/cloud`.
