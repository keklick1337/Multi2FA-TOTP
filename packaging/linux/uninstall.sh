#!/bin/sh
# Removes what install.sh installed. Vaults in ~/.config/Multi2FA are kept.
set -e

APP_ID=io.github.keklick1337.multi2fa
if [ -z "$PREFIX" ]; then
    if [ "$(id -u)" = 0 ]; then PREFIX=/usr/local; else PREFIX="$HOME/.local"; fi
fi

rm -f "$PREFIX/bin/multi2fa" "$PREFIX/bin/multi2fa-cli" "$PREFIX/share/applications/$APP_ID.desktop"
rm -f "$PREFIX"/share/icons/hicolor/*/apps/multi2fa.png "$PREFIX/share/mime/packages/$APP_ID.xml"
command -v update-mime-database >/dev/null 2>&1 && update-mime-database "$PREFIX/share/mime" >/dev/null 2>&1 || true

command -v gtk-update-icon-cache >/dev/null 2>&1 && gtk-update-icon-cache -q -t -f "$PREFIX/share/icons/hicolor" 2>/dev/null || true
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database -q "$PREFIX/share/applications" 2>/dev/null || true

echo "Multi2FA TOTP removed from $PREFIX (your vaults were not touched)."
