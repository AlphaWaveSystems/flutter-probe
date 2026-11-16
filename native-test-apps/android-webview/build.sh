#!/usr/bin/env bash
# Builds the WebView fixture APK from source with the Android SDK command-line tools (no Gradle):
#
#   ./build.sh            -> out/probe-webview-fixture.apk
#
# Needs: the Android SDK (ANDROID_HOME or ANDROID_SDK_ROOT, else ~/Library/Android/sdk or ~/Android/Sdk) with build-tools and one platform
# (API 26 or newer), and a JDK 11 or newer (javac, keytool). The APK is signed with a throwaway key made in out/ on every build (a
# debug key, not a release key); nothing built is committed.
set -euo pipefail
cd "$(dirname "$0")"

sdk="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
if [[ -z "$sdk" ]]; then
  for d in "$HOME/Library/Android/sdk" "$HOME/Android/Sdk"; do [[ -d "$d" ]] && sdk="$d" && break; done
fi
[[ -d "$sdk/build-tools" ]] || { echo "build.sh: no Android SDK found: set ANDROID_HOME" >&2; exit 1; }
for tool in javac keytool zip; do command -v "$tool" >/dev/null || { echo "build.sh: $tool is not installed" >&2; exit 1; }; done

# the newest build-tools and the newest platform
bt="$sdk/build-tools/$(ls "$sdk/build-tools" | sort -V | tail -1)"
platform="$(ls -d "$sdk"/platforms/android-[0-9]* 2>/dev/null | sort -V | tail -1 || true)"
[[ -n "$platform" && -f "$platform/android.jar" ]] || { echo "build.sh: no platform with android.jar under $sdk/platforms" >&2; exit 1; }
api="${platform##*android-}"; api="${api%%[!0-9]*}"
(( api >= 26 )) || { echo "build.sh: the newest platform is API $api; API 26 or newer is needed" >&2; exit 1; }
echo "build-tools: $(basename "$bt"), platform: API $api"

rm -rf out && mkdir -p out/classes
"$bt/aapt2" link -o out/base.apk -I "$platform/android.jar" --manifest AndroidManifest.xml -A assets \
  --min-sdk-version 26 --target-sdk-version "$api"
javac --release 11 -cp "$platform/android.jar" -d out/classes src/dev/probe/webviewfixture/MainActivity.java
"$bt/d8" --release --lib "$platform/android.jar" --min-api 26 --output out out/classes/dev/probe/webviewfixture/*.class
cp out/base.apk out/unsigned.apk
(cd out && zip -q -j unsigned.apk classes.dex)
"$bt/zipalign" -f -p 4 out/unsigned.apk out/aligned.apk
keytool -genkeypair -keystore out/debug.keystore -storepass android -keypass android -alias probe -keyalg RSA -keysize 2048 \
  -validity 3650 -dname "CN=Probe WebView Fixture (throwaway debug key)" >/dev/null 2>&1
"$bt/apksigner" sign --ks out/debug.keystore --ks-pass pass:android --key-pass pass:android --out out/probe-webview-fixture.apk out/aligned.apk
echo "built out/probe-webview-fixture.apk (package dev.probe.webviewfixture)"
