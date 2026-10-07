# Packages a binary staged by packaging/docker/build-rpm.sh.
%global debug_package %{nil}
%global _build_id_links none
# The binary is stripped by go build; the host strip cannot read cross compiled (aarch64) files.
%global __os_install_post %{nil}
%global app_id io.github.keklick1337.multi2fa

Name:           multi2fa-totp
Version:        %{pkgversion}
Release:        1%{?dist}
Summary:        Encrypted TOTP/HOTP two-factor authenticator
License:        MIT
URL:            https://github.com/keklick1337/Multi2FA-TOTP
Recommends:     ffmpeg
# The standalone CLI package ships the same /usr/bin/multi2fa-cli.
Conflicts:      multi2fa-cli

%description
Desktop authenticator for two-factor codes. Accounts are stored in password
protected vaults encrypted with Argon2id and XChaCha20-Poly1305. Supports QR
scanning from camera, screen, images and clipboard, Google Authenticator
exports, several vaults, auto-lock and encrypted backups. Includes
multi2fa-cli for reading codes and managing vaults from a terminal.

%install
cp -a %{stagedir}/. %{buildroot}/

%files
%{_bindir}/multi2fa
%{_bindir}/multi2fa-cli
%{_datadir}/applications/%{app_id}.desktop
%{_datadir}/metainfo/%{app_id}.metainfo.xml
%{_datadir}/mime/packages/%{app_id}.xml
%{_datadir}/icons/hicolor/*/apps/multi2fa.png
%dir %{_datadir}/doc/%{name}
%{_datadir}/doc/%{name}/README.md
%{_datadir}/doc/%{name}/CHANGELOG.md
%dir %{_datadir}/licenses/%{name}
%license %{_datadir}/licenses/%{name}/LICENSE

%changelog
* Wed Oct 07 2026 keklick1337 <keklick1337@gmail.com> - 1.0.0-1
- First release
