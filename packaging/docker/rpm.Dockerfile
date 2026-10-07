# Build environment for .rpm packages on RHEL-like systems (AlmaLinux, Rocky, Fedora).
# CROSS=aarch64 installs the target's libraries into /sysroot (no scriptlets, nothing is executed)
# and compiles cgo with clang + lld against it; Go itself runs natively. filesystem goes first so
# the base directories and symlinks (/lib64 -> usr/lib64) exist before anything else unpacks.
ARG BASE=almalinux:9
FROM ${BASE}
ARG GO_VERSION=1.26.8
ARG CROSS=
RUN dnf install -y dnf-plugins-core || true; \
    dnf config-manager --set-enabled crb 2>/dev/null || dnf config-manager --set-enabled powertools 2>/dev/null \
        || dnf config-manager setopt crb.enabled=1 2>/dev/null || true; \
    dnf install -y gcc make tar xz gzip findutils file rpm-build pkgconf-pkg-config \
        mesa-libGL-devel libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel \
        libXxf86vm-devel libxkbcommon-devel wayland-devel wayland-protocols-devel \
    && (command -v curl || dnf install -y curl) && dnf clean all
RUN if [ -z "$CROSS" ]; then exit 0; fi; set -e; \
    dnf install -y clang lld; \
    rel=$(rpm -E '%{?rhel}%{?fedora}'); \
    host=""; if dnf --version 2>/dev/null | grep -q dnf5; then host=--use-host-config; fi; \
    case "$(rpm -E '%{?rhel}')" in 8) host="$host --enablerepo=powertools" ;; 9|10) host="$host --enablerepo=crb" ;; esac; \
    root="dnf -y $host --forcearch=$CROSS --installroot=/sysroot --releasever=$rel \
        --setopt=install_weak_deps=False --setopt=tsflags=noscripts --nodocs"; \
    $root install filesystem; \
    $root install glibc-devel gcc mesa-libGL-devel libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel \
        libXi-devel libXxf86vm-devel libxkbcommon-devel wayland-devel; \
    dnf clean all; rm -rf /sysroot/var/cache; \
    echo "export GOARCH=arm64 CC='clang --target=$CROSS-redhat-linux --sysroot=/sysroot -fuse-ld=lld -Wno-unused-command-line-argument' \
PKG_CONFIG_SYSROOT_DIR=/sysroot PKG_CONFIG_LIBDIR=/sysroot/usr/lib64/pkgconfig:/sysroot/usr/share/pkgconfig PKG_ARCH=$CROSS" \
        > /etc/cross.env
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-buildvcs=false
