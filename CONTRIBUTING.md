# Contributing

Thanks for helping! Bug reports, translations, packaging fixes and code are all welcome.

## Reporting bugs

Open an [issue](https://github.com/keklick1337/Multi2FA-TOTP/issues/new/choose) with the version, the operating system
(and desktop environment on Linux), what you did, what you expected and what happened. Never attach a real vault or
real secrets. Security problems go through [SECURITY.md](SECURITY.md) instead.

## Development setup

You need Go 1.26+ and a C compiler; on Linux also the OpenGL and X11/Wayland headers:

```sh
# Debian / Ubuntu
sudo apt install gcc pkg-config libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev wayland-protocols
# Fedora
sudo dnf install gcc mesa-libGL-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel \
    libXxf86vm-devel libxkbcommon-devel wayland-devel
```

```sh
make run          # build and start the app
make cli          # build/multi2fa-cli
make check        # go vet + all tests
```

Use a test vault while developing: `./build/multi2fa -vault /tmp/test.m2fa`.

## Pull requests

- Keep changes focused; one topic per pull request.
- `make check` must pass and `gofmt -l .` must print nothing.
- Follow the surrounding style: short English comments that explain why, not what.
- Changes to the vault format need tests that open files written by the previous version.
- User visible text goes through `internal/i18n`: add the key to `en.go` and to every other language
  (`TestCatalogsComplete` checks it). Counted strings use `i18n.N` with `key|one` / `key|few` variants.
- Update `CHANGELOG.md` under an "Unreleased" heading.

## Translations

Each language is one file in `internal/i18n` (`en.go` is the reference). To add a language, copy `en.go`, translate the
values, add the language to `Languages` and `catalogs` in `i18n.go`, and add plural rules to `pluralForm` if it needs
more than one/other.

## Releases

Maintainers bump the version with `make bump-patch|bump-minor|bump-major`, update `CHANGELOG.md` and push a `v<version>`
tag; the release workflow builds and publishes every package. `make release` reproduces the Linux, Windows, CLI and
Android parts locally.
