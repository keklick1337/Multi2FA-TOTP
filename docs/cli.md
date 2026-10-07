# multi2fa-cli

Command line companion of [Multi2FA TOTP](https://github.com/keklick1337/Multi2FA-TOTP). It opens the same encrypted
vaults as the desktop app (`.m2fa`, backups `.m2fab`) and prints codes, lists, adds and removes accounts, exports and
backs up. It is a single static binary without dependencies for Linux, Windows, macOS, the BSDs and more.

## Installation

The desktop packages of Multi2FA TOTP already include `multi2fa-cli`. Without the desktop app, take the package for
your system from the [latest release](https://github.com/keklick1337/Multi2FA-TOTP/releases/latest):

| System | Install |
| --- | --- |
| Debian, Ubuntu, Mint, Raspberry Pi OS | `sudo apt install ./multi2fa-cli_<v>-1_<arch>.deb` |
| Fedora, RHEL, AlmaLinux, Rocky, openSUSE | `sudo dnf install ./multi2fa-cli-<v>-1.<arch>.rpm` (`zypper install` on openSUSE) |
| Arch Linux | `sudo pacman -U ./multi2fa-cli-<v>-1-<arch>.pkg.tar.zst` |
| Anything else | unpack `Multi2FA-TOTP-cli-<v>-<os>-<arch>.tar.gz` and put `multi2fa-cli` into your `PATH` |

Archives exist for Linux (amd64, 386, arm64, armv7, armv6, riscv64, ppc64le, ppc64, s390x, loong64, mips, mipsle,
mips64, mips64le), Windows (amd64, 386, arm64), macOS (amd64, arm64), FreeBSD, OpenBSD and NetBSD (amd64, 386, arm64,
armv7; OpenBSD also ppc64 and riscv64), illumos, Solaris and AIX. Each one holds the binary,
this manual and the license. Windows 10 and later unpack `.tar.gz` with `tar -xf`.

## Quick start

```sh
multi2fa-cli codes                 # every account with its current code and the seconds left
multi2fa-cli code github           # just the code, for scripts: 492039
multi2fa-cli code github -copy     # copy it to the clipboard instead
multi2fa-cli codes -watch          # live view, refreshed every second (Ctrl+C to quit)
```

Without `-vault` the CLI opens the app's default vault (`$MULTI2FA_VAULT` overrides it):

| OS | Default vault |
| --- | --- |
| Linux, BSD | `~/.config/Multi2FA/vault.m2fa` |
| macOS | `~/Library/Application Support/Multi2FA/vault.m2fa` |
| Windows | `%AppData%\Multi2FA\vault.m2fa` |

## Choosing accounts

`code`, `remove` and `export` take accounts in any of these forms:

| Form | Example | Meaning |
| --- | --- | --- |
| number | `3` or `#3` | position in `list` (favorites first, then by issuer and account) |
| id | `bfd7f16f` | the id from `list` or any unique prefix of at least 4 characters |
| query | `github`, `"work alice"` | words matched against issuer, account, notes, tags and folder |

A query that matches several accounts prints them with their numbers so you can pick one. An exact issuer
(`google`) or `Issuer:account` label wins over partial matches.

## Commands

| Command | What it does |
| --- | --- |
| `list [query]` | number, short id, issuer, account, folder and type of each account |
| `code <account>...` | only the current code(s), one per line; `-copy` puts them on the clipboard; `-next` advances a HOTP counter first |
| `codes [query]` | table of codes with the time left; `-watch` keeps it updating |
| `add <link or image>...` | add from `otpauth://` links, Google Authenticator `otpauth-migration://` exports or QR codes in image files (PNG, JPEG, GIF, BMP, WebP); `-folder Work/Servers` files them into a folder (created when missing) |
| `add -issuer X -account Y` | manual entry; the secret is asked for without echo. Options: `-hotp`, `-digits`, `-period`, `-algorithm SHA1/SHA256/SHA512`, `-counter` |
| `remove <account>...` | delete accounts after a confirmation (`-yes` skips it) |
| `export [query]` | print `otpauth://` links; `-qr` draws QR codes in the terminal, `-google` makes Google Authenticator transfer links |
| `backup <file.m2fab>` | encrypted backup with its own password (same encryption settings as the vault) |
| `import <file>` | merge a backup or another vault into this one; duplicates are skipped and folders kept |
| `passwd` | change the master password (cipher and key derivation stay the same) |
| `info` | encryption settings from the file header; works without the password |
| `version` | print the version |

Every command accepts `-json` for machine readable output and `-h` for its options. Flags may come before or after
the command and its arguments.

## Password and key file

The master password is taken from the first source that is set:

1. `-password-file FILE`: the first line of a file (keep it `chmod 600`).
2. `-password-stdin`: the first line of standard input, e.g. from a password manager:
   `pass show multi2fa | multi2fa-cli -password-stdin code github`.
3. `-password PASS`: convenient but visible to other users in the process list and saved in shell history.
4. `$MULTI2FA_PASSWORD`.
5. Otherwise it is asked for on the terminal without echo, even when stdin and stdout are redirected.

Vaults protected with a key file need `-key-file FILE` or `$MULTI2FA_KEY_FILE`.

## Clipboard

`-copy` uses `clip.exe` on Windows, `pbcopy` on macOS and `wl-copy`, `xclip` or `xsel` on Linux and BSD
(`termux-clipboard-set` on Termux). Unlike the desktop app the CLI does not clear the clipboard afterwards.

## Exit status

`0` on success, `1` when a command fails (wrong password, no matching account and so on), `2` for usage errors.

## Security notes

The CLI uses the same protection as the app: the master key lives in memguard enclaves, OTP secrets stay
encrypted in memory and are only decrypted into locked memory while a code is computed, core dumps are disabled
and on Linux the process cannot be traced by other processes of the same user. `export` and `backup` reveal or copy
secrets, so treat their output like the vault itself.
