package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/qr"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// row is one account as shown by list, codes and -json.
type row struct {
	N         int      `json:"n"`
	ID        string   `json:"id"`
	Issuer    string   `json:"issuer"`
	Account   string   `json:"account"`
	Folder    string   `json:"folder,omitempty"`
	Type      otp.Type `json:"type"`
	Algorithm string   `json:"algorithm"`
	Digits    int      `json:"digits"`
	Period    int      `json:"period,omitempty"`
	Counter   uint64   `json:"counter,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Favorite  bool     `json:"favorite,omitempty"`
	Code      string   `json:"code,omitempty"`
	Remaining int      `json:"remaining,omitempty"`
}

// numbered pairs entries with their stable number: the order of "list" (favorites first,
// then by issuer and account).
type numbered struct {
	n int
	e *vault.Entry
}

func allEntries(v *vault.Vault) []numbered {
	var out []numbered
	for i, e := range v.Entries() {
		out = append(out, numbered{i + 1, e})
	}
	return out
}

func folderPath(v *vault.Vault, e *vault.Entry) string {
	if e.Folder == "" {
		return ""
	}
	return strings.Join(v.FolderPath(e.Folder), "/")
}

func toRow(v *vault.Vault, n numbered) row {
	e := n.e
	return row{N: n.n, ID: e.ID, Issuer: e.Issuer, Account: e.Account, Folder: folderPath(v, e), Type: e.Type,
		Algorithm: string(e.Algorithm), Digits: e.Digits, Period: e.Period, Counter: e.Counter,
		Tags: e.Tags, Favorite: e.Favorite}
}

func matches(v *vault.Vault, n numbered, query string) bool {
	if n.e.Matches(query) {
		return true
	}
	// The folder path is searchable too: "codes work".
	return query != "" && strings.Contains(strings.ToLower(folderPath(v, n.e)), strings.ToLower(query))
}

func filter(v *vault.Vault, query string) []numbered {
	var out []numbered
	for _, n := range allEntries(v) {
		if matches(v, n, query) {
			out = append(out, n)
		}
	}
	return out
}

// selectOne resolves an account by number, id, id prefix or a query with exactly one match.
func selectOne(v *vault.Vault, sel string) (numbered, error) {
	all := allEntries(v)
	// A number outside the list may still be an id prefix made of digits.
	n, numErr := strconv.Atoi(strings.TrimPrefix(sel, "#"))
	if numErr == nil && n >= 1 && n <= len(all) {
		return all[n-1], nil
	}
	if numErr == nil && strings.HasPrefix(sel, "#") {
		return numbered{}, fmt.Errorf("no account number %d (there are %d)", n, len(all))
	}
	var prefix []numbered
	for _, n := range all {
		if n.e.ID == sel {
			return n, nil
		}
		if len(sel) >= 4 && strings.HasPrefix(n.e.ID, sel) {
			prefix = append(prefix, n)
		}
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	found := filter(v, sel)
	// An exact issuer or "issuer:account" label wins over partial matches.
	var exact []numbered
	for _, n := range found {
		if strings.EqualFold(n.e.Issuer, sel) || strings.EqualFold(n.e.Label(), sel) {
			exact = append(exact, n)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	switch len(found) {
	case 0:
		return numbered{}, fmt.Errorf("no account matches %q", sel)
	case 1:
		return found[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d accounts, pick one by number:", sel, len(found))
	for _, n := range found {
		fmt.Fprintf(&b, "\n  %3d  %s", n.n, label(n.e))
	}
	return numbered{}, errors.New(b.String())
}

func label(e *vault.Entry) string {
	switch {
	case e.Issuer != "" && e.Account != "":
		return e.Issuer + " (" + e.Account + ")"
	case e.Issuer != "":
		return e.Issuer
	}
	return e.Account
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func table() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) }

func cmdList(g *globals, args []string) error {
	fs := newFlagSet("list", g)
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	rows := []row{}
	for _, n := range filter(v, strings.Join(args, " ")) {
		rows = append(rows, toRow(v, n))
	}
	if g.json {
		return printJSON(rows)
	}
	w := table()
	fmt.Fprintln(w, "#\tID\tISSUER\tACCOUNT\tFOLDER\tTYPE")
	for _, r := range rows {
		issuer := r.Issuer
		if r.Favorite {
			issuer = "* " + issuer
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", r.N, r.ID[:8], issuer, r.Account, r.Folder, strings.ToUpper(string(r.Type)))
	}
	return w.Flush()
}

func cmdCode(g *globals, args []string) error {
	fs := newFlagSet("code", g)
	cp := fs.Bool("copy", false, "copy the code to the clipboard instead of printing it")
	next := fs.Bool("next", false, "HOTP: advance the counter first and print the new code")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("which account? e.g. \"code github\" or \"code 3\" (see \"list\")")
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	var rows []row
	for _, sel := range args {
		n, err := selectOne(v, sel)
		if err != nil {
			return err
		}
		if *next && n.e.Type == otp.HOTP {
			if err := v.IncrementCounter(n.e.ID); err != nil {
				return err
			}
			n.e = v.Get(n.e.ID)
		}
		code, left, err := n.e.Code(time.Now())
		if err != nil {
			return err
		}
		r := toRow(v, n)
		r.Code, r.Remaining = code, left
		rows = append(rows, r)
	}
	if *cp {
		codes := make([]string, len(rows))
		for i, r := range rows {
			codes[i] = r.Code
		}
		if err := copyToClipboard(strings.Join(codes, "\n")); err != nil {
			return err
		}
		for _, r := range rows {
			fmt.Fprintf(os.Stderr, "copied the code of %s%s\n", label(v.Get(r.ID)), validFor(r))
		}
		return nil
	}
	if g.json {
		return printJSON(rows)
	}
	for _, r := range rows {
		fmt.Println(r.Code)
	}
	return nil
}

func validFor(r row) string {
	if r.Type == otp.HOTP {
		return ""
	}
	return fmt.Sprintf(", valid for %d s", r.Remaining)
}

func cmdCodes(g *globals, args []string) error {
	fs := newFlagSet("codes", g)
	watch := fs.Bool("watch", false, "keep refreshing until Ctrl+C")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	query := strings.Join(args, " ")
	collect := func() ([]row, error) {
		rows := []row{}
		now := time.Now()
		for _, n := range filter(v, query) {
			code, left, err := n.e.Code(now)
			if err != nil {
				return nil, err
			}
			r := toRow(v, n)
			r.Code, r.Remaining = code, left
			rows = append(rows, r)
		}
		return rows, nil
	}
	if !*watch {
		rows, err := collect()
		if err != nil {
			return err
		}
		if g.json {
			return printJSON(rows)
		}
		return printCodes(rows, false)
	}
	if g.json {
		return errors.New("-watch cannot be combined with -json")
	}
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	for {
		rows, err := collect()
		if err != nil {
			return err
		}
		if tty {
			fmt.Print("\x1b[H\x1b[2J")
		}
		if err := printCodes(rows, tty); err != nil {
			return err
		}
		time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	}
}

func printCodes(rows []row, bars bool) error {
	w := table()
	fmt.Fprintln(w, "#\tISSUER\tACCOUNT\tCODE\tLEFT\tFOLDER")
	for _, r := range rows {
		left := fmt.Sprintf("%2ds", r.Remaining)
		if r.Type == otp.HOTP {
			left = fmt.Sprintf("#%d", r.Counter)
		} else if bars && r.Period > 0 {
			filled := (r.Remaining*8 + r.Period - 1) / r.Period
			left += " " + strings.Repeat("■", filled) + strings.Repeat("·", 8-filled)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", r.N, r.Issuer, r.Account, otp.FormatCode(r.Code), left, r.Folder)
	}
	return w.Flush()
}

func cmdAdd(g *globals, args []string) error {
	fs := newFlagSet("add", g)
	folder := fs.String("folder", "", "put the accounts into this folder path, e.g. Work/Servers (created if missing)")
	issuer := fs.String("issuer", "", "manual entry: service name")
	account := fs.String("account", "", "manual entry: account name")
	hotp := fs.Bool("hotp", false, "manual entry: counter based (HOTP) instead of time based")
	digits := fs.Int("digits", 6, "manual entry: code length (6-8)")
	period := fs.Int("period", 30, "manual entry: TOTP period in seconds")
	alg := fs.String("algorithm", "SHA1", "manual entry: SHA1, SHA256 or SHA512")
	counter := fs.Uint64("counter", 0, "manual entry: initial HOTP counter")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	var keys []*otp.Key
	for _, a := range args {
		k, err := keysFrom(a)
		if err != nil {
			return fmt.Errorf("%s: %w", a, err)
		}
		keys = append(keys, k...)
	}
	if len(args) == 0 {
		if *issuer == "" && *account == "" {
			return errors.New("give an otpauth:// link, a QR image, or -issuer/-account for a manual entry")
		}
		secret, err := readHidden("Secret key (base32): ")
		if err != nil {
			return err
		}
		a, err := otp.ParseAlgorithm(*alg)
		if err != nil {
			return err
		}
		k := &otp.Key{Type: otp.TOTP, Issuer: *issuer, Account: *account, Secret: string(secret), Algorithm: a,
			Digits: *digits, Period: *period, Counter: *counter}
		if *hotp {
			k.Type = otp.HOTP
		}
		if err := k.Normalize(); err != nil {
			return err
		}
		keys = append(keys, k)
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	folderID, err := ensureFolder(v, *folder)
	if err != nil {
		return err
	}
	added, skipped, err := v.AddKeys(keys, folderID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "added %d, skipped %d (duplicates)\n", added, skipped)
	return nil
}

// keysFrom reads accounts from an otpauth(-migration) link or from QR codes in an image file.
func keysFrom(arg string) ([]*otp.Key, error) {
	if strings.Contains(arg, "://") {
		return otp.ParseAny(arg)
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, err
	}
	texts, err := qr.DecodeBytes(data)
	if err != nil {
		return nil, err
	}
	var keys []*otp.Key
	for _, t := range texts {
		k, err := otp.ParseAny(t)
		if err != nil {
			continue
		}
		keys = append(keys, k...)
	}
	if len(keys) == 0 {
		return nil, errors.New("the image has no otpauth QR codes")
	}
	return keys, nil
}

// ensureFolder returns the id of a slash separated folder path, creating missing levels.
func ensureFolder(v *vault.Vault, path string) (string, error) {
	parent := ""
	for _, part := range strings.Split(path, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id := ""
		for _, f := range v.Folders() {
			if f.Parent == parent && strings.EqualFold(f.Name, part) {
				id = f.ID
				break
			}
		}
		if id == "" {
			f, err := v.AddFolder(part, parent)
			if err != nil {
				return "", err
			}
			id = f.ID
		}
		parent = id
	}
	return parent, nil
}

func cmdRemove(g *globals, args []string) error {
	fs := newFlagSet("remove", g)
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("which account? see \"list\"")
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	var ids []string
	var names []string
	for _, sel := range args {
		n, err := selectOne(v, sel)
		if err != nil {
			return err
		}
		ids = append(ids, n.e.ID)
		names = append(names, label(n.e))
	}
	if !*yes && !confirm(fmt.Sprintf("Delete %s? This cannot be undone without a backup.", strings.Join(names, ", "))) {
		return errors.New("cancelled")
	}
	if err := v.DeleteIDs(ids); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "deleted %d\n", len(ids))
	return nil
}

func cmdExport(g *globals, args []string) error {
	fs := newFlagSet("export", g)
	showQR := fs.Bool("qr", false, "draw QR codes in the terminal")
	migration := fs.Bool("google", false, "Google Authenticator transfer links (otpauth-migration://)")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	var sel []*vault.Entry
	if len(args) == 1 {
		if n, err := selectOne(v, args[0]); err == nil {
			sel = []*vault.Entry{n.e}
		}
	}
	if sel == nil {
		for _, n := range filter(v, strings.Join(args, " ")) {
			sel = append(sel, n.e)
		}
	}
	if len(sel) == 0 {
		return errors.New("no matching accounts")
	}
	keys, err := vault.RevealKeys(sel)
	if err != nil {
		return err
	}
	var links, titles []string
	if *migration {
		links = otp.EncodeMigration(keys, 10)
		for i := range links {
			titles = append(titles, fmt.Sprintf("Google Authenticator transfer %d of %d", i+1, len(links)))
		}
	} else {
		for _, k := range keys {
			links = append(links, k.URI())
			titles = append(titles, k.Label())
		}
	}
	if g.json {
		return printJSON(links)
	}
	for i, l := range links {
		if *showQR {
			fmt.Println(titles[i])
			if err := printQR(l); err != nil {
				return err
			}
			continue
		}
		fmt.Println(l)
	}
	return nil
}

func cmdBackup(g *globals, args []string) error {
	fs := newFlagSet("backup", g)
	force := fs.Bool("force", false, "overwrite an existing file")
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.New("usage: backup <file.m2fab>")
	}
	dst := args[0]
	if filepath.Ext(dst) == "" {
		dst += ".m2fab"
	}
	if vault.Exists(dst) && !*force {
		return fmt.Errorf("%s exists (use -force to overwrite)", dst)
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	pw, err := newPassword("Backup password")
	if err != nil {
		return err
	}
	c := vault.Credentials{Password: pw}
	defer wipeCredentials(c)
	b, err := vault.Create(dst, c, vault.Options{Security: v.Security(), Overwrite: true})
	if err != nil {
		return err
	}
	defer b.Close()
	added, _, err := b.ImportFrom(v, v.Entries(), true)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "backup with %d accounts written to %s\n", added, dst)
	return nil
}

func cmdImport(g *globals, args []string) error {
	fs := newFlagSet("import", g)
	args, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(args) != 1 {
		return errors.New("usage: import <file.m2fab|file.m2fa>")
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	pw, err := readHidden("Password of " + filepath.Base(args[0]) + ": ")
	if err != nil {
		return err
	}
	c := vault.Credentials{Password: pw}
	defer wipeCredentials(c)
	src, err := vault.Open(args[0], c)
	if errors.Is(err, vault.ErrWrongPassword) {
		return errors.New("wrong password for " + args[0])
	}
	if err != nil {
		return err
	}
	defer src.Close()
	added, skipped, err := v.ImportFrom(src, src.Entries(), true)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "imported %d, skipped %d (duplicates)\n", added, skipped)
	return nil
}

func cmdPasswd(g *globals, args []string) error {
	fs := newFlagSet("passwd", g)
	if _, err := parseInterleaved(fs, args); err != nil {
		return err
	}
	v, err := openVault(g)
	if err != nil {
		return err
	}
	defer v.Close()
	pw, err := newPassword("New master password")
	if err != nil {
		return err
	}
	kf, err := keyFile(g)
	if err != nil {
		return err
	}
	c := vault.Credentials{Password: pw, KeyFile: kf}
	defer wipeCredentials(c)
	if err := v.ChangeCredentials(c, v.Security()); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "master password changed")
	return nil
}

func cmdInfo(g *globals, args []string) error {
	fs := newFlagSet("info", g)
	if _, err := parseInterleaved(fs, args); err != nil {
		return err
	}
	path, err := resolvePath(g)
	if err != nil {
		return err
	}
	info, err := vault.Inspect(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if g.json {
		return printJSON(map[string]any{"path": path, "format": info.Version, "kdf": info.KDF.Kind.String(),
			"kdf_params": info.KDF, "key_file": info.KeyFile})
	}
	w := table()
	fmt.Fprintf(w, "Vault\t%s\n", path)
	fmt.Fprintf(w, "Format\tversion %d\n", info.Version)
	fmt.Fprintf(w, "Key derivation\t%s\n", kdfSummary(info.KDF))
	fmt.Fprintf(w, "Key file\t%s\n", map[bool]string{true: "required", false: "no"}[info.KeyFile])
	fmt.Fprintf(w, "Cipher\tnot stored in the file, detected when unlocking\n")
	return w.Flush()
}

func kdfSummary(p vault.KDFParams) string {
	switch p.Kind {
	case vault.KDFArgon2id:
		return fmt.Sprintf("Argon2id, %d MiB, %d iterations, %d threads", p.MemoryKiB/1024, p.Iterations, p.Threads)
	case vault.KDFScrypt:
		return fmt.Sprintf("scrypt, N=2^%d, r=%d, p=%d", p.LogN, p.BlockSize, p.Threads)
	case vault.KDFPBKDF2:
		return fmt.Sprintf("PBKDF2-HMAC-SHA512, %d iterations", p.Iterations)
	}
	return p.Kind.String()
}

// printQR draws a QR code with half block characters, two modules per character row.
func printQR(text string) error {
	img, err := qr.Encode(text, 0)
	if err != nil {
		return err
	}
	b := img.Bounds()
	dark := func(x, y int) bool {
		if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
			return false
		}
		r, _, _, _ := img.At(x, y).RGBA()
		return r < 0x8000
	}
	var sb strings.Builder
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x++ {
			top, bottom := dark(x, y), dark(x, y+1)
			// Light modules are drawn, dark ones left blank, so the code reads on dark terminals.
			switch {
			case !top && !bottom:
				sb.WriteRune('█')
			case !top:
				sb.WriteRune('▀')
			case !bottom:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		sb.WriteByte('\n')
	}
	fmt.Print(sb.String())
	return nil
}
