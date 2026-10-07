#!/bin/sh
# Builds multi2fa-totp_<ver>-1~deb<N>_<arch>.deb inside a Debian container (native or cross).
. /src/packaging/docker/common.sh
prepare_src

DEB_MAJOR=$(cut -d. -f1 /etc/debian_version)
# Debian 11 ships libwayland 1.18, too old for GLFW 3.4's Wayland backend.
if [ "$DEB_MAJOR" -le 11 ]; then GOTAGS=x11; fi
ARCH=${PKG_ARCH:-$(dpkg --print-architecture)}
DEB_VERSION="$VERSION-1~deb$DEB_MAJOR"
STAGE=/tmp/stage
rm -rf "$STAGE"
stage_fhs "$STAGE" /usr/share/doc/$PKG_NAME /usr/share/doc/$PKG_NAME/copyright

# dpkg-shlibdeps needs a debian/control next to it to compute library dependencies;
# dpkg-architecture points it at the target's libraries and objdump.
mkdir -p /tmp/shlib/debian && cd /tmp/shlib
printf 'Source: %s\n\nPackage: %s\nArchitecture: any\n' "$PKG_NAME" "$PKG_NAME" > debian/control
DEPENDS=$(dpkg-architecture -a"$ARCH" -c dpkg-shlibdeps -O -e "$STAGE/usr/bin/multi2fa" 2>/dev/null | sed -n 's/^shlibs:Depends=//p')
test -n "$DEPENDS" || { echo "dpkg-shlibdeps found no dependencies"; exit 1; }
cd "$WORK"

SIZE=$(du -sk "$STAGE" | cut -f1)
mkdir -p "$STAGE/DEBIAN"
cat > "$STAGE/DEBIAN/control" <<CONTROL
Package: $PKG_NAME
Version: $DEB_VERSION
Architecture: $ARCH
Maintainer: keklick1337 <keklick1337@gmail.com>
Installed-Size: $SIZE
Depends: $DEPENDS
Recommends: ffmpeg
Conflicts: multi2fa-cli
Replaces: multi2fa-cli
Section: utils
Priority: optional
Homepage: https://github.com/keklick1337/Multi2FA-TOTP
Description: Encrypted TOTP/HOTP two-factor authenticator
 Desktop authenticator for two-factor codes. Accounts are stored in password
 protected vaults encrypted with Argon2id and XChaCha20-Poly1305. Supports QR
 scanning from camera, screen, images and clipboard, Google Authenticator
 exports, several vaults, auto-lock and encrypted backups. Includes
 multi2fa-cli for reading codes and managing vaults from a terminal.
CONTROL

OUT_FILE="/out/${PKG_NAME}_${DEB_VERSION}_${ARCH}.deb"
dpkg-deb --root-owner-group -Zxz --build "$STAGE" "$OUT_FILE" >/dev/null
fix_owner "$OUT_FILE"
echo "built $OUT_FILE"
echo "  $(file -b "$STAGE/usr/bin/multi2fa" | cut -d, -f1-2)"
echo "  Depends: $DEPENDS"
