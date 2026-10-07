package otp

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Google Authenticator "Transfer accounts" export: otpauth-migration://offline?data=<base64 protobuf>.
// The protobuf schema is tiny, so it is decoded by hand instead of pulling in a protobuf runtime.

var errProto = errors.New("malformed migration payload")

func ParseMigration(raw string) ([]*Key, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(u.Scheme, "otpauth-migration") {
		return nil, ErrNotOTPAuth
	}
	data := u.Query().Get("data")
	if data == "" {
		return nil, errors.New("migration URI has no data")
	}
	data = strings.ReplaceAll(data, " ", "+")
	buf, err := decodeB64(data)
	if err != nil {
		return nil, fmt.Errorf("migration data: %w", err)
	}

	var keys []*Key
	err = walkProto(buf, func(field int, wire int, v uint64, b []byte) error {
		if field != 1 || wire != 2 {
			return nil
		}
		k, err := parseOtpParameters(b)
		if err != nil {
			return err
		}
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("migration payload contains no accounts")
	}
	return keys, nil
}

func decodeB64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func parseOtpParameters(buf []byte) (*Key, error) {
	k := &Key{Type: TOTP, Algorithm: SHA1, Digits: 6, Period: DefaultPeriod}
	var name string
	err := walkProto(buf, func(field int, wire int, v uint64, b []byte) error {
		switch field {
		case 1:
			k.Secret = EncodeSecret(b)
		case 2:
			name = string(b)
		case 3:
			k.Issuer = string(b)
		case 4:
			switch v {
			case 2:
				k.Algorithm = SHA256
			case 3:
				k.Algorithm = SHA512
			case 4:
				k.Algorithm = MD5
			}
		case 5:
			if v == 2 {
				k.Digits = 8
			}
		case 6:
			if v == 1 {
				k.Type = HOTP
			}
		case 7:
			k.Counter = v
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	labelIssuer, account := splitLabel(name)
	k.Account = account
	if k.Issuer == "" {
		k.Issuer = labelIssuer
	} else if labelIssuer != "" && !strings.EqualFold(labelIssuer, k.Issuer) {
		k.Account = name
	}
	if err := k.Normalize(); err != nil {
		return nil, err
	}
	return k, nil
}

func walkProto(buf []byte, fn func(field, wire int, v uint64, b []byte) error) error {
	for len(buf) > 0 {
		tag, n := binary.Uvarint(buf)
		if n <= 0 {
			return errProto
		}
		buf = buf[n:]
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			v, n := binary.Uvarint(buf)
			if n <= 0 {
				return errProto
			}
			buf = buf[n:]
			if err := fn(field, wire, v, nil); err != nil {
				return err
			}
		case 1:
			if len(buf) < 8 {
				return errProto
			}
			buf = buf[8:]
		case 2:
			l, n := binary.Uvarint(buf)
			if n <= 0 || uint64(len(buf)-n) < l {
				return errProto
			}
			b := buf[n : n+int(l)]
			buf = buf[n+int(l):]
			if err := fn(field, wire, 0, b); err != nil {
				return err
			}
		case 5:
			if len(buf) < 4 {
				return errProto
			}
			buf = buf[4:]
		default:
			return errProto
		}
	}
	return nil
}
