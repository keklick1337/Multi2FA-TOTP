# Build environment for .rpm packages on openSUSE.
# CROSS=aarch64: the target's packages are downloaded by zypper and unpacked into /sysroot with
# rpm --noscripts (their scriptlets cannot run here); cgo then uses clang + lld against it.
# Absolute symlinks are re-pointed into /sysroot, otherwise the linker would follow them to host files.
ARG BASE=opensuse/leap:15.6
FROM ${BASE}
ARG GO_VERSION=1.26.8
ARG CROSS=
RUN zypper -n install --no-recommends gcc make tar xz gzip findutils file rpm-build pkg-config curl \
        Mesa-libGL-devel Mesa-libEGL-devel libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel \
        libXxf86vm-devel libxkbcommon-devel wayland-devel wayland-protocols-devel \
    && zypper clean -a
RUN if [ -z "$CROSS" ]; then exit 0; fi; set -e; \
    zypper -n install --no-recommends clang lld; \
    . /etc/os-release; \
    zypper -n --root /sysroot ar -f "http://download.opensuse.org/distribution/leap/$VERSION_ID/repo/oss/" oss; \
    zypper -n --root /sysroot ar -f "http://download.opensuse.org/update/leap/$VERSION_ID/oss/" update; \
    printf '[main]\narch = %s\n' "$CROSS" > /tmp/zypp.conf; \
    ZYPP_CONF=/tmp/zypp.conf zypper -n --root /sysroot --gpg-auto-import-keys install --download-only --no-recommends \
        glibc-devel gcc Mesa-libGL-devel Mesa-libEGL-devel libX11-devel libXcursor-devel libXrandr-devel \
        libXinerama-devel libXi-devel libXxf86vm-devel libxkbcommon-devel wayland-devel; \
    rpm --root /sysroot -i --nodeps --noscripts --ignorearch --excludedocs \
        $(find /sysroot/var/cache/zypp/packages -name '*.rpm') >/dev/null 2>&1 || true; \
    for l in $(find /sysroot -type l -lname '/*'); do ln -sfn "/sysroot$(readlink "$l")" "$l"; done; \
    test -e /sysroot/usr/lib64/libX11.so; \
    rm -rf /sysroot/var/cache /tmp/zypp.conf; zypper clean -a; \
    echo "export GOARCH=arm64 CC='clang --target=$CROSS-suse-linux --sysroot=/sysroot -fuse-ld=lld -Wno-unused-command-line-argument' \
PKG_CONFIG_SYSROOT_DIR=/sysroot PKG_CONFIG_LIBDIR=/sysroot/usr/lib64/pkgconfig:/sysroot/usr/share/pkgconfig PKG_ARCH=$CROSS" \
        > /etc/cross.env
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-buildvcs=false
