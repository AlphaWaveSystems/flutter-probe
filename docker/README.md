# docker

CI image for running FlutterProbe: Ubuntu with the Flutter SDK, the Android SDK and emulator, and the `probe` CLI.

```bash
docker run --rm -v $(pwd):/app ghcr.io/flutterprobe/ci:latest probe test
```

Build arguments (see `Dockerfile`): `FLUTTER_VERSION`, `ANDROID_API_LEVEL`, `ANDROID_BUILD_TOOLS_VERSION`, `PROBE_VERSION`.
`entrypoint.sh` is the container entrypoint.

Docker images cannot run the iOS simulator; use a macOS runner for iOS. See
[CI/CD](https://flutterprobe.dev/ci-cd/) for pipeline examples.
