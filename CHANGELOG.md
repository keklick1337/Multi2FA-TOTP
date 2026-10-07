# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/), versions follow [SemVer](https://semver.org/).
The current version lives in the `VERSION` file (`make bump-patch|bump-minor|bump-major`).

## [1.0.0] - 2026-10-07

First release.

### Features

- TOTP/HOTP codes (RFC 6238/4226), `otpauth://` links and Google Authenticator `otpauth-migration://` exports.
- Add accounts from a camera (via ffmpeg), the screen, an image file, the clipboard (Ctrl+V) or by hand.
- Several vaults open at the same time; copy, move and merge accounts between them.
- Folders with any nesting depth, breadcrumbs, drag and drop of accounts and folders.
- Selection mode for bulk move, copy, export to a new vault and delete.
- Fast search across all folders, favorites, tags and notes.
- Auto-lock, lock on focus loss, clipboard clearing, codes hidden until clicked.
- Encrypted backups, transfer QR codes for Google Authenticator, vault reset when the password is lost.
- Navigation rail on wide windows, bottom bar on narrow ones, light/dark mode, 6 languages.
- Theme manager: 9 built-in color themes, custom themes with live preview, JSON import/export.
- Native file dialogs, "Show in folder", "Locate…" for moved vaults, `.m2fa`/`.m2fab` file association,
  opening a vault from the command line, system tray icon.
- `multi2fa-cli`: list accounts, print one code or all of them (with a live `-watch` view), copy to the clipboard,
  add from links, QR images or by hand, remove, export links and terminal QR codes, backups, import, password
  change, JSON output; the password from a hidden prompt, stdin, a file, an argument or the environment.

### Security

- Selectable key derivation (Argon2id, scrypt, PBKDF2-SHA512) with calibrated presets or custom parameters.
- Selectable encryption (XChaCha20-Poly1305, AES-256-GCM, AES + XChaCha cascade) with HKDF subkeys; the cipher
  is not stored in the file and is detected on open. Authenticated header.
- Optional key file (KeePass style composite key).
- Master key in memguard enclaves; OTP secrets stay encrypted in memory and are decrypted into
  locked memory only while a code is computed; passwords are typed into locked memory.
- Process hardening: not dumpable and no ptrace on Linux, debugger attachment denied on macOS,
  core dumps disabled. Locking destroys the keys and unloads all vaults.

### Packaging

- Linux tarball with desktop entry and installer and .deb packages (Debian 11/12/13) for amd64, arm64 and armhf.
- .rpm packages for EL 8/9/10, Fedora 42/43/44 and openSUSE Leap 15.6 on x86_64 and aarch64.
- Windows amd64 and arm64: installer (Start menu, uninstaller, optional desktop shortcut, file association and CLI
  on PATH) and portable zip, exe with icon and version info.
- macOS universal app, built on a Mac or from Linux with a macOS SDK (zig, ad-hoc signed).
- Android App Bundle for Google Play plus signed universal (arm64, armv7, x86_64, x86) and arm64 APKs, iOS app.
- Standalone `multi2fa-cli`: `.tar.gz` for 36 OS/CPU targets (Linux, Windows, macOS, the BSDs, illumos, Solaris,
  AIX) and `.deb`, `.rpm` and Arch Linux packages for 8 to 9 Linux CPUs.
- `make release` builds everything in Docker, cross compiling other CPUs, into one flat folder with `SHA256SUMS`.
