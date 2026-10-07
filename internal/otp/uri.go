package otp

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

var ErrNotOTPAuth = errors.New("not an otpauth URI")

// ParseURI parses an otpauth://totp|hotp/LABEL?secret=... URI
// (Google Authenticator Key Uri Format).
func ParseURI(raw string) (*Key, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(u.Scheme, "otpauth") {
		return nil, ErrNotOTPAuth
	}
	k := &Key{Type: Type(strings.ToLower(u.Host))}
	if k.Type != TOTP && k.Type != HOTP {
		return nil, fmt.Errorf("%w: %q", ErrBadType, u.Host)
	}

	label := strings.TrimPrefix(u.Path, "/")
	if p, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/")); err == nil {
		label = p
	}
	labelIssuer, account := splitLabel(label)
	k.Account = account

	q := u.Query()
	k.Secret = q.Get("secret")
	k.Issuer = strings.TrimSpace(q.Get("issuer"))
	if k.Issuer == "" {
		k.Issuer = labelIssuer
	}
	if a := q.Get("algorithm"); a != "" {
		alg, err := ParseAlgorithm(a)
		if err != nil {
			return nil, err
		}
		k.Algorithm = alg
	}
	if d := q.Get("digits"); d != "" {
		if k.Digits, err = strconv.Atoi(d); err != nil {
			return nil, ErrBadDigits
		}
	}
	if p := q.Get("period"); p != "" {
		if k.Period, err = strconv.Atoi(p); err != nil {
			return nil, ErrBadPeriod
		}
	}
	if c := q.Get("counter"); c != "" {
		if k.Counter, err = strconv.ParseUint(c, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid counter: %w", err)
		}
	}
	if err := k.Normalize(); err != nil {
		return nil, err
	}
	return k, nil
}

func splitLabel(label string) (issuer, account string) {
	if i := strings.Index(label, ":"); i >= 0 {
		return strings.TrimSpace(label[:i]), strings.TrimSpace(label[i+1:])
	}
	return "", strings.TrimSpace(label)
}

// URI builds an otpauth URI that other authenticator apps can import.
func (k *Key) URI() string {
	label := k.Account
	if k.Issuer != "" {
		label = k.Issuer + ":" + k.Account
	}
	q := url.Values{}
	q.Set("secret", NormalizeSecret(k.Secret))
	if k.Issuer != "" {
		q.Set("issuer", k.Issuer)
	}
	alg := k.Algorithm
	if alg == "" {
		alg = SHA1
	}
	q.Set("algorithm", string(alg))
	digits := k.Digits
	if digits == 0 {
		digits = DefaultDigits
	}
	q.Set("digits", strconv.Itoa(digits))
	typ := k.Type
	if typ == "" {
		typ = TOTP
	}
	if typ == HOTP {
		q.Set("counter", strconv.FormatUint(k.Counter, 10))
	} else {
		period := k.Period
		if period == 0 {
			period = DefaultPeriod
		}
		q.Set("period", strconv.Itoa(period))
	}
	u := url.URL{
		Scheme:   "otpauth",
		Host:     string(typ),
		Path:     "/" + label,
		RawQuery: strings.ReplaceAll(q.Encode(), "+", "%20"),
	}
	return u.String()
}

// ParseAny accepts an otpauth:// or otpauth-migration:// payload and returns all keys in it.
func ParseAny(raw string) ([]*Key, error) {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "otpauth-migration://"):
		return ParseMigration(raw)
	case strings.HasPrefix(lower, "otpauth://"):
		k, err := ParseURI(raw)
		if err != nil {
			return nil, err
		}
		return []*Key{k}, nil
	}
	return nil, ErrNotOTPAuth
}
