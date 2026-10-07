# Build environment for .deb packages and the portable tarball.
# CROSS=arm64|armhf adds a Debian multiarch cross toolchain with the target's libraries;
# Go itself always runs natively, only cgo uses the cross compiler.
ARG BASE=debian:12
FROM ${BASE}
ARG GO_VERSION=1.26.8
ARG CROSS=
ENV DEBIAN_FRONTEND=noninteractive
# Debian 11 is end of life and its mirrors are being emptied: use the pinned snapshot the image ships.
RUN if grep -q '^11' /etc/debian_version && grep -q '^# deb http://snapshot' /etc/apt/sources.list; then \
        sed -i -e 's|^deb http://deb.debian.org.*||' \
               -e 's|^# deb http://snapshot|deb [check-valid-until=no] http://snapshot|' /etc/apt/sources.list; \
    fi
RUN libs="libgl1-mesa-dev libx11-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev \
          libxkbcommon-dev libwayland-dev"; \
    if [ -n "$CROSS" ]; then \
        dpkg --add-architecture "$CROSS"; \
        libs="crossbuild-essential-$CROSS $(for p in $libs; do printf '%s:%s ' "$p" "$CROSS"; done)"; \
    fi; \
    apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gcc libc6-dev pkg-config make xz-utils zip file dpkg-dev wayland-protocols $libs \
    && rm -rf /var/lib/apt/lists/*
# /etc/cross.env is read by common.sh before building.
RUN case "$CROSS" in \
        arm64) t=aarch64-linux-gnu; goarch="GOARCH=arm64" ;; \
        armhf) t=arm-linux-gnueabihf; goarch="GOARCH=arm GOARM=7" ;; \
        "") exit 0 ;; \
        *) echo "unsupported CROSS=$CROSS"; exit 1 ;; \
    esac; \
    echo "export $goarch CC=$t-gcc PKG_CONFIG_LIBDIR=/usr/lib/$t/pkgconfig:/usr/share/pkgconfig PKG_ARCH=$CROSS" > /etc/cross.env
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
ENV PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local GOFLAGS=-buildvcs=false
