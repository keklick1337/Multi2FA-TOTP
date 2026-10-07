#!/bin/sh
# Portable Linux tarball. Built on the oldest supported glibc (Debian 11, glibc 2.31)
# so the binary runs on practically every current distribution.
. /src/packaging/docker/common.sh
prepare_src
ARCH=${PKG_ARCH:-$(dpkg --print-architecture)}
# X11 only: no libwayland dependency, works on Wayland desktops through XWayland.
make linux OUT=/tmp/out TAGS=x11 ARCH="$ARCH" >/dev/null
cp /tmp/out/linux/*.tar.gz /out/
for f in /tmp/out/linux/*.tar.gz; do
    fix_owner "/out/$(basename "$f")"
    echo "built /out/$(basename "$f")"
done
