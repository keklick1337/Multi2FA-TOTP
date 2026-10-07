package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

var testKDF = vault.KDFParams{Kind: vault.KDFArgon2id, Iterations: 1, MemoryKiB: 8 * 1024, Threads: 1}

func testVault(t *testing.T) (*vault.Vault, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v.m2fa")
	v, err := vault.Create(path, vault.Credentials{Password: []byte("secret-pass")}, vault.Options{Security: vault.Security{KDF: testKDF}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	secrets := []string{"JBSWY3DPEHPK3PXP", "KRSXG5CTMVRXEZLU", "MFRGGZDFMZTWQ2LK", "GEZDGNBVGY3TQOJQ"}
	for i, a := range [][2]string{{"GitHub", "alice@example.com"}, {"Google", "alice@example.com"}, {"GitLab", "bob"}, {"Steam", "carol"}} {
		k := otp.Key{Type: otp.TOTP, Issuer: a[0], Account: a[1], Secret: secrets[i], Algorithm: otp.SHA1, Digits: 6, Period: 30}
		if err := v.Add(&vault.Entry{Key: k}); err != nil {
			t.Fatal(err)
		}
	}
	return v, path
}

func TestParseInterleaved(t *testing.T) {
	g := &globals{}
	fs := newFlagSet("code", g)
	cp := fs.Bool("copy", false, "")
	pos, err := parseInterleaved(fs, []string{"github", "-copy", "-json", "gitlab", "--", "-x"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pos, []string{"github", "gitlab", "-x"}) || !*cp || !g.json {
		t.Fatalf("pos=%v copy=%v json=%v", pos, *cp, g.json)
	}

	// The top level stops at the command so that its own flags are left alone.
	g = &globals{}
	rest, err := parseInterleaved(newFlagSet("", g), []string{"-vault", "x.m2fa", "code", "-copy", "github"})
	if err != nil {
		t.Fatal(err)
	}
	if g.vaultPath != "x.m2fa" || !slices.Equal(rest, []string{"code", "-copy", "github"}) {
		t.Fatalf("vault=%q rest=%v", g.vaultPath, rest)
	}
}

func TestSelectOne(t *testing.T) {
	v, _ := testVault(t)
	all := allEntries(v)
	cases := map[string]string{
		"1":             all[0].e.Issuer,
		"#2":            all[1].e.Issuer,
		"steam":         "Steam",
		"bob":           "GitLab",
		"github":        "GitHub",
		"Google":        "Google",
		"gitl":          "GitLab",
		all[3].e.ID[:8]: all[3].e.Issuer,
	}
	for sel, want := range cases {
		n, err := selectOne(v, sel)
		if err != nil {
			t.Errorf("%q: %v", sel, err)
			continue
		}
		if n.e.Issuer != want {
			t.Errorf("%q: got %s, want %s", sel, n.e.Issuer, want)
		}
	}
	for _, sel := range []string{"alice", "git", "99", "nothing"} {
		if _, err := selectOne(v, sel); err == nil {
			t.Errorf("%q: expected an error", sel)
		}
	}
	if _, err := selectOne(v, "alice"); err == nil || !strings.Contains(err.Error(), "GitHub") {
		t.Errorf("ambiguous selection should list the candidates, got %v", err)
	}
}

func TestEnsureFolder(t *testing.T) {
	v, _ := testVault(t)
	a, err := ensureFolder(v, "Work/Servers")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ensureFolder(v, " work / servers ")
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b {
		t.Fatalf("folder ids differ: %q %q", a, b)
	}
	if got := strings.Join(v.FolderPath(a), "/"); got != "Work/Servers" {
		t.Fatalf("path %q", got)
	}
	if id, _ := ensureFolder(v, ""); id != "" {
		t.Fatalf("empty path gave %q", id)
	}
}

func TestPasswordSources(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pw")
	if err := os.WriteFile(file, []byte("from-file\r\nignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pw, err := password(&globals{passwordFile: file}, "")
	if err != nil || string(pw) != "from-file" {
		t.Fatalf("file: %q %v", pw, err)
	}
	pw, err = password(&globals{password: "from-arg"}, "")
	if err != nil || string(pw) != "from-arg" {
		t.Fatalf("arg: %q %v", pw, err)
	}
	t.Setenv("MULTI2FA_PASSWORD", "from-env")
	pw, err = password(&globals{}, "")
	if err != nil || string(pw) != "from-env" {
		t.Fatalf("env: %q %v", pw, err)
	}
}

func TestOpenVaultAndCodes(t *testing.T) {
	_, path := testVault(t)
	t.Setenv("MULTI2FA_PASSWORD", "secret-pass")
	v, err := openVault(&globals{vaultPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if len(v.Entries()) != 4 {
		t.Fatalf("%d entries", len(v.Entries()))
	}
	t.Setenv("MULTI2FA_PASSWORD", "wrong")
	if _, err := openVault(&globals{vaultPath: path}); err == nil || !strings.Contains(err.Error(), "wrong password") {
		t.Fatalf("wrong password: %v", err)
	}
}

func TestPrintQR(t *testing.T) {
	if err := printQR("otpauth://totp/A:b?secret=JBSWY3DPEHPK3PXP&issuer=A"); err != nil {
		t.Fatal(err)
	}
}
