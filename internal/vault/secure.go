package vault

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"time"

	"github.com/awnumar/memguard"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
)

// secretBox keeps OTP secrets encrypted in memory with a random per-session key.
// A secret is decrypted into locked memory only while a code is computed.
type secretBox struct {
	key   *memguard.Enclave
	fpKey *memguard.Enclave
}

func newSecretBox() *secretBox {
	return &secretBox{key: memguard.NewEnclaveRandom(keySize), fpKey: memguard.NewEnclaveRandom(sha256.Size)}
}

func (b *secretBox) seal(raw []byte) ([]byte, error) {
	if b == nil || b.key == nil {
		return nil, ErrLocked
	}
	aead, done, err := newAEAD(b.key)
	if err != nil {
		return nil, err
	}
	defer done()
	nonce := make([]byte, nonceSize, nonceSize+len(raw)+aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, raw, nil), nil
}

func (b *secretBox) open(sealed []byte) (*memguard.LockedBuffer, error) {
	if b == nil || b.key == nil || len(sealed) <= nonceSize+chacha20poly1305.Overhead {
		return nil, ErrLocked
	}
	aead, done, err := newAEAD(b.key)
	if err != nil {
		return nil, err
	}
	defer done()
	lb := memguard.NewBuffer(len(sealed) - nonceSize - aead.Overhead())
	if _, err := aead.Open(lb.Bytes()[:0], sealed[:nonceSize], sealed[nonceSize:], nil); err != nil {
		lb.Destroy()
		return nil, ErrLocked
	}
	return lb, nil
}

// fingerprint identifies a secret for duplicate detection without keeping it in clear.
func (b *secretBox) fingerprint(raw []byte) ([32]byte, error) {
	var fp [32]byte
	lb, err := b.fpKey.Open()
	if err != nil {
		return fp, ErrLocked
	}
	defer lb.Destroy()
	copy(fp[:], lockedHMAC(sha256.New, lb.Bytes(), raw, nil))
	return fp, nil
}

func (b *secretBox) destroy() {
	if b == nil {
		return
	}
	b.key, b.fpKey = nil, nil
}

// lockedHMAC is HMAC with the key-derived pads kept in locked memory; crypto/hmac would leave
// them on the regular heap. Hash states are reset afterwards so they hold no key material.
func lockedHMAC(newHash func() hash.Hash, key, msg, out []byte) []byte {
	h := newHash()
	bs := h.BlockSize()
	pads := memguard.NewBuffer(2 * bs)
	defer pads.Destroy()
	ipad, opad := pads.Bytes()[:bs], pads.Bytes()[bs:]
	if len(key) > bs {
		short := memguard.NewBuffer(h.Size())
		defer short.Destroy()
		h.Write(key)
		h.Sum(short.Bytes()[:0])
		h.Reset()
		key = short.Bytes()
	}
	copy(ipad, key)
	copy(opad, key)
	for i := range ipad {
		ipad[i] ^= 0x36
		opad[i] ^= 0x5c
	}
	h.Write(ipad)
	h.Write(msg)
	inner := h.Sum(nil)
	h.Reset()
	h.Write(opad)
	h.Write(inner)
	out = h.Sum(out)
	h.Reset()
	return out
}

// Code computes the current OTP for an entry.
func (e *Entry) Code(now time.Time) (code string, remaining int, err error) {
	if e.box == nil || e.sealed == nil {
		return "", 0, ErrLocked
	}
	newHash, err := e.Algorithm.Hash()
	if err != nil {
		return "", 0, err
	}
	raw, err := e.box.open(e.sealed)
	if err != nil {
		return "", 0, err
	}
	defer raw.Destroy()
	counter, remaining := e.Key.Moment(now)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	return otp.Truncate(lockedHMAC(newHash, raw.Bytes(), msg[:], nil), e.Digits), remaining, nil
}

// Reveal returns the full key including the base32 secret, for showing or exporting it.
// The returned string cannot be wiped, so call it only on explicit user request.
func (e *Entry) Reveal() (otp.Key, error) {
	k := e.Key
	raw, err := e.box.open(e.sealed)
	if err != nil {
		return k, err
	}
	defer raw.Destroy()
	k.Secret = otp.EncodeSecret(raw.Bytes())
	return k, nil
}
