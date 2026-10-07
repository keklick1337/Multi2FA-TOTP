# Security policy

Multi2FA TOTP stores second factors, so security reports get priority over everything else.

## Supported versions

Only the latest release receives fixes. Please check that the problem still exists there before reporting.

## Reporting a vulnerability

Please do **not** open a public issue. Report privately through
[GitHub security advisories](https://github.com/keklick1337/Multi2FA-TOTP/security/advisories/new)
or by email to keklick1337@gmail.com.

Include what you can of:

- the version (`multi2fa -version` or `multi2fa-cli version`) and the operating system,
- the steps or a proof of concept,
- the impact you expect (for example: secrets readable without the password, a crash while parsing a file).

You will get an answer within a few days. Once a fix is released the advisory is published with credit to you,
unless you prefer to stay anonymous.

## Scope

In scope:

- the vault and backup formats (`.m2fa`, `.m2fab`), key derivation and encryption,
- secrets leaking into plain memory, swap, logs, crash dumps or the clipboard beyond the documented clearing,
- parsing of `otpauth://` and `otpauth-migration://` links, QR images and vault files,
- the desktop app, the CLI and the release packages.

Out of scope, as described in the README: an attacker with root or administrator rights or kernel access on the
unlocked device, and text that the user deliberately reveals or copies.

## Design summary

Vaults use Argon2id, scrypt or PBKDF2-HMAC-SHA512 with a KeePass style composite key (password plus optional key file)
and XChaCha20-Poly1305, AES-256-GCM or both in cascade with HKDF subkeys. The header is authenticated and the data
padded. While unlocked the master key lives in memguard enclaves and every OTP secret stays sealed with a session key;
it is opened into locked memory only while a code is computed. See the Security section of the README for details.
