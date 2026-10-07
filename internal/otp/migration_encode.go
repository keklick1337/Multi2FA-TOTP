package otp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"net/url"
)

// EncodeMigration builds otpauth-migration:// URIs that Google Authenticator can scan.
// Accounts are split into batches so each QR code stays readable.
func EncodeMigration(keys []*Key, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = 10
	}
	var idb [4]byte
	_, _ = rand.Read(idb[:])
	batchID := uint64(binary.BigEndian.Uint32(idb[:]) & 0x7fffffff)
	total := (len(keys) + batchSize - 1) / batchSize
	var out []string
	for i := 0; i < total; i++ {
		var p []byte
		for _, k := range keys[i*batchSize : min(len(keys), (i+1)*batchSize)] {
			p = appendBytes(p, 1, encodeParams(k))
		}
		p = appendVarint(p, 2, 1)
		p = appendVarint(p, 3, uint64(total))
		p = appendVarint(p, 4, uint64(i))
		p = appendVarint(p, 5, batchID)
		out = append(out, "otpauth-migration://offline?data="+url.QueryEscape(base64.StdEncoding.EncodeToString(p)))
	}
	return out
}

func encodeParams(k *Key) []byte {
	secret, _ := DecodeSecret(k.Secret)
	name := k.Account
	if k.Issuer != "" {
		name = k.Issuer + ":" + k.Account
	}
	var p []byte
	p = appendBytes(p, 1, secret)
	p = appendBytes(p, 2, []byte(name))
	p = appendBytes(p, 3, []byte(k.Issuer))
	alg := map[Algorithm]uint64{SHA1: 1, SHA256: 2, SHA512: 3, MD5: 4}[k.Algorithm]
	if alg == 0 {
		alg = 1
	}
	p = appendVarint(p, 4, alg)
	digits := uint64(1)
	if k.Digits == 8 {
		digits = 2
	}
	p = appendVarint(p, 5, digits)
	if k.Type == HOTP {
		p = appendVarint(p, 6, 1)
		p = appendVarint(p, 7, k.Counter)
	} else {
		p = appendVarint(p, 6, 2)
	}
	return p
}

func appendBytes(b []byte, field int, v []byte) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3|2))
	b = binary.AppendUvarint(b, uint64(len(v)))
	return append(b, v...)
}

func appendVarint(b []byte, field int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3))
	return binary.AppendUvarint(b, v)
}
