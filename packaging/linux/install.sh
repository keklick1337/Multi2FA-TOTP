#!/bin/sh
# Installs Multi2FA TOTP with its menu entry and icons.
#   ./install.sh            per-user install into ~/.local
#   sudo ./install.sh       system-wide install into /usr/local
#   PREFIX=/opt/x ./install.sh
set -e

APP_ID=io.github.keklick1337.multi2fa
HERE=$(cd "$(dirname "$0")" && pwd)

if [ -z "$PREFIX" ]; then
    if [ "$(id -u)" = 0 ]; then PREFIX=/usr/local; else PREFIX="$HOME/.local"; fi
fi
BIN="$PREFIX/bin"
SHARE="$PREFIX/share"

mkdir -p "$BIN" "$SHARE/applications"
install -m 755 "$HERE/multi2fa" "$BIN/multi2fa"
if [ -f "$HERE/multi2fa-cli" ]; then install -m 755 "$HERE/multi2fa-cli" "$BIN/multi2fa-cli"; fi

for dir in "$HERE"/icons/hicolor/*/apps; do
    size=$(basename "$(dirname "$dir")")
    mkdir -p "$SHARE/icons/hicolor/$size/apps"
    install -m 644 "$dir/multi2fa.png" "$SHARE/icons/hicolor/$size/apps/multi2fa.png"
done

sed -e "s|^Exec=.*|Exec=\"$BIN/multi2fa\" %f|" "$HERE/$APP_ID.desktop" > "$SHARE/applications/$APP_ID.desktop"
chmod 644 "$SHARE/applications/$APP_ID.desktop"

mkdir -p "$SHARE/mime/packages"
install -m 644 "$HERE/$APP_ID.xml" "$SHARE/mime/packages/$APP_ID.xml"
command -v update-mime-database >/dev/null 2>&1 && update-mime-database "$SHARE/mime" >/dev/null 2>&1 || true
command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f "$SHARE/icons/hicolor" 2>/dev/null || true
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q "$SHARE/applications" 2>/dev/null || true
command -v kbuildsycoca6 >/dev/null 2>&1 && kbuildsycoca6 --noincremental >/dev/null 2>&1 || true
command -v kbuildsycoca5 >/dev/null 2>&1 && kbuildsycoca5 --noincremental >/dev/null 2>&1 || true

echo "Multi2FA TOTP installed:"
echo "  binary:     $BIN/multi2fa (+ multi2fa-cli)"
echo "  menu entry: $SHARE/applications/$APP_ID.desktop"
case ":$PATH:" in *":$BIN:"*) ;; *) echo "Note: $BIN is not in PATH, the menu entry works anyway." ;; esac
