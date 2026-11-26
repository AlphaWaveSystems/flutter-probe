# docker

A Dockerfile for running FlutterProbe in CI: Ubuntu with the Flutter SDK, the Android SDK and emulator, and the `probe` CLI built from this repository.
**Nothing built from it is published**: build it yourself, from the **repository root** (the Dockerfile copies the source of the repository):

```bash
docker build -f docker/Dockerfile -t probe-ci .
docker run --rm -v $(pwd):/app probe-ci probe test
```

**Not verified.** This file has not been built by whoever last edited it (no Docker on that machine). The Dockerfile installs Go 1.23.0 while `go.mod` requires a newer Go, and
it declares a `PROBE_VERSION` argument that nothing uses.

Build arguments (see `Dockerfile`): `FLUTTER_VERSION`, `ANDROID_API_LEVEL`, `ANDROID_BUILD_TOOLS_VERSION`, `PROBE_VERSION`.
`entrypoint.sh` is the container entrypoint.

Containers cannot run the iOS simulator; use a macOS runner for iOS. See
[CI/CD](https://flutterprobe.dev/ci-cd/) for pipeline examples.
