package vault

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
)

var testKDF = KDFParams{Kind: KDFArgon2id, Iterations: 1, MemoryKiB: 8 * 1024, Threads: 1}

func pw(s string) Credentials { return Credentials{Password: []byte(s)} }

func newVault(t *testing.T, c Credentials) *Vault {
	t.Helper()
	v, err := Create(filepath.Join(t.TempDir(), "v.m2fa"), c, Options{Security: Security{KDF: testKDF}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLifecycle(t *testing.T) {
	v := newVault(t, pw("correct horse"))
	path := v.Path()
	e := &Entry{Key: otp.Key{Issuer: "GitHub", Account: "alice", Secret: "jbsw y3dp ehpk 3pxp"}, Tags: []string{"work", " work ", ""}}
	if err := v.Add(e); err != nil {
		t.Fatal(err)
	}
	if e.Secret != "" {
		t.Fatal("plain secret kept in entry")
	}
	if err := v.Add(&Entry{Key: otp.Key{Secret: "JBSWY3DPEHPK3PXP"}}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	v.Close()

	if _, err := Open(path, pw("wrong")); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("expected wrong password, got %v", err)
	}
	v, err := Open(path, pw("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	got := v.Entries()
	if len(got) != 1 || len(got[0].Tags) != 1 || got[0].Secret != "" {
		t.Fatalf("unexpected entries: %+v", got)
	}
	k, err := got[0].Reveal()
	if err != nil || k.Secret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("reveal: %v %v", k.Secret, err)
	}
	if !v.HasSecret("jbswy3dpehpk3pxp") {
		t.Fatal("HasSecret failed")
	}
	if !v.VerifyCredentials(pw("correct horse")) || v.VerifyCredentials(pw("nope")) {
		t.Fatal("VerifyCredentials mismatch")
	}
	if err := v.ChangeCredentials(pw("new pass"), Security{KDF: testKDF, Cipher: CipherCascade}); err != nil {
		t.Fatal(err)
	}
	v.Close()
	if _, _, err := got[0].Code(time.Now()); !errors.Is(err, ErrLocked) {
		t.Fatalf("code generated after Close: %v", err)
	}
	if _, err := Open(path, pw("correct horse")); err == nil {
		t.Fatal("old password still works")
	}
	if _, err := Open(path, pw("new pass")); err != nil {
		t.Fatal(err)
	}
}

func TestCodesMatchReference(t *testing.T) {
	v := newVault(t, pw("x"))
	for _, k := range []otp.Key{
		{Secret: otp.EncodeSecret([]byte("12345678901234567890")), Digits: 8, Algorithm: otp.SHA1},
		{Secret: otp.EncodeSecret([]byte("12345678901234567890123456789012")), Digits: 8, Algorithm: otp.SHA256},
		{Secret: otp.EncodeSecret([]byte("1234567890123456789012345678901234567890123456789012345678901234")), Digits: 8, Algorithm: otp.SHA512},
		{Secret: otp.EncodeSecret([]byte("abc")), Digits: 6, Type: otp.HOTP, Counter: 7},
	} {
		ref := k
		e := &Entry{Key: k}
		if err := v.Add(e); err != nil {
			t.Fatal(err)
		}
		_ = ref.Normalize()
		for _, ts := range []int64{59, 1111111109, 2000000000} {
			want, _, _ := ref.Code(time.Unix(ts, 0))
			got, _, err := e.Code(time.Unix(ts, 0))
			if err != nil || got != want {
				t.Fatalf("%s @%d: got %s want %s (%v)", k.Algorithm, ts, got, want, err)
			}
		}
	}
}

func TestKeyFile(t *testing.T) {
	kf := []byte("random key file content")
	v := newVault(t, Credentials{Password: []byte("pw"), KeyFile: kf})
	path := v.Path()
	v.Close()
	info, err := Inspect(path)
	if err != nil || !info.KeyFile || info.Version != formatV3 {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	if _, err := Open(path, pw("pw")); err == nil {
		t.Fatal("opened without key file")
	}
	if _, err := Open(path, Credentials{Password: []byte("pw"), KeyFile: []byte("other")}); err == nil {
		t.Fatal("opened with wrong key file")
	}
	if _, err := Open(path, Credentials{Password: []byte("pw"), KeyFile: kf}); err != nil {
		t.Fatal(err)
	}
}

func TestTamper(t *testing.T) {
	v := newVault(t, pw("pw"))
	path := v.Path()
	v.Close()
	data, _ := os.ReadFile(path)
	for _, off := range []int{8, 15, 20, headerV2Size + 5, len(data) - 1} {
		bad := append([]byte(nil), data...)
		bad[off] ^= 1
		os.WriteFile(path, bad, 0o600)
		if _, err := Open(path, pw("pw")); err == nil {
			t.Fatalf("tampered byte %d not detected", off)
		}
	}
}

// writeV1 produces a vault in the original format: raw password into Argon2id, base32 secrets.
func writeV1(t *testing.T, path, password string, secrets []string) {
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	key := argon2.IDKey([]byte(password), salt, testKDF.Iterations, testKDF.MemoryKiB, testKDF.Threads, keySize)
	var entries []map[string]any
	for i, s := range secrets {
		entries = append(entries, map[string]any{"id": string(rune('a' + i)), "type": "totp", "issuer": "Old", "secret": s, "digits": 6, "period": 30, "algorithm": "SHA1"})
	}
	plain, _ := json.Marshal(map[string]any{"version": 1, "entries": entries, "settings": DefaultSettings})
	padded := make([]byte, 4096)
	binary.BigEndian.PutUint32(padded, uint32(len(plain)))
	copy(padded[4:], plain)
	hdr := []byte(magic)
	hdr = append(hdr, formatV1, byte(KDFArgon2id))
	hdr = binary.BigEndian.AppendUint32(hdr, testKDF.Iterations)
	hdr = binary.BigEndian.AppendUint32(hdr, testKDF.MemoryKiB)
	hdr = append(hdr, testKDF.Threads)
	hdr = append(hdr, salt...)
	hdr = append(hdr, nonce...)
	aead, _ := chacha20poly1305.NewX(key)
	if err := os.WriteFile(path, aead.Seal(hdr, nonce, padded, hdr), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.m2fa")
	writeV1(t, path, "legacy", []string{"JBSWY3DPEHPK3PXP"})
	v, err := Open(path, pw("legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Entries()) != 1 || !v.HasSecret("JBSWY3DPEHPK3PXP") {
		t.Fatal("v1 entries not loaded")
	}
	v.Close()
	if info, _ := Inspect(path); info.Version != formatV3 {
		t.Fatalf("not upgraded: %+v", info)
	}
	if _, err := Open(path, pw("legacy")); err != nil {
		t.Fatalf("upgraded vault does not open: %v", err)
	}
}

func TestFolders(t *testing.T) {
	v := newVault(t, pw("pw"))
	work, _ := v.AddFolder("Work", "")
	servers, err := v.AddFolder("Servers", work.ID)
	if err != nil {
		t.Fatal(err)
	}
	prod, _ := v.AddFolder("Prod", servers.ID)
	e := &Entry{Key: otp.Key{Issuer: "AWS", Secret: "JBSWY3DPEHPK3PXP"}, Folder: prod.ID}
	if err := v.Add(e); err != nil {
		t.Fatal(err)
	}
	if p := v.FolderPath(prod.ID); len(p) != 3 || p[2] != "Prod" {
		t.Fatalf("path %v", p)
	}
	if err := v.MoveFolder(work.ID, prod.ID); !errors.Is(err, ErrFolderCycle) {
		t.Fatalf("cycle not detected: %v", err)
	}
	if err := v.DeleteFolder(servers.ID); err != nil {
		t.Fatal(err)
	}
	if v.Folder(prod.ID).Parent != work.ID {
		t.Fatal("subfolder not moved up")
	}
	if err := v.DeleteFolder(prod.ID); err != nil {
		t.Fatal(err)
	}
	if v.Get(e.ID).Folder != work.ID {
		t.Fatal("entry not moved up")
	}
	if err := v.MoveEntries([]string{e.ID}, ""); err != nil || v.Get(e.ID).Folder != "" {
		t.Fatal("move to root failed")
	}

	path := v.Path()
	v.Close()
	v, _ = Open(path, pw("pw"))
	if len(v.Folders()) != 1 || v.Folders()[0].Name != "Work" {
		t.Fatalf("folders not persisted: %+v", v.Folders())
	}
}

func TestImportFromKeepsFolders(t *testing.T) {
	src := newVault(t, pw("a"))
	dst := newVault(t, pw("b"))
	f1, _ := src.AddFolder("Work", "")
	f2, _ := src.AddFolder("Mail", f1.ID)
	src.AddFolder("Empty", "")
	e := &Entry{Key: otp.Key{Issuer: "Gmail", Secret: "JBSWY3DPEHPK3PXP"}, Folder: f2.ID, Notes: "n"}
	src.Add(e)
	existing, _ := dst.AddFolder("work", "")

	added, skipped, err := dst.ImportFrom(src, src.Entries(), true)
	if err != nil || added != 1 || skipped != 0 {
		t.Fatalf("added=%d skipped=%d err=%v", added, skipped, err)
	}
	got := dst.Entries()[0]
	if p := dst.FolderPath(got.Folder); len(p) != 2 || got.Notes != "n" {
		t.Fatalf("path %v", p)
	}
	if dst.Folder(got.Folder).Parent != existing.ID {
		t.Fatal("existing folder with the same name was not reused")
	}
	if len(dst.Folders()) != 3 {
		t.Fatalf("empty folder not copied: %d", len(dst.Folders()))
	}
	if added, skipped, _ = dst.ImportFrom(src, src.Entries(), true); added != 0 || skipped != 1 {
		t.Fatal("duplicate imported")
	}
	if !dst.Contains(e) {
		t.Fatal("Contains failed")
	}
	src.Close()
	if c1, _, err := got.Code(time.Now()); err != nil || c1 == "" {
		t.Fatal("imported entry does not depend on the source vault")
	}
}

func TestAddKeys(t *testing.T) {
	v := newVault(t, pw("pw"))
	keys := []*otp.Key{{Secret: "JBSWY3DPEHPK3PXP"}, {Secret: "KRSXG5CTMVRXEZLU"}, {Secret: "!"}, {Secret: "JBSWY3DPEHPK3PXP"}}
	added, skipped, err := v.AddKeys(keys, "")
	if err != nil || added != 2 || skipped != 2 {
		t.Fatalf("added=%d skipped=%d err=%v", added, skipped, err)
	}
	if keys[0].Secret == "" {
		t.Fatal("caller's key was modified")
	}
}

func TestCalibrate(t *testing.T) {
	for _, k := range KDFs {
		p := Calibrate(k, Level{Target: 30 * time.Millisecond, MemoryKiB: 8 * 1024})
		if err := p.validate(); err != nil {
			t.Fatalf("%s: %+v %v", k, p, err)
		}
	}
}

func TestDestroy(t *testing.T) {
	v := newVault(t, pw("pw"))
	v.Save()
	path := v.Path()
	v.Close()
	if err := Destroy(path); err != nil || Exists(path) || Exists(path+".bak") {
		t.Fatal("destroy failed")
	}
}

func TestAllKDFsAndCiphers(t *testing.T) {
	kdfs := []KDFParams{
		testKDF,
		{Kind: KDFScrypt, LogN: 10, BlockSize: 8, Threads: 1},
		{Kind: KDFPBKDF2, Iterations: 10000},
	}
	for _, kdf := range kdfs {
		for _, mode := range Ciphers {
			path := filepath.Join(t.TempDir(), "v.m2fa")
			v, err := Create(path, pw("pw"), Options{Security: Security{KDF: kdf, Cipher: mode}})
			if err != nil {
				t.Fatalf("%s/%s: %v", kdf.Kind, mode, err)
			}
			v.Add(&Entry{Key: otp.Key{Issuer: "X", Secret: "JBSWY3DPEHPK3PXP"}})
			v.Close()
			if _, err := Open(path, pw("bad")); !errors.Is(err, ErrWrongPassword) {
				t.Fatalf("%s/%s: wrong password accepted: %v", kdf.Kind, mode, err)
			}
			v, err = Open(path, pw("pw"))
			if err != nil {
				t.Fatalf("%s/%s: %v", kdf.Kind, mode, err)
			}
			got := v.Security()
			if got.Cipher != mode || got.KDF != kdf || len(v.Entries()) != 1 {
				t.Fatalf("%s/%s: detected %+v", kdf.Kind, mode, got)
			}
			v.Close()
		}
	}
}

func TestUpgradeFromV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v2.m2fa")
	salt := make([]byte, saltSize)
	nonce := make([]byte, nonceSize)
	comp := pw("pw2").composite()
	key := testKDF.derive(comp.Bytes(), salt)
	comp.Destroy()
	plain := []byte(`{"version":2,"folders":[{"id":"f","name":"Work"}],"settings":{},"entries":[{"id":"a","type":"totp","issuer":"Old","digits":6,"period":30,"algorithm":"SHA1","folder":"f","key":"SGVsbG8hIQ=="}]}`)
	padded := make([]byte, 4096)
	binary.BigEndian.PutUint32(padded, uint32(len(plain)))
	copy(padded[4:], plain)
	hdr := []byte(magic)
	hdr = append(hdr, formatV2, byte(KDFArgon2id))
	hdr = binary.BigEndian.AppendUint32(hdr, testKDF.Iterations)
	hdr = binary.BigEndian.AppendUint32(hdr, testKDF.MemoryKiB)
	hdr = append(hdr, testKDF.Threads, 0)
	hdr = append(hdr, salt...)
	hdr = append(hdr, nonce...)
	aead, _ := chacha20poly1305.NewX(key)
	os.WriteFile(path, aead.Seal(hdr, nonce, padded, hdr), 0o600)

	v, err := Open(path, pw("pw2"))
	if err != nil {
		t.Fatal(err)
	}
	if e := v.Entries(); len(e) != 1 || v.FolderPath(e[0].Folder)[0] != "Work" {
		t.Fatal("v2 content not loaded")
	}
	v.Close()
	if info, _ := Inspect(path); info.Version != formatV3 {
		t.Fatal("v2 not upgraded")
	}
	if _, err := Open(path, pw("pw2")); err != nil {
		t.Fatal(err)
	}
}
