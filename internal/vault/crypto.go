package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"reflect"
	"unsafe"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// File layout (integers big-endian):
//
//	magic[4] "M2FA" | version u8 | kdf u8 | p1 u32 | p2 u32 | p3 u8 | flags u8 | salt[32] | nonce[24] | ciphertext
//
// p1..p3 are the KDF parameters (see KDFParams.pack). The whole header is authenticated as AEAD
// associated data, so tampering with parameters, flags or salt fails decryption.
//
// Version 3 does not record the cipher: the client tries every supported mode and the
// authentication tag tells which one is right. Each mode uses its own HKDF subkey.
// Version 2 (XChaCha20-Poly1305 with the master key) and version 1 (no flags byte, raw password
// into Argon2id) are still read and upgraded on save.

const (
	magic        = "M2FA"
	formatV1     = 1
	formatV2     = 2
	formatV3     = 3
	saltSize     = 32
	keySize      = chacha20poly1305.KeySize
	nonceSize    = chacha20poly1305.NonceSizeX
	headerV1Size = 4 + 1 + 1 + 4 + 4 + 1 + saltSize + nonceSize
	headerV2Size = headerV1Size + 1
	padBlockSize = 4096

	flagKeyFile = 1 << 0
)

var (
	ErrWrongPassword = errors.New("wrong password, key file or corrupted vault")
	ErrNotVault      = errors.New("file is not a Multi2FA vault")
	ErrUnsupported   = errors.New("unsupported vault version")
	ErrLocked        = errors.New("vault is locked")
)

// CipherKind selects the authenticated encryption used for the vault file.
type CipherKind uint8

const (
	CipherXChaCha20 CipherKind = 1 // XChaCha20-Poly1305
	CipherAESGCM    CipherKind = 2 // AES-256-GCM
	CipherCascade   CipherKind = 3 // AES-256-GCM inside XChaCha20-Poly1305, independent keys
)

// Ciphers lists every mode in the order they are tried when opening a vault.
var Ciphers = []CipherKind{CipherXChaCha20, CipherAESGCM, CipherCascade}

func (c CipherKind) String() string {
	switch c {
	case CipherXChaCha20:
		return "XChaCha20-Poly1305"
	case CipherAESGCM:
		return "AES-256-GCM"
	case CipherCascade:
		return "AES-256-GCM + XChaCha20-Poly1305"
	}
	return "unknown"
}

// Security is the full protection setup of a vault file.
type Security struct {
	KDF    KDFParams
	Cipher CipherKind
}

// Credentials unlock a vault. The caller owns the slices and wipes them.
type Credentials struct {
	Password []byte
	KeyFile  []byte
}

// composite follows the KeePass scheme: SHA-256(SHA-256(password) || SHA-256(keyfile)).
func (c Credentials) composite() *memguard.LockedBuffer {
	out := memguard.NewBuffer(sha256.Size)
	parts := memguard.NewBuffer(2 * sha256.Size)
	defer parts.Destroy()
	p := parts.Bytes()
	h := sha256.New()
	h.Write(c.Password)
	h.Sum(p[:0])
	h.Reset()
	n := sha256.Size
	if len(c.KeyFile) > 0 {
		h.Write(c.KeyFile)
		h.Sum(p[n:n])
		h.Reset()
		n += sha256.Size
	}
	h.Write(p[:n])
	h.Sum(out.Bytes()[:0])
	h.Reset()
	return out
}

type header struct {
	version byte
	kdf     KDFParams
	flags   byte
	salt    []byte
	nonce   []byte
}

func (h *header) size() int {
	if h.version == formatV1 {
		return headerV1Size
	}
	return headerV2Size
}

// marshal always writes the current format.
func (h *header) marshal() []byte {
	p1, p2, p3 := h.kdf.pack()
	b := make([]byte, 0, headerV2Size)
	b = append(b, magic...)
	b = append(b, formatV3, byte(h.kdf.Kind))
	b = binary.BigEndian.AppendUint32(b, p1)
	b = binary.BigEndian.AppendUint32(b, p2)
	b = append(b, p3, h.flags)
	b = append(b, h.salt...)
	b = append(b, h.nonce...)
	return b
}

func parseHeader(data []byte) (*header, error) {
	if len(data) < 6 || !bytes.Equal(data[:4], []byte(magic)) {
		return nil, ErrNotVault
	}
	h := &header{version: data[4]}
	if h.version < formatV1 || h.version > formatV3 {
		return nil, ErrUnsupported
	}
	kind := KDFKind(data[5])
	if h.version < formatV3 && kind != KDFArgon2id {
		return nil, ErrUnsupported
	}
	if len(data) < h.size()+16 {
		return nil, ErrNotVault
	}
	h.kdf = unpackKDF(kind, binary.BigEndian.Uint32(data[6:10]), binary.BigEndian.Uint32(data[10:14]), data[14])
	off := 15
	if h.version >= formatV2 {
		h.flags = data[15]
		off = 16
	}
	h.salt = append([]byte(nil), data[off:off+saltSize]...)
	h.nonce = append([]byte(nil), data[off+saltSize:off+saltSize+nonceSize]...)
	return h, h.kdf.validate()
}

// deriveKey runs the KDF and moves the result straight into an encrypted enclave.
func deriveKey(c Credentials, h *header) *memguard.Enclave {
	if h.version == formatV1 {
		return memguard.NewEnclave(h.kdf.derive(c.Password, h.salt))
	}
	comp := c.composite()
	defer comp.Destroy()
	return memguard.NewEnclave(h.kdf.derive(comp.Bytes(), h.salt))
}

// sealer is one configured cipher mode.
type sealer struct {
	layers []aeadLayer
}

type aeadLayer struct {
	aead     cipher.AEAD
	nonceLen int
}

func (s *sealer) overhead() int {
	n := 0
	for _, l := range s.layers {
		n += l.aead.Overhead()
	}
	return n
}

// seal applies the layers from the inside out.
func (s *sealer) seal(nonce, plain, aad []byte, dst []byte) []byte {
	data := plain
	for i, l := range s.layers {
		if i == len(s.layers)-1 {
			return l.aead.Seal(dst, nonce[:l.nonceLen], data, aad)
		}
		data = l.aead.Seal(nil, nonce[:l.nonceLen], data, aad)
	}
	return nil
}

func (s *sealer) open(nonce, ct, aad, dst []byte) ([]byte, error) {
	data := ct
	for i := len(s.layers) - 1; i >= 0; i-- {
		l := s.layers[i]
		var out []byte
		var err error
		if i == 0 {
			out, err = l.aead.Open(dst, nonce[:l.nonceLen], data, aad)
		} else {
			out, err = l.aead.Open(nil, nonce[:l.nonceLen], data, aad)
		}
		if err != nil {
			return nil, err
		}
		data = out
	}
	return data, nil
}

func (s *sealer) wipe() {
	for _, l := range s.layers {
		wipeAEAD(l.aead)
	}
}

// subkey derives an independent key for one cipher layer into locked memory.
func subkey(master []byte, label string) *memguard.LockedBuffer {
	out := memguard.NewBuffer(keySize)
	r := hkdf.New(sha256.New, master, nil, []byte("multi2fa v3 "+label))
	if _, err := io.ReadFull(r, out.Bytes()); err != nil {
		panic(err)
	}
	return out
}

func layerXChaCha(key []byte) (aeadLayer, error) {
	a, err := chacha20poly1305.NewX(key)
	return aeadLayer{a, nonceSize}, err
}

// AES key schedules live inside crypto/aes and cannot be wiped; they only exist while a file
// is read or written.
func layerAESGCM(key []byte) (aeadLayer, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return aeadLayer{}, err
	}
	a, err := cipher.NewGCM(b)
	return aeadLayer{a, 12}, err
}

// newSealer builds the cipher for a file version and mode from the master key enclave.
func newSealer(key *memguard.Enclave, version byte, mode CipherKind) (*sealer, error) {
	lb, err := key.Open()
	if err != nil {
		return nil, ErrLocked
	}
	defer lb.Destroy()
	master := lb.Bytes()
	if version < formatV3 {
		l, err := layerXChaCha(master)
		if err != nil {
			return nil, err
		}
		return &sealer{layers: []aeadLayer{l}}, nil
	}
	build := func(label string, mk func([]byte) (aeadLayer, error)) (aeadLayer, error) {
		k := subkey(master, label)
		defer k.Destroy()
		return mk(k.Bytes())
	}
	var layers []aeadLayer
	switch mode {
	case CipherXChaCha20:
		l, err := build("xchacha20-poly1305", layerXChaCha)
		if err != nil {
			return nil, err
		}
		layers = []aeadLayer{l}
	case CipherAESGCM:
		l, err := build("aes-256-gcm", layerAESGCM)
		if err != nil {
			return nil, err
		}
		layers = []aeadLayer{l}
	case CipherCascade:
		inner, err := build("cascade aes-256-gcm", layerAESGCM)
		if err != nil {
			return nil, err
		}
		outer, err := build("cascade xchacha20-poly1305", layerXChaCha)
		if err != nil {
			return nil, err
		}
		layers = []aeadLayer{inner, outer}
	default:
		return nil, fmt.Errorf("unknown cipher %d", mode)
	}
	return &sealer{layers: layers}, nil
}

// newAEAD is the plain XChaCha20-Poly1305 used for in-memory secrets.
func newAEAD(key *memguard.Enclave) (cipher.AEAD, func(), error) {
	lb, err := key.Open()
	if err != nil {
		return nil, nil, ErrLocked
	}
	aead, err := chacha20poly1305.NewX(lb.Bytes())
	lb.Destroy()
	if err != nil {
		return nil, nil, err
	}
	return aead, func() { wipeAEAD(aead) }, nil
}

// wipeAEAD zeroes the key copy kept inside the x/crypto ChaCha20-Poly1305 value.
func wipeAEAD(a cipher.AEAD) {
	v := reflect.ValueOf(a)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	e := v.Elem()
	if e.Kind() == reflect.Struct && e.NumField() > 0 && e.Field(0).Type() == reflect.TypeOf([keySize]byte{}) {
		k := (*[keySize]byte)(unsafe.Pointer(e.Field(0).UnsafeAddr()))
		memguard.WipeBytes(k[:])
	}
}

// seal encrypts a padded payload with a fresh nonce.
func seal(key *memguard.Enclave, h *header, mode CipherKind, padded []byte) ([]byte, error) {
	s, err := newSealer(key, formatV3, mode)
	if err != nil {
		return nil, err
	}
	defer s.wipe()
	h.nonce = make([]byte, nonceSize)
	if _, err := rand.Read(h.nonce); err != nil {
		return nil, err
	}
	hdr := h.marshal()
	return s.seal(h.nonce, padded, hdr, hdr), nil
}

// open decrypts into a wiped-on-release scratch buffer. For version 3 every cipher mode is
// tried; the one that authenticates is returned.
func open(key *memguard.Enclave, h *header, data []byte) (*scratch, []byte, CipherKind, error) {
	n := h.size()
	hdr, nonce, ct := data[:n], data[n-nonceSize:n], data[n:]
	modes := Ciphers
	if h.version < formatV3 {
		modes = []CipherKind{CipherXChaCha20}
	}
	for _, mode := range modes {
		s, err := newSealer(key, h.version, mode)
		if err != nil {
			return nil, nil, 0, err
		}
		if len(ct) <= s.overhead() {
			s.wipe()
			continue
		}
		sc := newScratch(len(ct) - s.overhead())
		_, err = s.open(nonce, ct, hdr, sc.b[:0])
		s.wipe()
		if err != nil {
			sc.destroy()
			continue
		}
		p := sc.b
		if len(p) < 4 || uint64(binary.BigEndian.Uint32(p)) > uint64(len(p)-4) {
			sc.destroy()
			return nil, nil, 0, ErrNotVault
		}
		return sc, p[4 : 4+binary.BigEndian.Uint32(p)], mode, nil
	}
	return nil, nil, 0, ErrWrongPassword
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func wipe(b []byte) { memguard.WipeBytes(b) }
