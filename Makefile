# Multi2FA TOTP build system.
# Output layout:
#   build/linux/Multi2FA-TOTP-<ver>-linux-<arch>/    + .tar.gz
#   build/windows/Multi2FA-TOTP-<ver>-windows-amd64/ + .zip
#   build/darwin/Multi2FA TOTP.app                   + .zip/.dmg (built on a Mac)
#   build/release/                                   every release file, flat (make release, Docker)
#   build/android/ (Docker), build/ios/ (on a Mac)

NAME     := Multi2FA TOTP
BIN      := multi2fa
CLI      := multi2fa-cli
EXE      := Multi2FA-TOTP.exe
CLI_PKG  := ./cmd/multi2fa-cli
APP_ID   := io.github.keklick1337.multi2fa
VERSION  := $(shell cat VERSION)
BUILD    ?= 1
PKG      := ./cmd/multi2fa
OUT      := build
ARCH     ?= $(shell go env GOARCH)
LDFLAGS  := -s -w -X main.version=$(VERSION)
WINRES   := go run github.com/tc-hib/go-winres@v0.3.3
TAGS     ?=
GOBUILD  := CGO_ENABLED=1 go build -trimpath -tags "$(TAGS)"
# The CLI is pure Go: it cross compiles to any GOOS/GOARCH without a C toolchain.
CLIBUILD := CGO_ENABLED=0 go build -trimpath

GO_VERSION   ?= 1.26.8
FYNE         := go run fyne.io/tools/cmd/fyne@v1.7.3
MOBILE_NAME  := Multi2FA
IOS_CERT     ?= Apple Distribution
IOS_PROFILE  ?=

LINUX_DIR := $(OUT)/linux/Multi2FA-TOTP-$(VERSION)-linux-$(ARCH)
WIN_ARCH  ?= amd64
WIN_DIR   := $(OUT)/windows/Multi2FA-TOTP-$(VERSION)-windows-$(WIN_ARCH)
# C cross compilers for cgo when building Windows on Linux (arm64: llvm-mingw).
WIN_CC_amd64 := x86_64-w64-mingw32-gcc
WIN_CC_arm64 := aarch64-w64-mingw32-clang
MAC_DIR   := $(OUT)/darwin
MAC_APP   := $(MAC_DIR)/$(NAME).app

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	NATIVE := darwin
else
	NATIVE := linux
endif

.PHONY: all help build cli run test vet check linux windows winres darwin dist icons version bump-patch bump-minor bump-major clean \
	android android-dev android-release android-keystore ios ios-simulator ios-release \
	release release-portable release-deb release-rpm release-windows release-macos release-android release-cli checksums darwin-cross

all: check $(NATIVE)

help:
	@echo "Multi2FA TOTP $(VERSION) build targets"
	@echo ""
	@echo "Development"
	@echo "  make                 vet + tests + package for this OS"
	@echo "  make build           quick binary in build/$(BIN)"
	@echo "  make cli             command line tool in build/$(CLI) (pure Go, any GOOS/GOARCH)"
	@echo "  make run             build and start"
	@echo "  make check           vet + tests"
	@echo "  make icons           regenerate all icons from assets/icon.svg (ICON_SVG=<file> for another SVG)"
	@echo "                       (concepts preview: assets/concepts/index.html)"
	@echo ""
	@echo "Desktop packages"
	@echo "  make linux           build/linux/...: binary, .desktop, icons, install.sh + .tar.gz"
	@echo "  make windows         build/windows/...: .exe with icon and version info + .zip (mingw-w64 on Linux)"
	@echo "                       WIN_ARCH=arm64 for Windows on ARM (llvm-mingw on Linux)"
	@echo "  make darwin          build/darwin/$(NAME).app universal + .zip + .dmg (run on macOS)"
	@echo "  make darwin-cross    the same .app + .zip from Linux via Docker; needs a macOS SDK in macos-sdk/"
	@echo "  make dist            everything this machine can build natively"
	@echo ""
	@echo "Release in Docker (every file flat in build/release/, parts run in parallel: RELEASE_JOBS=$(RELEASE_JOBS))"
	@echo "  make release         vet + tests, then:"
	@echo "                       portable tar.gz        amd64, arm64, armhf (Debian 11 glibc, X11)"
	@echo "                       .deb Debian 11/12/13   amd64, arm64, armhf"
	@echo "                       .rpm EL 8/9/10, Fedora 42-44, openSUSE 15.6   x86_64, aarch64"
	@echo "                       Windows amd64 + arm64: portable .zip and -setup.exe installer,"
	@echo "                       multi2fa-cli: .tar.gz for 36 OS/CPU targets + .deb/.rpm/Arch packages,"
	@echo "                       Android .aab + APKs (release key with KEYSTORE_PASS=..., else development key),"
	@echo "                       macOS .app (universal) when a macOS SDK is in macos-sdk/ (see README), SHA256SUMS"
	@echo "  make release-portable | release-deb | release-rpm | release-windows | release-macos | release-cli | release-android"
	@echo "  single jobs:         make deb@debian_12@arm64, rpm@fedora_44@aarch64, portable@debian_11@armhf"
	@echo ""
	@echo "Mobile"
	@echo "  make android         installable test build signed with a local development key -> build/android/"
	@echo "                       Multi2FA-TOTP-<v>-android-universal.apk (arm64, armv7, x86_64, x86), -arm64-v8a.apk, .aab"
	@echo "  make android-keystore create keystore/multi2fa-release.keystore (asks for passwords; back it up!)"
	@echo "  make android-release KEYSTORE_PASS=... [KEY_PASS=...] [BUILD=<versionCode>]"
	@echo "                       Google Play .aab + universal and arm64 APKs signed with your key"
	@echo "                       -> build/android-release/"
	@echo "  make ios             iOS app for devices (macOS + Xcode, provisioning profile) -> build/ios/"
	@echo "  make ios-simulator   iOS simulator app (macOS + Xcode) -> build/ios/"
	@echo "  make ios-release     App Store .ipa: IOS_CERT=\"Apple Distribution\" IOS_PROFILE=<profile name>"
	@echo ""
	@echo "Versioning"
	@echo "  make version         print the version (from VERSION)"
	@echo "  make bump-patch | bump-minor | bump-major"
	@echo ""
	@echo "  make clean           remove build output"

build:
	@mkdir -p $(OUT)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(OUT)/$(BIN) $(PKG)

cli:
	@mkdir -p $(OUT)
	$(CLIBUILD) -ldflags "$(LDFLAGS)" -o $(OUT)/$(CLI)$(shell go env GOEXE) $(CLI_PKG)

run: build
	./$(OUT)/$(BIN)

test:
	go test ./...

vet:
	go vet ./...

check: vet test

linux:
	@rm -rf "$(LINUX_DIR)" && mkdir -p "$(LINUX_DIR)/icons"
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o "$(LINUX_DIR)/$(BIN)" $(PKG)
	$(CLIBUILD) -ldflags "$(LDFLAGS)" -o "$(LINUX_DIR)/$(CLI)" $(CLI_PKG)
	cp -r assets/hicolor "$(LINUX_DIR)/icons/"
	cp assets/icon.png "$(LINUX_DIR)/icon.png"
	cp packaging/linux/$(APP_ID).desktop packaging/linux/$(APP_ID).xml packaging/linux/install.sh packaging/linux/uninstall.sh "$(LINUX_DIR)/"
	chmod 755 "$(LINUX_DIR)/$(APP_ID).desktop" "$(LINUX_DIR)/install.sh" "$(LINUX_DIR)/uninstall.sh"
	tar -C "$(OUT)/linux" -czf "$(LINUX_DIR).tar.gz" "$(notdir $(LINUX_DIR))"
	@echo "linux package: $(LINUX_DIR) (+ .tar.gz)"

# Embeds icon, manifest and version info into the .exe via .syso files picked up by go build.
winres:
	$(WINRES) simply --arch amd64,arm64 --out $(PKG)/rsrc --manifest gui \
		--icon assets/icon.ico \
		--product-name "$(NAME)" --file-description "$(NAME)" \
		--product-version $(VERSION).$(BUILD) --file-version $(VERSION).$(BUILD) \
		--original-filename $(EXE) --copyright "Copyright (c) 2026 keklick1337"

windows: winres
	@rm -rf "$(WIN_DIR)" && mkdir -p "$(WIN_DIR)"
ifeq ($(OS),Windows_NT)
	$(GOBUILD) -ldflags "$(LDFLAGS) -H=windowsgui" -o "$(WIN_DIR)/$(EXE)" $(PKG)
else
	CC=$(WIN_CC_$(WIN_ARCH)) GOOS=windows GOARCH=$(WIN_ARCH) \
		$(GOBUILD) -ldflags "$(LDFLAGS) -H=windowsgui" -o "$(WIN_DIR)/$(EXE)" $(PKG)
endif
	GOOS=windows GOARCH=$(WIN_ARCH) $(CLIBUILD) -ldflags "$(LDFLAGS)" -o "$(WIN_DIR)/$(CLI).exe" $(CLI_PKG)
	cp assets/icon.ico assets/icon.png packaging/windows/README.txt "$(WIN_DIR)/"
	cd "$(OUT)/windows" && rm -f "$(notdir $(WIN_DIR)).zip" && zip -qr "$(notdir $(WIN_DIR)).zip" "$(notdir $(WIN_DIR))"
	@echo "windows package: $(WIN_DIR) (+ .zip)"

# macOS from Linux: the fyne-cross darwin image (zig) plus a macOS SDK that you provide, either
# as a directory (macos-sdk/MacOSX15.sdk) or a tarball (MACOS_SDK=MacOSX15.sdk.tar.xz).
DARWIN_IMAGE ?= fyneio/fyne-cross-images:darwin
MACOS_SDK    ?= $(firstword $(wildcard macos-sdk/*.sdk macos-sdk/*.sdk.tar.* macos-sdk/*.tar.xz))

# darwin_cross OUTDIR
define darwin_cross
	@sdk="$(MACOS_SDK)"; \
	if [ -z "$$sdk" ]; then echo "skip macos: put a macOS SDK into macos-sdk/ or pass MACOS_SDK=... (see README)"; exit 0; fi; \
	if [ -f "$$sdk" ]; then \
		mkdir -p "$(OUT)/macos-sdk" && tar -C "$(OUT)/macos-sdk" -xf "$$sdk" && sdk=$$(ls -d "$(OUT)"/macos-sdk/*.sdk | head -1); \
	fi; \
	mkdir -p "$(1)" && \
	docker run --rm -v "$(CURDIR)":/src:ro -v "$$(cd "$$sdk" && pwd)":/sdk:ro -v "$(abspath $(1))":/out \
		-v multi2fa-darwin-go:/go -e GOPATH=/go -e GOMODCACHE=/go/pkg/mod -e GOCACHE=/go/cache \
		-e GOTOOLCHAIN=go$(GO_VERSION) -e HOME=/tmp -e VERSION=$(VERSION) -e BUILD=$(BUILD) \
		-e HOST_UID=$$(id -u) -e HOST_GID=$$(id -g) \
		--entrypoint bash $(DARWIN_IMAGE) /src/packaging/darwin/build.sh
endef

darwin-cross:
	$(call darwin_cross,$(OUT)/darwin)

# Universal (arm64 + amd64) app bundle; needs Xcode command line tools.
darwin:
	@test "$(UNAME_S)" = Darwin || (echo "macOS bundles need a Mac (cgo + Apple SDK). Run 'make darwin' on macOS."; exit 1)
	@rm -rf "$(MAC_DIR)" && mkdir -p "$(MAC_APP)/Contents/MacOS" "$(MAC_APP)/Contents/Resources" "$(MAC_DIR)/tmp"
	GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o "$(MAC_DIR)/tmp/$(BIN)-arm64" $(PKG)
	GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o "$(MAC_DIR)/tmp/$(BIN)-amd64" $(PKG)
	lipo -create -output "$(MAC_APP)/Contents/MacOS/$(BIN)" "$(MAC_DIR)/tmp/$(BIN)-arm64" "$(MAC_DIR)/tmp/$(BIN)-amd64"
	GOARCH=arm64 $(CLIBUILD) -ldflags "$(LDFLAGS)" -o "$(MAC_DIR)/tmp/$(CLI)-arm64" $(CLI_PKG)
	GOARCH=amd64 $(CLIBUILD) -ldflags "$(LDFLAGS)" -o "$(MAC_DIR)/tmp/$(CLI)-amd64" $(CLI_PKG)
	lipo -create -output "$(MAC_APP)/Contents/MacOS/$(CLI)" "$(MAC_DIR)/tmp/$(CLI)-arm64" "$(MAC_DIR)/tmp/$(CLI)-amd64"
	sed -e "s/@VERSION@/$(VERSION)/" -e "s/@BUILD@/$(BUILD)/" packaging/darwin/Info.plist > "$(MAC_APP)/Contents/Info.plist"
	cp assets/icon.icns "$(MAC_APP)/Contents/Resources/icon.icns"
	cp assets/icon.png "$(MAC_DIR)/icon.png"
	rm -rf "$(MAC_DIR)/tmp"
	codesign --force --deep --sign - "$(MAC_APP)" || true
	cd "$(MAC_DIR)" && ditto -c -k --keepParent "$(NAME).app" "Multi2FA-TOTP-$(VERSION)-macos-universal.zip"
	hdiutil create -volname "$(NAME)" -srcfolder "$(MAC_APP)" -ov -format UDZO "$(MAC_DIR)/Multi2FA-TOTP-$(VERSION)-macos-universal.dmg"
	@echo "macOS bundle: $(MAC_APP) (+ .zip, .dmg)"

dist: check $(NATIVE)
ifneq ($(NATIVE),darwin)
	@if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then $(MAKE) windows; else echo "skip windows: x86_64-w64-mingw32-gcc not found"; fi
endif

# Builds every icon from assets/icon.svg (or ICON_SVG=<file>.svg).
icons:
	go run ./scripts/genicon -svg $(or $(ICON_SVG),assets/icon.svg)

version:
	@echo $(VERSION)

bump-patch bump-minor bump-major:
	@awk -F. -v part=$(subst bump-,,$@) '{ \
		if (part=="major") { $$1++; $$2=0; $$3=0 } \
		else if (part=="minor") { $$2++; $$3=0 } \
		else { $$3++ } \
		print $$1"."$$2"."$$3 }' VERSION > VERSION.tmp && mv VERSION.tmp VERSION
	@echo "version is now $$(cat VERSION); add a section to CHANGELOG.md"

clean:
	rm -rf $(OUT) fyne-cross $(PKG)/rsrc_windows_*.syso

# ---- Release builds in Docker -------------------------------------------------------------
# Every artifact lands directly in $(REL). Linux packages for other CPUs are cross compiled
# (Debian multiarch, RPM sysroots + clang), no emulation needed. Parts run in parallel.

REL              ?= $(OUT)/release
RELEASE_JOBS     ?= 6
PORTABLE_ARCHES  ?= amd64 arm64 armhf
DEB_RELEASES     ?= debian:11 debian:12 debian:13
DEB_ARCHES       ?= amd64 arm64 armhf
RPM_TARGETS      ?= almalinux:8 almalinux:9 almalinux:10 fedora:42 fedora:43 fedora:44
SUSE_TARGETS     ?= opensuse/leap:15.6
RPM_ARCHES       ?= x86_64 aarch64
# os/arch pairs for the standalone CLI archives (pure Go, cross compiled in one container);
# the CLI .deb/.rpm/Arch packages are made from the linux ones (see packaging/docker/build-cli.sh).
CLI_TARGETS      ?= linux/amd64 linux/386 linux/arm64 linux/armv7 linux/armv6 linux/riscv64 linux/ppc64le linux/ppc64 \
                    linux/s390x linux/loong64 linux/mips linux/mipsle linux/mips64 linux/mips64le \
                    windows/amd64 windows/386 windows/arm64 darwin/amd64 darwin/arm64 \
                    freebsd/amd64 freebsd/386 freebsd/arm64 freebsd/armv7 \
                    openbsd/amd64 openbsd/386 openbsd/arm64 openbsd/armv7 openbsd/ppc64 openbsd/riscv64 \
                    netbsd/amd64 netbsd/386 netbsd/arm64 netbsd/armv7 \
                    illumos/amd64 solaris/amd64 aix/ppc64
RUN_DOCKER       := GO_VERSION=$(GO_VERSION) sh packaging/docker/run.sh "$(REL)"

# Job names encode image and arch: deb@debian_12@arm64 (":" -> "_", "/" -> "+").
job_id      = $(subst /,+,$(subst :,_,$(1)))
job_image   = $(subst +,/,$(subst _,:,$(word 1,$(subst @, ,$(1)))))
job_arch    = $(word 2,$(subst @, ,$(1)))
PORTABLE_JOBS := $(foreach a,$(PORTABLE_ARCHES),portable@debian_11@$(a))
DEB_JOBS      := $(foreach b,$(DEB_RELEASES),$(foreach a,$(DEB_ARCHES),deb@$(call job_id,$(b))@$(a)))
RPM_JOBS      := $(foreach b,$(RPM_TARGETS),$(foreach a,$(RPM_ARCHES),rpm@$(call job_id,$(b))@$(a)))
SUSE_JOBS     := $(foreach b,$(SUSE_TARGETS),$(foreach a,$(RPM_ARCHES),suse@$(call job_id,$(b))@$(a)))

release: check
	rm -rf "$(REL)"
	@$(MAKE) --no-print-directory -j$(RELEASE_JOBS) -O \
		release-portable release-deb release-rpm release-windows release-macos release-cli release-android
	@$(MAKE) --no-print-directory checksums
	@echo "release $(VERSION): $$(ls "$(REL)" | wc -l) files in $(REL)/"

release-portable: $(PORTABLE_JOBS)
release-deb: $(DEB_JOBS)
release-rpm: $(RPM_JOBS) $(SUSE_JOBS)

portable@%:
	$(RUN_DOCKER) deb.Dockerfile $(call job_image,$*) build-portable.sh $(call job_arch,$*)
deb@%:
	$(RUN_DOCKER) deb.Dockerfile $(call job_image,$*) build-deb.sh $(call job_arch,$*)
rpm@%:
	$(RUN_DOCKER) rpm.Dockerfile $(call job_image,$*) build-rpm.sh $(call job_arch,$*)
suse@%:
	$(RUN_DOCKER) suse.Dockerfile $(call job_image,$*) build-rpm.sh $(call job_arch,$*)

release-windows:
	BUILD=$(BUILD) $(RUN_DOCKER) windows.Dockerfile debian:12 build-windows.sh

release-macos:
	$(call darwin_cross,$(REL))

release-cli:
	CLI_TARGETS="$(CLI_TARGETS)" $(RUN_DOCKER) deb.Dockerfile debian:12 build-cli.sh

# Android .aab and APKs: signed with the release key when it and KEYSTORE_PASS are available,
# otherwise with the local development key (fine for testing, but later releases signed with the
# real key cannot update such installs).
release-android:
	@if [ -f "$(KEYSTORE_DIR)/$(KEYSTORE)" ] && [ -n "$(KEYSTORE_PASS)" ]; then \
		$(MAKE) --no-print-directory android-release ANDROID_OUT="$(REL)"; \
	else \
		echo "warning: no release key ($(KEYSTORE_DIR)/$(KEYSTORE) + KEYSTORE_PASS), Android is signed with the development key"; \
		$(MAKE) --no-print-directory $(DEV_KEYSTORE) && \
		$(MAKE) --no-print-directory android-dev ANDROID_OUT="$(REL)"; \
	fi

checksums:
	cd "$(REL)" && find . -maxdepth 1 -type f ! -name SHA256SUMS -printf '%P\n' | sort | xargs sha256sum > SHA256SUMS
	@cat "$(REL)/SHA256SUMS"

# ---- Mobile -------------------------------------------------------------------------------
# Android builds run in the fyne-cross Android image (SDK, NDK, bundletool, JDK) via
# packaging/android/build.sh and produce a signed .aab plus universal and arm64 APKs.
# iOS needs macOS with Xcode.

ANDROID_IMAGE    ?= fyneio/fyne-cross-images:android
KEYSTORE_DIR     := keystore
KEYSTORE         ?= multi2fa-release.keystore
KEY_ALIAS        ?= multi2fa
KEYSTORE_PASS    ?=
KEY_PASS         ?= $(KEYSTORE_PASS)
DEV_KEYSTORE     := $(KEYSTORE_DIR)/development.keystore
ANDROID_OUT      ?= $(OUT)/android-release

# android_build OUTDIR KEYSTORE STOREPASS ALIAS KEYPASS
define android_build
	@mkdir -p $(1)
	docker run --rm -v "$(CURDIR)":/src:ro -v "$(CURDIR)/$(KEYSTORE_DIR)":/keystore:ro -v "$(CURDIR)/$(1)":/out \
		-v multi2fa-android-go:/go -e GOPATH=/go -e GOMODCACHE=/go/pkg/mod -e GOCACHE=/go/cache \
		-e GOTOOLCHAIN=go$(GO_VERSION) -e HOME=/tmp -e VERSION=$(VERSION) -e BUILD=$(BUILD) -e APP_ID=$(APP_ID) \
		-e KEYSTORE=$(2) -e KEYSTORE_PASS=$(3) -e KEY_ALIAS=$(4) -e KEY_PASS=$(5) \
		-e HOST_UID=$$(id -u) -e HOST_GID=$$(id -g) \
		--entrypoint bash $(ANDROID_IMAGE) /src/packaging/android/build.sh
endef

$(DEV_KEYSTORE):
	@mkdir -p $(KEYSTORE_DIR)
	docker run --rm --user $$(id -u):$$(id -g) -v "$(CURDIR)/$(KEYSTORE_DIR)":/k --entrypoint keytool $(ANDROID_IMAGE) \
		-genkeypair -keystore /k/development.keystore -alias development -keyalg RSA -keysize 2048 -validity 10000 \
		-storepass development -keypass development -dname "CN=Multi2FA development build"

# Installable test build signed with a local development key (not for Google Play).
android: $(DEV_KEYSTORE)
	$(call android_build,$(OUT)/android,development.keystore,development,development,development)

android-dev:
	$(call android_build,$(ANDROID_OUT),development.keystore,development,development,development)

# Signed release for Google Play and direct download. Create the key once with android-keystore
# and keep a backup: updates must be signed with the same key.
android-release:
	@test -f "$(KEYSTORE_DIR)/$(KEYSTORE)" || (echo "no $(KEYSTORE_DIR)/$(KEYSTORE): run 'make android-keystore' first"; exit 1)
	@test -n "$(KEYSTORE_PASS)" || (echo "usage: make android-release KEYSTORE_PASS=... [KEY_PASS=...] [KEYSTORE=$(KEYSTORE)] [KEY_ALIAS=$(KEY_ALIAS)] [BUILD=<versionCode>]"; exit 1)
	$(call android_build,$(ANDROID_OUT),$(KEYSTORE),$(KEYSTORE_PASS),$(KEY_ALIAS),$(KEY_PASS))

# Creates keystore/multi2fa-release.keystore; keytool asks for the passwords and your name.
android-keystore:
	@mkdir -p $(KEYSTORE_DIR)
	@test ! -f "$(KEYSTORE_DIR)/$(KEYSTORE)" || (echo "$(KEYSTORE_DIR)/$(KEYSTORE) already exists"; exit 1)
	docker run --rm -it --user $$(id -u):$$(id -g) -v "$(CURDIR)/$(KEYSTORE_DIR)":/k --entrypoint keytool $(ANDROID_IMAGE) \
		-genkeypair -keystore /k/$(KEYSTORE) -alias $(KEY_ALIAS) -keyalg RSA -keysize 4096 -validity 10000
	@echo "created $(KEYSTORE_DIR)/$(KEYSTORE) - back it up, Google Play updates need the same key"

ios ios-simulator:
	@test "$(UNAME_S)" = Darwin || (echo "iOS builds need macOS with Xcode."; exit 1)
	@mkdir -p $(OUT)/ios
	cd $(PKG) && $(FYNE) package -os $(subst ios-simulator,iossimulator,$@) -app-id $(APP_ID) -app-version $(VERSION) \
		-app-build $(BUILD) -icon ../../assets/icon.png -name $(MOBILE_NAME)
	rm -rf "$(OUT)/ios/$(MOBILE_NAME)-$@.app" && mv $(PKG)/$(MOBILE_NAME).app "$(OUT)/ios/$(MOBILE_NAME)-$@.app"

ios-release:
	@test "$(UNAME_S)" = Darwin || (echo "iOS builds need macOS with Xcode."; exit 1)
	@test -n "$(IOS_PROFILE)" || (echo "usage: make ios-release IOS_PROFILE=<provisioning profile> [IOS_CERT=\"Apple Distribution\"]"; exit 1)
	@mkdir -p $(OUT)/ios
	cd $(PKG) && $(FYNE) release -os ios -app-id $(APP_ID) -app-version $(VERSION) -app-build $(BUILD) \
		-icon ../../assets/icon.png -name $(MOBILE_NAME) -certificate "$(IOS_CERT)" -profile "$(IOS_PROFILE)"
	mv $(PKG)/*.ipa $(OUT)/ios/
