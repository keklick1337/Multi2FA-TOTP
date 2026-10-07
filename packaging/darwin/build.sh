#!/bin/bash
# macOS build on Linux. Runs as root in the fyneio/fyne-cross-images:darwin image (zig as the C
# compiler) with a macOS SDK mounted at /sdk, sources at /src (read-only) and results in /out:
#   Multi2FA-TOTP-<v>-macos-universal.zip   "Multi2FA TOTP.app" for Intel and Apple Silicon,
#                                           multi2fa-cli inside Contents/MacOS, ad-hoc signed
set -euo pipefail

: "${VERSION:?}" "${BUILD:?}"
MIN_MACOS=11.0
RCODESIGN_VERSION=0.29.0
APP="Multi2FA TOTP.app"
export GOFLAGS=-buildvcs=false CGO_ENABLED=1 GOOS=darwin

test -f /sdk/usr/include/stdlib.h || { echo "no macOS SDK at /sdk"; exit 1; }
rm -rf /work /tmp/mac && mkdir -p /work /tmp/mac
tar -C /src --exclude=./build --exclude=./keystore --exclude=./macos-sdk --exclude=./.git -cf - . | tar -C /work -xf -
cd /work

for arch in amd64 arm64; do
    target=x86_64-macos.$MIN_MACOS
    [ "$arch" = arm64 ] && target=aarch64-macos.$MIN_MACOS
    echo "==> $arch"
    CC="zig cc -target $target -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CXX="zig c++ -target $target -isysroot /sdk -iwithsysroot /usr/include -iframeworkwithsysroot /System/Library/Frameworks" \
    CGO_LDFLAGS="--sysroot /sdk -F/System/Library/Frameworks -L/usr/lib" \
    GOARCH=$arch go build -buildmode=pie -trimpath -ldflags "-s -w" -o "/tmp/mac/multi2fa-$arch" ./cmd/multi2fa
    CGO_ENABLED=0 GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "/tmp/mac/multi2fa-cli-$arch" ./cmd/multi2fa-cli
done

# Universal binaries and the bundle.
MAKEFAT="go run github.com/randall77/makefat@v0.0.0-20260406194835-1b91746796b7"
B="/tmp/mac/$APP/Contents"
mkdir -p "$B/MacOS" "$B/Resources"
GOOS= CGO_ENABLED=0 $MAKEFAT "$B/MacOS/multi2fa" /tmp/mac/multi2fa-amd64 /tmp/mac/multi2fa-arm64
GOOS= CGO_ENABLED=0 $MAKEFAT "$B/MacOS/multi2fa-cli" /tmp/mac/multi2fa-cli-amd64 /tmp/mac/multi2fa-cli-arm64
chmod 755 "$B/MacOS/"*
sed -e "s/@VERSION@/$VERSION/" -e "s/@BUILD@/$BUILD/" packaging/darwin/Info.plist > "$B/Info.plist"
cp assets/icon.icns "$B/Resources/icon.icns"

# Ad-hoc signature for the whole bundle (Apple Silicon refuses unsigned code).
RCODESIGN=/go/bin/rcodesign-$RCODESIGN_VERSION
if [ ! -x "$RCODESIGN" ]; then
    mkdir -p /go/bin
    curl -fsSL "https://github.com/indygreg/apple-platform-rs/releases/download/apple-codesign/$RCODESIGN_VERSION/apple-codesign-$RCODESIGN_VERSION-x86_64-unknown-linux-musl.tar.gz" \
        | tar -xzO --wildcards '*/rcodesign' > "$RCODESIGN"
    chmod 755 "$RCODESIGN"
fi
"$RCODESIGN" sign "/tmp/mac/$APP" >/tmp/rcodesign.log 2>&1 || { cat /tmp/rcodesign.log; exit 1; }
# "rcodesign verify" expects a CMS blob, which ad-hoc signatures do not have; check the flags instead.
for bin in multi2fa multi2fa-cli; do
    n=$("$RCODESIGN" print-signature-info "$B/MacOS/$bin" | grep -c 'flags: CodeSignatureFlags(ADHOC)')
    [ "$n" -ge 2 ] || { echo "$bin: missing ad-hoc signature"; exit 1; }
done

ZIP="/out/Multi2FA-TOTP-$VERSION-macos-universal.zip"
rm -f "$ZIP"
(cd /tmp/mac && zip -qry "$ZIP" "$APP")
if [ -n "${HOST_UID:-}" ]; then chown "$HOST_UID:${HOST_GID:-$HOST_UID}" "$ZIP"; fi
echo "built $ZIP"
