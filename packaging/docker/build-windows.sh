#!/bin/sh
# Windows builds for amd64 (mingw-w64) and arm64 (llvm-mingw): the app with icon, manifest and
# version info plus multi2fa-cli, as a portable .zip and an NSIS installer (-setup.exe).
. /src/packaging/docker/common.sh
prepare_src
BUILD=${BUILD:-1}

for arch in ${WIN_ARCHES:-amd64 arm64}; do
    make windows OUT=/tmp/out WIN_ARCH="$arch" BUILD="$BUILD" >/dev/null
    name=Multi2FA-TOTP-$VERSION-windows-$arch
    cp "/tmp/out/windows/$name.zip" /out/
    makensis -V2 -DVERSION="$VERSION" -DBUILD="$BUILD" -DARCH="$arch" -DSRC="/tmp/out/windows/$name" \
        -DROOT="$WORK" -DOUTFILE="/out/$name-setup.exe" packaging/windows/installer.nsi
    fix_owner "/out/$name.zip" "/out/$name-setup.exe"
    echo "built /out/$name.zip"
    echo "built /out/$name-setup.exe"
done
