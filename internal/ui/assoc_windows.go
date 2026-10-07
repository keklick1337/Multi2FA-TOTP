//go:build windows

package ui

import (
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const fileAssocSupported = true

// associateFiles registers .m2fa and .m2fab for the current user (no administrator rights needed).
func associateFiles() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	types := []struct{ ext, progID, desc string }{
		{".m2fa", "Multi2FA.Vault", "Multi2FA TOTP vault"},
		{".m2fab", "Multi2FA.Backup", "Multi2FA TOTP backup"},
	}
	set := func(path, value string) error {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\`+path, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		return k.SetStringValue("", value)
	}
	for _, t := range types {
		for _, kv := range [][2]string{
			{t.ext, t.progID},
			{t.progID, t.desc},
			{t.progID + `\DefaultIcon`, `"` + exe + `",0`},
			{t.progID + `\shell\open\command`, `"` + exe + `" "%1"`},
		} {
			if err := set(kv[0], kv[1]); err != nil {
				return err
			}
		}
	}
	// SHCNE_ASSOCCHANGED so Explorer picks up the new icons right away.
	windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify").Call(0x08000000, 0, 0, 0)
	return nil
}
