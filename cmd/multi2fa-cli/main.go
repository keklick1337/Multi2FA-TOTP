// Command multi2fa-cli reads and edits Multi2FA vaults from a terminal: list accounts, print
// codes, add, remove, export and back up. It shares the vault format and crypto with the GUI
// and builds without cgo for every platform Go supports.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	multi2fa "github.com/keklick1337/Multi2FA-TOTP"
	"github.com/keklick1337/Multi2FA-TOTP/internal/harden"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const name = "multi2fa-cli"

// version can be overridden with -ldflags "-X main.version=..."; by default it comes from VERSION.
var version = multi2fa.Version()

// globals are accepted before or after the command.
type globals struct {
	vaultPath    string
	keyFile      string
	password     string
	passwordFile string
	passStdin    bool
	json         bool
}

type command struct {
	name, args, help string
	run              func(g *globals, args []string) error
}

var commands []command

func init() {
	commands = []command{
		{"list", "[query]", "list accounts (number, id, issuer, account, folder)", cmdList},
		{"code", "<account>...", "print only the current code of each account", cmdCode},
		{"codes", "[query]", "show the codes of all (or matching) accounts", cmdCodes},
		{"add", "[otpauth-uri | image]...", "add accounts from links, QR images or by hand", cmdAdd},
		{"remove", "<account>...", "delete accounts", cmdRemove},
		{"export", "[query]", "print otpauth:// links or QR codes (reveals secrets)", cmdExport},
		{"backup", "<file.m2fab>", "write an encrypted backup with its own password", cmdBackup},
		{"import", "<file.m2fab|file.m2fa>", "merge accounts from a backup or another vault", cmdImport},
		{"passwd", "", "change the master password", cmdPasswd},
		{"info", "", "show the vault's encryption settings (no password needed)", cmdInfo},
		{"version", "", "print the version", func(*globals, []string) error { fmt.Println(name, version); return nil }},
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `%s %s - Multi2FA TOTP from the command line

Usage: %s [flags] <command> [arguments]

Commands:
`, name, version, name)
	for _, c := range commands {
		fmt.Fprintf(w, "  %-8s %-26s %s\n", c.name, c.args, c.help)
	}
	fmt.Fprintf(w, `
Accounts are selected by number (from "list"), id or id prefix, or a search
query matching issuer, account, notes or tags ("github", "work alice").

Flags (before or after the command):
  -vault FILE           vault file (default: $MULTI2FA_VAULT or the app's vault)
  -key-file FILE        key file, if the vault uses one ($MULTI2FA_KEY_FILE)
  -password-file FILE   read the master password from the first line of FILE
  -password-stdin       read the master password from the first line of stdin
  -password PASS        master password as an argument (visible to other users!)
  -json                 machine readable output
The password can also come from $MULTI2FA_PASSWORD; otherwise it is asked for
on the terminal without echo.

Examples:
  %[1]s codes                      every code with the seconds left
  %[1]s codes -watch               live view, refreshed every second
  %[1]s code github -copy          copy GitHub's code to the clipboard
  %[1]s add 'otpauth://totp/...'   add an account from a link
  %[1]s add screenshot.png         add every account from QR codes in an image
  %[1]s export 3 -qr               show account 3 as a QR code in the terminal

Run "%[1]s <command> -h" for the options of a command.
`, name)
}

func main() {
	harden.Apply()
	code := run(os.Args[1:])
	harden.Purge()
	os.Exit(code)
}

func run(args []string) int {
	g := &globals{}
	fs := newFlagSet("", g)
	rest, err := parseInterleaved(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		usage(os.Stdout)
		return 0
	}
	if err != nil {
		return 2
	}
	if len(rest) == 0 {
		usage(os.Stderr)
		return 2
	}
	for _, c := range commands {
		if c.name == rest[0] {
			if err := c.run(g, rest[1:]); err != nil {
				if !errors.Is(err, flag.ErrHelp) {
					fmt.Fprintln(os.Stderr, name+":", err)
					return 1
				}
			}
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "%s: unknown command %q\n\n", name, rest[0])
	usage(os.Stderr)
	return 2
}

// newFlagSet creates a flag set that also understands the global flags.
func newFlagSet(cmd string, g *globals) *flag.FlagSet {
	fs := flag.NewFlagSet(strings.TrimSpace(name+" "+cmd), flag.ContinueOnError)
	fs.StringVar(&g.vaultPath, "vault", g.vaultPath, "vault file")
	fs.StringVar(&g.keyFile, "key-file", g.keyFile, "key file")
	fs.StringVar(&g.password, "password", g.password, "master password (visible to other users)")
	fs.StringVar(&g.passwordFile, "password-file", g.passwordFile, "read the master password from a file")
	fs.BoolVar(&g.passStdin, "password-stdin", g.passStdin, "read the master password from stdin")
	fs.BoolVar(&g.json, "json", g.json, "JSON output")
	if cmd == "" {
		fs.Usage = func() {}
	}
	return fs
}

// parseInterleaved lets flags follow positional arguments: "code github -copy".
// Without a command yet it stops at the first positional argument so that the command's
// own flags are left for the command.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		if fs.Name() == name {
			return append(pos, args...), nil
		}
		if args[0] == "--" {
			return append(pos, args[1:]...), nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func resolvePath(g *globals) (string, error) {
	if g.vaultPath != "" {
		return g.vaultPath, nil
	}
	return vault.DefaultPath()
}
