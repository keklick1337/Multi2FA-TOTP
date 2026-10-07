package otp

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"strings"
	"time"
)

type Type string

const (
	TOTP Type = "totp"
	HOTP Type = "hotp"
)

type Algorithm string

const (
	SHA1   Algorithm = "SHA1"
	SHA256 Algorithm = "SHA256"
	SHA512 Algorithm = "SHA512"
	MD5    Algorithm = "MD5"
)

const (
	DefaultDigits = 6
	DefaultPeriod = 30
)

var (
	ErrEmptySecret   = errors.New("secret is empty")
	ErrInvalidSecret = errors.New("secret is not valid base32")
	ErrBadDigits     = errors.New("digits must be between 6 and 10")
	ErrBadPeriod     = errors.New("period must be between 1 and 3600 seconds")
	ErrBadAlgorithm  = errors.New("unsupported algorithm")
	ErrBadType       = errors.New("type must be totp or hotp")
)

// Key holds everything needed to generate codes for one account.
type Key struct {
	Type      Type      `json:"type"`
	Issuer    string    `json:"issuer"`
	Account   string    `json:"account"`
	Secret    string    `json:"secret"`
	Algorithm Algorithm `json:"algorithm"`
	Digits    int       `json:"digits"`
	Period    int       `json:"period"`
	Counter   uint64    `json:"counter"`
}

// NormalizeSecret uppercases a base32 secret and strips spaces, dashes and padding.
func NormalizeSecret(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '-', '=':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func DecodeSecret(s string) ([]byte, error) {
	s = NormalizeSecret(s)
	if s == "" {
		return nil, ErrEmptySecret
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil || len(raw) == 0 {
		return nil, ErrInvalidSecret
	}
	return raw, nil
}

func EncodeSecret(raw []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
}

func ParseAlgorithm(s string) (Algorithm, error) {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", "")) {
	case "", "SHA1":
		return SHA1, nil
	case "SHA256":
		return SHA256, nil
	case "SHA512":
		return SHA512, nil
	case "MD5":
		return MD5, nil
	}
	return "", ErrBadAlgorithm
}

func (a Algorithm) hasher() (func() hash.Hash, error) {
	switch a {
	case SHA1, "":
		return sha1.New, nil
	case SHA256:
		return sha256.New, nil
	case SHA512:
		return sha512.New, nil
	case MD5:
		return md5.New, nil
	}
	return nil, ErrBadAlgorithm
}

// Normalize fills defaults and validates the key including the secret.
func (k *Key) Normalize() error {
	if err := k.NormalizeParams(); err != nil {
		return err
	}
	k.Secret = NormalizeSecret(k.Secret)
	_, err := DecodeSecret(k.Secret)
	return err
}

// NormalizeParams fills defaults and validates everything except the secret.
func (k *Key) NormalizeParams() error {
	k.Issuer = strings.TrimSpace(k.Issuer)
	k.Account = strings.TrimSpace(k.Account)
	if k.Type == "" {
		k.Type = TOTP
	}
	if k.Type != TOTP && k.Type != HOTP {
		return ErrBadType
	}
	alg, err := ParseAlgorithm(string(k.Algorithm))
	if err != nil {
		return err
	}
	k.Algorithm = alg
	if k.Digits == 0 {
		k.Digits = DefaultDigits
	}
	if k.Digits < 6 || k.Digits > 10 {
		return ErrBadDigits
	}
	if k.Period == 0 {
		k.Period = DefaultPeriod
	}
	if k.Period < 1 || k.Period > 3600 {
		return ErrBadPeriod
	}
	return nil
}

// HOTPCode implements RFC 4226.
func HOTPCode(secret []byte, counter uint64, digits int, alg Algorithm) (string, error) {
	h, err := alg.hasher()
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(h, secret)
	mac.Write(msg[:])
	return Truncate(mac.Sum(nil), digits), nil
}

// Hash returns the hash constructor for the algorithm.
func (a Algorithm) Hash() (func() hash.Hash, error) { return a.hasher() }

// Truncate applies the RFC 4226 dynamic truncation to an HMAC result.
func Truncate(sum []byte, digits int) string {
	if digits == 0 {
		digits = DefaultDigits
	}
	off := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint64(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, uint64(bin)%mod)
}

// Moment returns the moving factor for now and, for TOTP, the seconds left in the step.
func (k *Key) Moment(now time.Time) (counter uint64, remaining int) {
	if k.Type == HOTP {
		return k.Counter, 0
	}
	period := int64(k.Period)
	if period <= 0 {
		period = DefaultPeriod
	}
	ts := now.Unix()
	return uint64(ts / period), int(period - ts%period)
}

// Code returns the current code and, for TOTP, how many seconds it stays valid.
func (k *Key) Code(now time.Time) (code string, remaining int, err error) {
	secret, err := DecodeSecret(k.Secret)
	if err != nil {
		return "", 0, err
	}
	counter, remaining := k.Moment(now)
	code, err = HOTPCode(secret, counter, orDigits(k.Digits), k.Algorithm)
	return code, remaining, err
}

func orDigits(d int) int {
	if d == 0 {
		return DefaultDigits
	}
	return d
}

// Label is the human readable "Issuer (account)" form.
func (k *Key) Label() string {
	switch {
	case k.Issuer != "" && k.Account != "":
		return k.Issuer + " (" + k.Account + ")"
	case k.Issuer != "":
		return k.Issuer
	default:
		return k.Account
	}
}

// FormatCode splits a code into readable groups: 123 456, 1234 5678.
func FormatCode(code string) string {
	switch len(code) {
	case 6:
		return code[:3] + " " + code[3:]
	case 7:
		return code[:3] + " " + code[3:]
	case 8:
		return code[:4] + " " + code[4:]
	case 9:
		return code[:3] + " " + code[3:6] + " " + code[6:]
	case 10:
		return code[:5] + " " + code[5:]
	}
	return code
}
