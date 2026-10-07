#!/bin/bash
# Android release build. Runs as root inside the fyneio/fyne-cross-images:android image
# (Android SDK, NDK, bundletool, JDK). Sources at /src (read-only), keystore at /keystore,
# results in /out:
#   Multi2FA-TOTP-<v>-android.aab                 signed bundle for Google Play
#   Multi2FA-TOTP-<v>-android-universal.apk       every ABI (arm64-v8a, armeabi-v7a, x86_64, x86)
#   Multi2FA-TOTP-<v>-android-arm64-v8a.apk       64-bit ARM only (practically every current phone)
set -euo pipefail

: "${VERSION:?}" "${BUILD:?}" "${APP_ID:?}" "${KEYSTORE:?}" "${KEYSTORE_PASS:?}" "${KEY_ALIAS:?}" "${KEY_PASS:?}"
BT=$(ls -d /usr/local/android_sdk/build-tools/* | sort -V | tail -1)
# zipalign and aapt2 from build-tools need the NDK's libc++ in this image.
export LD_LIBRARY_PATH="$BT/lib64:/usr/local/android_sdk/ndk-bundle/toolchains/llvm/prebuilt/linux-x86_64/lib64"
# The image's fyne CLI is older; v1.7.3 targets the current Play SDK level (36).
GOBIN=/go/bin go install fyne.io/tools/cmd/fyne@v1.7.3
FYNE=/go/bin/fyne
NAME=Multi2FA
FILE=Multi2FA-TOTP-$VERSION-android
OUT=/out
KS=/keystore/$(basename "$KEYSTORE")

rm -rf /work && mkdir -p /work "$OUT"
tar -C /src --exclude=./build --exclude=./fyne-cross --exclude=./keystore --exclude=./.git -cf - . | tar -C /work -xf -
cd /work/cmd/multi2fa
sed -e "s/@VERSION@/$VERSION/" -e "s/@BUILD@/$BUILD/" /work/packaging/android/AndroidManifest.xml > AndroidManifest.xml

echo "==> building the signed App Bundle (all ABIs, release manifest)"
"$FYNE" release -os android -app-id "$APP_ID" -app-version "$VERSION" -app-build "$BUILD" \
    -icon /work/assets/icon.png -name "$NAME" \
    -keystore "$KS" -keystore-pass "$KEYSTORE_PASS" -key-name "$KEY_ALIAS" -key-pass "$KEY_PASS" \
    >/tmp/fyne.log 2>&1 || { grep -v "adding:\|inflating:\|extracting:\|creating:" /tmp/fyne.log | tail -20; exit 1; }
AAB="$OUT/$FILE.aab"
mv -f "$NAME.aab" "$AAB"
B=/tmp/bundle
rm -rf "$B" && mkdir -p "$B"
jarsigner -verify "$AAB" | grep -m1 "jar verified"

echo "==> universal APK"
bundletool build-apks --bundle="$AAB" --output="$B/app.apks" --mode=universal --overwrite \
    --ks="$KS" --ks-pass="pass:$KEYSTORE_PASS" --ks-key-alias="$KEY_ALIAS" --key-pass="pass:$KEY_PASS"
unzip -q -o "$B/app.apks" universal.apk -d "$B"
UNI="$OUT/$FILE-universal.apk"
cp "$B/universal.apk" "$UNI"

echo "==> arm64-v8a APK"
ARM="$OUT/$FILE-arm64-v8a.apk"
cp "$B/universal.apk" "$B/arm64.apk"
zip -q -d "$B/arm64.apk" 'lib/armeabi-v7a/*' 'lib/x86/*' 'lib/x86_64/*' 'META-INF/*'
"$BT/zipalign" -f -p 4 "$B/arm64.apk" "$ARM"
"$BT/apksigner" sign --ks "$KS" --ks-pass "pass:$KEYSTORE_PASS" --ks-key-alias "$KEY_ALIAS" --key-pass "pass:$KEY_PASS" "$ARM"

for apk in "$UNI" "$ARM"; do
    "$BT/apksigner" verify "$apk"
    echo "$(basename "$apk"): $(unzip -l "$apk" | grep -o 'lib/[^/]*/' | sort -u | tr -d '\n' | sed 's|lib/||g; s|/| |g')"
done
"$BT/aapt2" dump badging "$UNI" | grep -E "^package:|^sdkVersion|^targetSdkVersion|^uses-permission|^application-label:" | sed 's/^/  /'
"$BT/aapt2" dump xmltree --file AndroidManifest.xml "$UNI" | grep -E "allowBackup|debuggable" | sed 's/^ */  /'
if "$BT/aapt2" dump badging "$UNI" | grep -q "application-debuggable"; then
    echo "error: release APK is debuggable" >&2
    exit 1
fi
rm -f "$OUT"/*.idsig
if [ -n "${HOST_UID:-}" ]; then chown "$HOST_UID:${HOST_GID:-$HOST_UID}" "$AAB" "$UNI" "$ARM"; fi
ls -la "$AAB" "$UNI" "$ARM"
