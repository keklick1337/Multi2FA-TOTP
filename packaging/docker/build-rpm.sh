#!/bin/sh
# Builds multi2fa-totp-<ver>-1.<dist>.<arch>.rpm inside an RPM based container (native or cross).
. /src/packaging/docker/common.sh
prepare_src

DIST=$(rpm --eval '%{?dist}')
if [ -z "$DIST" ]; then
    DIST=$(. /etc/os-release && case "$ID" in
        opensuse-leap) echo ".lp$(echo "$VERSION_ID" | tr -d .)" ;;
        *) echo ".$ID$(echo "$VERSION_ID" | tr -d .)" ;;
    esac)
fi

STAGE=/tmp/stage
rm -rf "$STAGE" /tmp/rpm
stage_fhs "$STAGE" /usr/share/doc/$PKG_NAME /usr/share/licenses/$PKG_NAME/LICENSE

rpmbuild -bb --target "${PKG_ARCH:-$(uname -m)}" packaging/rpm/$PKG_NAME.spec \
    --define "_topdir /tmp/rpm" \
    --define "stagedir $STAGE" \
    --define "pkgversion $VERSION" \
    --define "dist $DIST" >/tmp/rpmbuild.log 2>&1 || { cat /tmp/rpmbuild.log; exit 1; }

for f in /tmp/rpm/RPMS/*/*.rpm; do
    cp "$f" /out/
    fix_owner "/out/$(basename "$f")"
    echo "built /out/$(basename "$f")"
    echo "  $(file -b "$STAGE/usr/bin/multi2fa" | cut -d, -f1-2)"
    rpm -qp --requires "$f" | grep -v '^rpmlib' | tr '\n' ' ' | sed 's/^/  Requires: /'; echo
done
