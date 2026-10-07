#!/bin/sh
# multi2fa-cli for every platform. The CLI is pure Go, so everything is cross compiled here:
#   Multi2FA-TOTP-cli-<ver>-<os>-<arch>.tar.gz   multi2fa-cli (.exe on Windows), README.md, LICENSE
#   multi2fa-cli_<ver>-1_<arch>.deb, multi2fa-cli-<ver>-1.<arch>.rpm, multi2fa-cli-<ver>-1-<arch>.pkg.tar.zst (Arch)
. /src/packaging/docker/common.sh
prepare_src

NFPM_VERSION=v2.47.0
TARGETS=${CLI_TARGETS:-linux/amd64 linux/arm64 windows/amd64 darwin/arm64}
# Linux packages per format, by archive arch name.
DEB_ARCHES="amd64 386 arm64 armv7 riscv64 ppc64le s390x loong64 mips64le"
RPM_ARCHES="amd64 386 arm64 armv7 riscv64 ppc64le s390x loong64"
ARCHLINUX_ARCHES="amd64 arm64 armv7"

GOBIN=/tmp/bin go install "github.com/goreleaser/nfpm/v2/cmd/nfpm@$NFPM_VERSION"
BUILT=/tmp/cli
rm -rf "$BUILT" && mkdir -p "$BUILT"

for t in $TARGETS; do
    os=${t%/*} arch=${t#*/}
    goarch=$arch goarm= gomips=
    case "$arch" in
        armv7) goarch=arm goarm=7 ;;
        armv6) goarch=arm goarm=6 ;;
        # Software floating point: runs on routers without an FPU as well.
        mips|mipsle) gomips=softfloat ;;
    esac
    exe=multi2fa-cli
    [ "$os" = windows ] && exe=multi2fa-cli.exe
    dir=$BUILT/$os-$arch
    mkdir -p "$dir"
    CGO_ENABLED=0 GOOS=$os GOARCH=$goarch GOARM=$goarm GOMIPS=$gomips go build -trimpath \
        -ldflags "-s -w -X main.version=$VERSION" -o "$dir/$exe" ./cmd/multi2fa-cli
    cp LICENSE "$dir/"
    cp docs/cli.md "$dir/README.md"
    file=/out/Multi2FA-TOTP-cli-$VERSION-$os-$arch.tar.gz
    tar -C "$dir" --owner=0 --group=0 --numeric-owner -czf "$file" "$exe" README.md LICENSE
    fix_owner "$file"
    echo "built $file"
done

# package FORMAT ARCH: nfpm names the file after the format's own architecture names.
package() {
    bin=$BUILT/linux-$2/multi2fa-cli
    if [ ! -f "$bin" ]; then
        echo "skip $1 $2: linux/$2 is not in CLI_TARGETS"
        return
    fi
    case "$2" in
        armv7) pkgarch=arm7 ;;
        armv6) pkgarch=arm6 ;;
        *) pkgarch=$2 ;;
    esac
    sed -e "s|\${PKG_ARCH}|$pkgarch|g" -e "s|\${VERSION}|$VERSION|g" -e "s|\${BIN}|$bin|g" \
        packaging/cli/nfpm.yaml > /tmp/nfpm.yaml
    out=$(/tmp/bin/nfpm package -f /tmp/nfpm.yaml -p "$1" -t /out/ 2>&1) || { echo "$out"; exit 1; }
    file=$(echo "$out" | sed -n 's/.*created package: //p')
    fix_owner "$file"
    echo "built $file"
}

for a in $DEB_ARCHES; do package deb "$a"; done
for a in $RPM_ARCHES; do package rpm "$a"; done
for a in $ARCHLINUX_ARCHES; do package archlinux "$a"; done
