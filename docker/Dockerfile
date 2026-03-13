# FlutterProbe CI Docker Image
# ghcr.io/flutterprobe/ci:latest
#
# Includes: Flutter SDK, Android SDK + emulator, probe CLI
# Usage:
#   docker run --rm -v $(pwd):/app ghcr.io/flutterprobe/ci:latest probe test

FROM ubuntu:22.04

ARG FLUTTER_VERSION=3.24.0
ARG ANDROID_API_LEVEL=34
ARG ANDROID_BUILD_TOOLS_VERSION=34.0.0
ARG PROBE_VERSION=latest

ENV DEBIAN_FRONTEND=noninteractive
ENV ANDROID_SDK_ROOT=/opt/android-sdk
ENV FLUTTER_ROOT=/opt/flutter
ENV PATH="${FLUTTER_ROOT}/bin:${ANDROID_SDK_ROOT}/cmdline-tools/latest/bin:${ANDROID_SDK_ROOT}/platform-tools:${ANDROID_SDK_ROOT}/emulator:/usr/local/go/bin:/root/go/bin:${PATH}"

# ---- System dependencies ----
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl wget unzip git xz-utils ca-certificates \
    openjdk-17-jdk \
    libglu1-mesa \
    adb \
    qemu-kvm libvirt-daemon-system \
    && rm -rf /var/lib/apt/lists/*

# ---- Go toolchain ----
RUN curl -fsSL https://go.dev/dl/go1.23.0.linux-amd64.tar.gz | tar -C /usr/local -xz

# ---- Flutter SDK ----
RUN curl -fsSL \
    "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
    | tar -C /opt -xJ \
 && flutter precache --android \
 && flutter config --no-analytics

# ---- Android SDK ----
RUN mkdir -p "${ANDROID_SDK_ROOT}/cmdline-tools" \
 && curl -fsSL \
    "https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip" \
    -o /tmp/cmdtools.zip \
 && unzip -q /tmp/cmdtools.zip -d /tmp/cmdtools \
 && mv /tmp/cmdtools/cmdline-tools "${ANDROID_SDK_ROOT}/cmdline-tools/latest" \
 && rm -rf /tmp/cmdtools.zip /tmp/cmdtools

RUN yes | sdkmanager --licenses && \
    sdkmanager \
      "platform-tools" \
      "platforms;android-${ANDROID_API_LEVEL}" \
      "build-tools;${ANDROID_BUILD_TOOLS_VERSION}" \
      "emulator" \
      "system-images;android-${ANDROID_API_LEVEL};google_apis;x86_64"

# ---- Create AVD ----
RUN echo no | avdmanager create avd \
    --name probe_ci \
    --package "system-images;android-${ANDROID_API_LEVEL};google_apis;x86_64" \
    --device "pixel_4"

# ---- probe CLI ----
# Install from release or build from source
COPY . /probe-src
RUN cd /probe-src && go build -ldflags="-s -w" -o /usr/local/bin/probe ./cmd/probe

# ---- Entrypoint script ----
COPY docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

WORKDIR /app
ENTRYPOINT ["/entrypoint.sh"]
CMD ["probe", "test", "--format", "junit", "--output", "reports/results.xml"]
