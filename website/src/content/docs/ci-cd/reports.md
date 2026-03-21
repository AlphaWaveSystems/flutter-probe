---
title: Reports
description: Generate JSON, JUnit XML, and interactive HTML reports from test results.
---

FlutterProbe supports three output formats for test results.

## Terminal (Default)

```bash
probe test tests/
```

Prints colored pass/fail output to stdout.

## JUnit XML

Standard format supported by GitHub Actions, Jenkins, CircleCI, and other CI systems:

```bash
probe test tests/ --format junit -o reports/results.xml
```

Use with `dorny/test-reporter` or similar actions to display results in pull request summaries.

## JSON

Structured output with full test details, timings, and artifact references:

```bash
probe test tests/ --format json -o reports/results.json --video
```

### Report Metadata

JSON reports include a `metadata` section for CI/CD traceability:

```json
{
  "metadata": {
    "device_name": "iPhone 16 Pro",
    "device_id": "A1B2C3D4-E5F6-7890-ABCD-EF1234567890",
    "platform": "ios",
    "os_version": "iOS 18.6",
    "app_id": "com.example.myapp",
    "app_version": "1.2.16",
    "config_file": "probe.ios.yaml"
  }
}
```

Metadata is collected automatically: iOS version from the simulator runtime, Android version from `getprop`, app version from `dumpsys package`.

## HTML Report

Generate an interactive HTML dashboard from JSON results:

```bash
probe report --input reports/results.json -o reports/report.html
probe report --input reports/results.json -o reports/report.html --open
```

The HTML report displays:
- Pass/fail summary with timings
- Device and environment metadata
- Per-test details with step-by-step results
- Embedded screenshots and video playback
- Failure details with error messages

## Artifact Structure

The `reports/` directory is fully self-contained and portable:

```
reports/
  results.json           # test results with relative artifact paths
  report.html            # interactive HTML dashboard
  screenshots/           # failure & on-demand screenshots (PNG)
    failure_login_1234.png
    main_menu_5678.png
  videos/                # per-test screen recordings (H.264)
    login_test.mov
    navigation_test.mov
```

All artifact paths are **relative** to the report file. The entire folder can be uploaded to CI artifact storage, S3, or shared without breaking references.

## Viewing HTML Reports

The HTML report uses relative paths for embedded media. To view with full video/screenshot support:

```bash
# Recommended: serve via HTTP
cd reports && python3 -m http.server 8080
# Open http://localhost:8080/report.html

# Quick view (screenshots work, videos may not in all browsers)
open reports/report.html
```

## Configuration

```yaml
# probe.yaml
reports_folder: reports    # default output directory

defaults:
  screenshots: on_failure  # always | on_failure | never
  video: false             # enable per-test video recording
```

The `-o` flag directory takes precedence over `reports_folder` when specified.

## Video Recording Notes

| Platform | Format | Codec |
|----------|--------|-------|
| iOS Simulator | MOV | H.264 (for browser compatibility) |
| Android | MP4 | H.264 |

Install `ffmpeg` to stitch multi-segment Android recordings into a single file. Without it, segments are kept separate.
