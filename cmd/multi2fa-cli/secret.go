package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/awnumar/memguard"
	"golang.org/x/term"

	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// openTTY returns the controlling terminal for prompts, so they work while stdin or stdout
// are redirected.
func openTTY() (in, out *os.File, err error) {
	if runtime.GOOS == "windows" {
		if in, err = os.OpenFile("CONIN$", os.O_RDWR, 0); err != nil {
			return nil, nil, err
		}
		if out, err = os.OpenFile("CONOUT$", os.O_RDWR, 0); err != nil {
			in.Close()
			return nil, nil, err
		}
		return in, out, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	return tty, tty, err
}

// readHidden asks for a secret on the terminal without echo.
func readHidden(prompt string) ([]byte, error) {
	in, out, err := openTTY()
	if err != nil {
		return nil, errors.New("no terminal to ask for the password; use -password-stdin, -password-file or $MULTI2FA_PASSWORD")
	}
	defer in.Close()
	if out != in {
		defer out.Close()
	}
	fmt.Fprint(out, prompt)
	b, err := term.ReadPassword(int(in.Fd()))
	fmt.Fprintln(out)
	return b, err
}

// confirm asks a yes/no question on the terminal; without one the answer is no.
func confirm(question string) bool {
	in, out, err := openTTY()
	if err != nil {
		return false
	}
	defer in.Close()
	if out != in {
		defer out.Close()
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	line, _ := bufio.NewReader(in).ReadString('\n')
	return len(line) > 0 && (line[0] == 'y' || line[0] == 'Y')
}

// firstLine reads up to the first newline (a trailing \r is dropped too).
func firstLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[:i]
	}
	return bytes.TrimSuffix(b, []byte("\r"))
}

// password returns the master password from the first configured source.
func password(g *globals, prompt string) ([]byte, error) {
	switch {
	case g.passwordFile != "":
		b, err := os.ReadFile(g.passwordFile)
		if err != nil {
			return nil, err
		}
		pw := append([]byte(nil), firstLine(b)...)
		memguard.WipeBytes(b)
		return pw, nil
	case g.passStdin:
		r := bufio.NewReader(os.Stdin)
		line, err := r.ReadBytes('\n')
		if err != nil && len(line) == 0 {
			return nil, errors.New("no password on stdin")
		}
		pw := append([]byte(nil), firstLine(line)...)
		memguard.WipeBytes(line)
		return pw, nil
	case g.password != "":
		return []byte(g.password), nil
	case os.Getenv("MULTI2FA_PASSWORD") != "":
		return []byte(os.Getenv("MULTI2FA_PASSWORD")), nil
	}
	return readHidden(prompt)
}

func keyFile(g *globals) ([]byte, error) {
	p := g.keyFile
	if p == "" {
		p = os.Getenv("MULTI2FA_KEY_FILE")
	}
	if p == "" {
		return nil, nil
	}
	return os.ReadFile(p)
}

func credentials(g *globals, path, prompt string) (vault.Credentials, error) {
	kf, err := keyFile(g)
	if err != nil {
		return vault.Credentials{}, err
	}
	if kf == nil {
		if info, err := vault.Inspect(path); err == nil && info.KeyFile {
			return vault.Credentials{}, errors.New("this vault needs its key file: use -key-file or $MULTI2FA_KEY_FILE")
		}
	}
	pw, err := password(g, prompt)
	if err != nil {
		memguard.WipeBytes(kf)
		return vault.Credentials{}, err
	}
	return vault.Credentials{Password: pw, KeyFile: kf}, nil
}

func wipeCredentials(c vault.Credentials) {
	memguard.WipeBytes(c.Password)
	memguard.WipeBytes(c.KeyFile)
}

// openVault unlocks the vault selected by the flags.
func openVault(g *globals) (*vault.Vault, error) {
	path, err := resolvePath(g)
	if err != nil {
		return nil, err
	}
	if !vault.Exists(path) {
		return nil, fmt.Errorf("no vault at %s (create one in the app, or pass -vault)", path)
	}
	c, err := credentials(g, path, "Master password: ")
	if err != nil {
		return nil, err
	}
	defer wipeCredentials(c)
	v, err := vault.Open(path, c)
	if errors.Is(err, vault.ErrWrongPassword) {
		return nil, errors.New("wrong password or key file")
	}
	return v, err
}

// newPassword asks twice for a password that protects a new file.
func newPassword(what string) ([]byte, error) {
	pw, err := readHidden(what + ": ")
	if err != nil {
		return nil, err
	}
	again, err := readHidden("Repeat: ")
	if err != nil {
		memguard.WipeBytes(pw)
		return nil, err
	}
	defer memguard.WipeBytes(again)
	if !bytes.Equal(pw, again) {
		memguard.WipeBytes(pw)
		return nil, errors.New("passwords do not match")
	}
	if len(pw) < minPasswordLen {
		memguard.WipeBytes(pw)
		return nil, fmt.Errorf("the password must be at least %d characters", minPasswordLen)
	}
	return pw, nil
}

const minPasswordLen = 8
