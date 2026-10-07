# Windows cross build: mingw-w64 (amd64), llvm-mingw (arm64) and NSIS for the installers.
FROM debian:12
ARG GO_VERSION=1.26.8
ARG LLVM_MINGW=20261006
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl gcc-mingw-w64-x86-64 make zip xz-utils nsis \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://github.com/mstorsjo/llvm-mingw/releases/download/${LLVM_MINGW}/llvm-mingw-${LLVM_MINGW}-ucrt-ubuntu-22.04-x86_64.tar.xz" \
        | tar -C /opt -xJ && mv /opt/llvm-mingw-* /opt/llvm-mingw
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xz
# llvm-mingw goes last in PATH so the Debian mingw gcc keeps handling amd64.
ENV PATH=/usr/local/go/bin:$PATH:/opt/llvm-mingw/bin GOTOOLCHAIN=local GOFLAGS=-buildvcs=false
