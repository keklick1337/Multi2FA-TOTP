package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	multi2fa "github.com/keklick1337/Multi2FA-TOTP"
	"github.com/keklick1337/Multi2FA-TOTP/internal/harden"
	"github.com/keklick1337/Multi2FA-TOTP/internal/ui"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// version can be overridden with -ldflags "-X main.version=..."; by default it comes from VERSION.
var version = multi2fa.Version()

func main() {
	harden.Apply()
	defer harden.Purge()

	path := flag.String("vault", "", "path to the vault file (default: user config dir, or $MULTI2FA_VAULT)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: multi2fa [flags] [vault.m2fa]\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println(ui.AppName, version)
		return
	}
	// On Android and iOS the UI picks the app's private storage instead.
	if *path == "" && runtime.GOOS != "android" && runtime.GOOS != "ios" {
		p, err := vault.DefaultPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "cannot determine vault location:", err)
			os.Exit(1)
		}
		*path = p
	}
	// GLFW uses RESOURCE_NAME as the X11 WM_CLASS instance; it must match StartupWMClass in the .desktop file.
	if runtime.GOOS == "linux" && os.Getenv("RESOURCE_NAME") == "" {
		os.Setenv("RESOURCE_NAME", "multi2fa")
	}
	ui.Version = version
	ui.Run(*path, flag.Arg(0))
}
