package vault

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/awnumar/memguard"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
)

// Entry is one account. Its secret never stays in clear in memory: Key.Secret is empty and the
// raw secret lives encrypted in sealed (see secretBox).
type Entry struct {
	ID string `json:"id"`
	otp.Key
	Folder   string    `json:"folder,omitempty"`
	Notes    string    `json:"notes,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Favorite bool      `json:"favorite,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`

	sealed []byte
	fp     [32]byte
	box    *secretBox
}

// Folder groups entries; Parent is empty for top level folders.
type Folder struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Parent  string    `json:"parent,omitempty"`
	Created time.Time `json:"created"`
}

type Settings struct {
	AutoLockMinutes    int  `json:"auto_lock_minutes"`
	ClipboardClearSecs int  `json:"clipboard_clear_secs"`
	LockOnFocusLoss    bool `json:"lock_on_focus_loss"`
	HideCodes          bool `json:"hide_codes"`
}

var DefaultSettings = Settings{AutoLockMinutes: 5, ClipboardClearSecs: 30}

// storedEntry is the on-disk form. Version 1 files carry Secret (base32), version 2 carries Key (raw).
type storedEntry struct {
	ID        string        `json:"id"`
	Type      otp.Type      `json:"type"`
	Issuer    string        `json:"issuer"`
	Account   string        `json:"account"`
	Algorithm otp.Algorithm `json:"algorithm"`
	Digits    int           `json:"digits"`
	Period    int           `json:"period"`
	Counter   uint64        `json:"counter"`
	Secret    string        `json:"secret,omitempty"`
	Folder    string        `json:"folder,omitempty"`
	Notes     string        `json:"notes,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
	Favorite  bool          `json:"favorite,omitempty"`
	Created   time.Time     `json:"created"`
	Updated   time.Time     `json:"updated"`
	Key       []byte        `json:"key,omitempty"`
}

type payloadHead struct {
	Version  int       `json:"version"`
	Folders  []*Folder `json:"folders"`
	Settings Settings  `json:"settings"`
}

type payloadIn struct {
	payloadHead
	Entries []storedEntry `json:"entries"`
}

type Vault struct {
	mu      sync.Mutex
	path    string
	key     *memguard.Enclave
	hdr     *header
	cipher  CipherKind
	box     *secretBox
	entries []*Entry
	folders []*Folder
	set     Settings
}

// Options control vault creation. A zero Security means DefaultSecurity.
type Options struct {
	Security  Security
	Overwrite bool
}

// Info is what can be read from a vault header without the password.
type Info struct {
	Version byte
	KDF     KDFParams
	KeyFile bool
}

var (
	ErrDuplicate      = errors.New("an account with the same secret already exists")
	ErrFolderNotFound = errors.New("folder not found")
	ErrFolderCycle    = errors.New("a folder cannot be moved into itself")
)

func Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

func DefaultPath() (string, error) {
	if p := os.Getenv("MULTI2FA_VAULT"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Multi2FA", "vault.m2fa"), nil
}

// Inspect reads the header of a vault file, e.g. to know whether a key file is needed.
func Inspect(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	buf := make([]byte, headerV2Size+64)
	n, _ := f.Read(buf)
	h, err := parseHeader(buf[:n])
	if err != nil {
		return Info{}, err
	}
	return Info{Version: h.version, KDF: h.kdf, KeyFile: h.flags&flagKeyFile != 0}, nil
}

// Create makes a new empty vault protected by the given credentials.
func Create(path string, c Credentials, opt Options) (*Vault, error) {
	if Exists(path) && !opt.Overwrite {
		return nil, fmt.Errorf("vault already exists: %s", path)
	}
	sec, err := normalizeSecurity(opt.Security)
	if err != nil {
		return nil, err
	}
	salt, err := randomBytes(saltSize)
	if err != nil {
		return nil, err
	}
	h := &header{version: formatV3, kdf: sec.KDF, salt: salt}
	if len(c.KeyFile) > 0 {
		h.flags |= flagKeyFile
	}
	v := &Vault{path: path, hdr: h, cipher: sec.Cipher, box: newSecretBox(), set: DefaultSettings}
	v.key = deriveKey(c, h)
	if err := v.Save(); err != nil {
		v.Close()
		return nil, err
	}
	return v, nil
}

func Open(path string, c Credentials) (*Vault, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	h, err := parseHeader(data)
	if err != nil {
		return nil, err
	}
	key := deriveKey(c, h)
	sc, plain, mode, err := open(key, h, data)
	if err != nil {
		return nil, err
	}
	defer sc.destroy()

	v := &Vault{path: path, hdr: h, key: key, cipher: mode, box: newSecretBox()}
	if err := v.load(plain); err != nil {
		v.Close()
		return nil, err
	}
	if h.version < formatV3 {
		// Upgrade to format 3. Version 1 hashed the raw password, so the key is derived again.
		v.hdr = &header{version: formatV3, kdf: h.kdf, flags: h.flags, salt: h.salt}
		if len(c.KeyFile) > 0 {
			v.hdr.flags |= flagKeyFile
		}
		if h.version == formatV1 {
			v.key = deriveKey(c, v.hdr)
		}
		if err := v.Save(); err != nil {
			v.Close()
			return nil, err
		}
	}
	return v, nil
}

func (v *Vault) load(plain []byte) error {
	var p payloadIn
	if err := json.Unmarshal(plain, &p); err != nil {
		return fmt.Errorf("vault payload: %w", err)
	}
	v.set = p.Settings
	v.folders = p.Folders
	for i := range p.Entries {
		s := &p.Entries[i]
		raw := s.Key
		if len(raw) == 0 && s.Secret != "" {
			var err error
			if raw, err = otp.DecodeSecret(s.Secret); err != nil {
				return err
			}
		}
		e := &Entry{
			ID: s.ID,
			Key: otp.Key{Type: s.Type, Issuer: s.Issuer, Account: s.Account, Algorithm: s.Algorithm,
				Digits: s.Digits, Period: s.Period, Counter: s.Counter},
			Folder: s.Folder, Notes: s.Notes, Tags: s.Tags, Favorite: s.Favorite, Created: s.Created, Updated: s.Updated,
		}
		err := v.attachSecretLocked(e, raw)
		wipe(raw)
		if err != nil {
			return err
		}
		v.entries = append(v.entries, e)
	}
	return nil
}

func (v *Vault) attachSecretLocked(e *Entry, raw []byte) error {
	sealed, err := v.box.seal(raw)
	if err != nil {
		return err
	}
	fp, err := v.box.fingerprint(raw)
	if err != nil {
		return err
	}
	e.sealed, e.fp, e.box, e.Secret = sealed, fp, v.box, ""
	return nil
}

func (v *Vault) Path() string { return v.path }

// Security returns the key derivation and cipher currently protecting the vault.
func (v *Vault) Security() Security {
	v.mu.Lock()
	defer v.mu.Unlock()
	return Security{KDF: v.hdr.kdf, Cipher: v.cipher}
}

func normalizeSecurity(s Security) (Security, error) {
	if s.KDF == (KDFParams{}) {
		s.KDF = DefaultKDF
	}
	if s.Cipher == 0 {
		s.Cipher = CipherXChaCha20
	}
	if s.Cipher < CipherXChaCha20 || s.Cipher > CipherCascade {
		return s, fmt.Errorf("unknown cipher %d", s.Cipher)
	}
	return s, s.KDF.validate()
}

func (v *Vault) KeyFileRequired() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.hdr.flags&flagKeyFile != 0
}

// Save re-encrypts the vault with a fresh nonce and replaces the file atomically.
func (v *Vault) Save() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.saveLocked()
}

// saveLocked serializes into locked memory only. encoding/json is used for the non-secret parts;
// secrets are base64 encoded straight into the locked buffer.
func (v *Vault) saveLocked() error {
	if v.key == nil {
		return ErrLocked
	}
	head, err := json.Marshal(payloadHead{Version: formatV2, Folders: v.folders, Settings: v.set})
	if err != nil {
		return err
	}
	head = append(head[:len(head)-1], `,"entries":[`...)
	metas := make([][]byte, len(v.entries))
	size := len(head) + 2
	for i, e := range v.entries {
		m, err := json.Marshal(storedEntry{
			ID: e.ID, Type: e.Type, Issuer: e.Issuer, Account: e.Account, Algorithm: e.Algorithm,
			Digits: e.Digits, Period: e.Period, Counter: e.Counter, Folder: e.Folder, Notes: e.Notes,
			Tags: e.Tags, Favorite: e.Favorite, Created: e.Created, Updated: e.Updated,
		})
		if err != nil {
			return err
		}
		metas[i] = append(m[:len(m)-1], `,"key":"`...)
		rawLen := len(e.sealed) - nonceSize - 16
		size += len(metas[i]) + base64.StdEncoding.EncodedLen(rawLen) + 2 + 1
	}
	total := 4 + size
	padded := (total/padBlockSize + 1) * padBlockSize
	sc := newScratch(padded)
	defer sc.destroy()
	buf := sc.b
	off := 4
	put := func(b []byte) { off += copy(buf[off:], b) }
	put(head)
	for i, e := range v.entries {
		if i > 0 {
			put([]byte{','})
		}
		put(metas[i])
		raw, err := e.box.open(e.sealed)
		if err != nil {
			return err
		}
		n := base64.StdEncoding.EncodedLen(raw.Size())
		base64.StdEncoding.Encode(buf[off:off+n], raw.Bytes())
		raw.Destroy()
		off += n
		put([]byte(`"}`))
	}
	put([]byte(`]}`))
	binary.BigEndian.PutUint32(buf, uint32(off-4))

	data, err := seal(v.key, v.hdr, v.cipher, buf)
	if err != nil {
		return err
	}
	return writeAtomic(v.path, data)
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".vault-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if Exists(path) {
		_ = copyFile(path, path+".bak")
	}
	return os.Rename(tmpName, path)
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o600)
}

// VerifyCredentials checks credentials against the open vault in constant time.
func (v *Vault) VerifyCredentials(c Credentials) bool {
	v.mu.Lock()
	hdr, key := v.hdr, v.key
	v.mu.Unlock()
	if key == nil {
		return false
	}
	test := deriveKey(c, hdr)
	a, err1 := test.Open()
	b, err2 := key.Open()
	if err1 != nil || err2 != nil {
		return false
	}
	defer a.Destroy()
	defer b.Destroy()
	return subtle.ConstantTimeCompare(a.Bytes(), b.Bytes()) == 1
}

// ChangeCredentials re-keys the vault with new credentials, a new salt, KDF and cipher.
func (v *Vault) ChangeCredentials(c Credentials, sec Security) error {
	sec, err := normalizeSecurity(sec)
	if err != nil {
		return err
	}
	salt, err := randomBytes(saltSize)
	if err != nil {
		return err
	}
	h := &header{version: formatV3, kdf: sec.KDF, salt: salt}
	if len(c.KeyFile) > 0 {
		h.flags |= flagKeyFile
	}
	key := deriveKey(c, h)
	v.mu.Lock()
	defer v.mu.Unlock()
	oldKey, oldHdr, oldCipher := v.key, v.hdr, v.cipher
	v.key, v.hdr, v.cipher = key, h, sec.Cipher
	if err := v.saveLocked(); err != nil {
		v.key, v.hdr, v.cipher = oldKey, oldHdr, oldCipher
		return err
	}
	return nil
}

// Close drops the key, the session box and every decrypted structure. Sealed secrets that may
// still be referenced elsewhere become useless because their session key is gone.
func (v *Vault) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.key = nil
	v.box.destroy()
	for _, e := range v.entries {
		e.sealed, e.box = nil, nil
		e.fp = [32]byte{}
	}
	v.entries, v.folders = nil, nil
}

func (v *Vault) Settings() Settings {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.set
}

func (v *Vault) SetSettings(s Settings) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.set = s
	return v.saveLocked()
}

// Entries returns a sorted snapshot: favorites first, then by issuer and account.
func (v *Vault) Entries() []*Entry {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := slices.Clone(v.entries)
	slices.SortStableFunc(out, func(a, b *Entry) int {
		if a.Favorite != b.Favorite {
			if a.Favorite {
				return -1
			}
			return 1
		}
		if c := strings.Compare(strings.ToLower(a.Issuer), strings.ToLower(b.Issuer)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Account), strings.ToLower(b.Account))
	})
	return out
}

func (v *Vault) Get(id string) *Entry {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.getLocked(id)
}

func (v *Vault) getLocked(id string) *Entry {
	for _, e := range v.entries {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (v *Vault) hasFPLocked(fp [32]byte, exceptID string) bool {
	for _, e := range v.entries {
		if e.ID != exceptID && subtle.ConstantTimeCompare(e.fp[:], fp[:]) == 1 {
			return true
		}
	}
	return false
}

// HasSecret reports whether a base32 secret is already stored.
func (v *Vault) HasSecret(secret string) bool {
	raw, err := otp.DecodeSecret(secret)
	if err != nil {
		return false
	}
	defer wipe(raw)
	v.mu.Lock()
	defer v.mu.Unlock()
	fp, err := v.box.fingerprint(raw)
	return err == nil && v.hasFPLocked(fp, "")
}

// Contains reports whether an entry of another vault already exists here.
func (v *Vault) Contains(e *Entry) bool {
	raw, err := e.box.open(e.sealed)
	if err != nil {
		return false
	}
	defer raw.Destroy()
	v.mu.Lock()
	defer v.mu.Unlock()
	fp, err := v.box.fingerprint(raw.Bytes())
	return err == nil && v.hasFPLocked(fp, "")
}

func newID() (string, error) {
	b, err := randomBytes(12)
	return hex.EncodeToString(b), err
}

func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Second) }

// Add stores a new entry; e.Key.Secret must hold the base32 secret and is cleared afterwards.
func (v *Vault) Add(e *Entry) error {
	if err := e.Key.Normalize(); err != nil {
		return err
	}
	raw, err := otp.DecodeSecret(e.Secret)
	if err != nil {
		return err
	}
	defer wipe(raw)
	v.mu.Lock()
	defer v.mu.Unlock()
	fp, err := v.box.fingerprint(raw)
	if err != nil {
		return err
	}
	if v.hasFPLocked(fp, "") {
		return ErrDuplicate
	}
	if e.ID, err = newID(); err != nil {
		return err
	}
	if err := v.attachSecretLocked(e, raw); err != nil {
		return err
	}
	now := nowUTC()
	e.Created, e.Updated = now, now
	e.Tags = cleanTags(e.Tags)
	if v.folderLocked(e.Folder) == nil {
		e.Folder = ""
	}
	v.entries = append(v.entries, e)
	if err := v.saveLocked(); err != nil {
		v.entries = v.entries[:len(v.entries)-1]
		return err
	}
	return nil
}

// Update replaces the stored entry with the same ID. An empty Key.Secret keeps the current secret.
func (v *Vault) Update(e *Entry) error {
	newSecret := strings.TrimSpace(e.Secret) != ""
	if newSecret {
		if err := e.Key.Normalize(); err != nil {
			return err
		}
	} else if err := e.Key.NormalizeParams(); err != nil {
		return err
	}
	var raw []byte
	if newSecret {
		var err error
		if raw, err = otp.DecodeSecret(e.Secret); err != nil {
			return err
		}
		defer wipe(raw)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	i := slices.IndexFunc(v.entries, func(x *Entry) bool { return x.ID == e.ID })
	if i < 0 {
		return errors.New("entry not found")
	}
	old := v.entries[i]
	if newSecret {
		fp, err := v.box.fingerprint(raw)
		if err != nil {
			return err
		}
		if v.hasFPLocked(fp, e.ID) {
			return ErrDuplicate
		}
		if err := v.attachSecretLocked(e, raw); err != nil {
			return err
		}
	} else {
		e.sealed, e.fp, e.box, e.Secret = old.sealed, old.fp, old.box, ""
	}
	e.Created = old.Created
	e.Updated = nowUTC()
	e.Tags = cleanTags(e.Tags)
	if v.folderLocked(e.Folder) == nil {
		e.Folder = ""
	}
	v.entries[i] = e
	if err := v.saveLocked(); err != nil {
		v.entries[i] = old
		return err
	}
	return nil
}

func (v *Vault) Delete(id string) error { return v.DeleteIDs([]string{id}) }

// DeleteIDs removes several entries with a single save.
func (v *Vault) DeleteIDs(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	old := slices.Clone(v.entries)
	v.entries = slices.DeleteFunc(v.entries, func(e *Entry) bool { return slices.Contains(ids, e.ID) })
	if err := v.saveLocked(); err != nil {
		v.entries = old
		return err
	}
	return nil
}

// IncrementCounter advances a HOTP counter and persists it.
func (v *Vault) IncrementCounter(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	e := v.getLocked(id)
	if e == nil {
		return errors.New("entry not found")
	}
	e.Counter++
	if err := v.saveLocked(); err != nil {
		e.Counter--
		return err
	}
	return nil
}

// RevealKeys returns full keys with secrets, e.g. for transfer QR codes.
func RevealKeys(entries []*Entry) ([]*otp.Key, error) {
	out := make([]*otp.Key, 0, len(entries))
	for _, e := range entries {
		k, err := e.Reveal()
		if err != nil {
			return nil, err
		}
		out = append(out, &k)
	}
	return out, nil
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Destroy permanently deletes a vault and its .bak copy. Used when the password is forgotten.
func Destroy(path string) error {
	if err := removeFile(path); err != nil {
		return err
	}
	return removeFile(path + ".bak")
}

func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// Matches reports whether every whitespace-separated term of q appears in the entry.
func (e *Entry) Matches(q string) bool {
	terms := strings.Fields(strings.ToLower(q))
	if len(terms) == 0 {
		return true
	}
	hay := strings.ToLower(e.Issuer + "\x00" + e.Account + "\x00" + e.Notes + "\x00" + strings.Join(e.Tags, "\x00"))
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}
