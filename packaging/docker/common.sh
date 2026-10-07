#!/bin/sh
# Shared helpers for the container build scripts. Sources are mounted read-only at /src,
# results go straight to /out (owned by HOST_UID/HOST_GID at the end).
set -eu

# Cross images describe their target (GOARCH, CC, pkg-config paths, PKG_ARCH) here.
if [ -f /etc/cross.env ]; then . /etc/cross.env; fi

APP_ID=io.github.keklick1337.multi2fa
PKG_NAME=multi2fa-totp
WORK=/work

prepare_src() {
    rm -rf "$WORK" && mkdir -p "$WORK"
    tar -C /src --exclude=./build --exclude=./.git --exclude=./keystore --exclude=./macos-sdk --exclude=./fyne-cross -cf - . | tar -C "$WORK" -xf -
    cd "$WORK"
    VERSION=$(cat VERSION)
}

# GOTAGS=x11 builds without native Wayland (runs through XWayland); needed where libwayland < 1.20.
# GLFW does not use pkg-config; some distros (openSUSE) keep Wayland/xkbcommon headers in subdirectories.
build_binary() {
    extra=$(pkg-config --cflags wayland-client xkbcommon 2>/dev/null || true)
    CGO_CFLAGS="-O2 -g $extra" CGO_ENABLED=1 go build -trimpath -tags "${GOTAGS:-}" -ldflags "-s -w -X main.version=$VERSION" -o "$1" ./cmd/multi2fa
}

# The command line tool needs no cgo; GOARCH/GOARM from cross.env apply to it as well.
build_cli() {
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$1" ./cmd/multi2fa-cli
}

# stage_fhs DEST DOCDIR LICENSEFILE installs the app into DEST/usr like a distro package.
stage_fhs() {
    dest=$1 docdir=$2 license=$3
    build_binary "$dest/usr/bin/multi2fa"
    build_cli "$dest/usr/bin/multi2fa-cli"
    install -Dm644 packaging/linux/$APP_ID.desktop "$dest/usr/share/applications/$APP_ID.desktop"
    sed -i 's|^Exec=.*|Exec=multi2fa %f|' "$dest/usr/share/applications/$APP_ID.desktop"
    install -Dm644 packaging/linux/$APP_ID.xml "$dest/usr/share/mime/packages/$APP_ID.xml"
    install -Dm644 packaging/linux/$APP_ID.metainfo.xml "$dest/usr/share/metainfo/$APP_ID.metainfo.xml"
    for dir in assets/hicolor/*/apps; do
        size=$(basename "$(dirname "$dir")")
        install -Dm644 "$dir/multi2fa.png" "$dest/usr/share/icons/hicolor/$size/apps/multi2fa.png"
    done
    install -Dm644 README.md "$dest$docdir/README.md"
    install -Dm644 CHANGELOG.md "$dest$docdir/CHANGELOG.md"
    install -Dm644 LICENSE "$dest$license"
}

fix_owner() {
    if [ -n "${HOST_UID:-}" ]; then
        chown -R "$HOST_UID:${HOST_GID:-$HOST_UID}" "$@"
    fi
}
