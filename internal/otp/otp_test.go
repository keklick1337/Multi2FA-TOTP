package otp

import (
	"encoding/base64"
	"encoding/binary"
	"net/url"
	"testing"
	"time"
)

func TestRFC6238(t *testing.T) {
	seeds := map[Algorithm]string{
		SHA1:   "12345678901234567890",
		SHA256: "12345678901234567890123456789012",
		SHA512: "1234567890123456789012345678901234567890123456789012345678901234",
	}
	cases := []struct {
		ts   int64
		alg  Algorithm
		want string
	}{
		{59, SHA1, "94287082"},
		{59, SHA256, "46119246"},
		{59, SHA512, "90693936"},
		{1111111109, SHA1, "07081804"},
		{1234567890, SHA256, "91819424"},
		{20000000000, SHA512, "47863826"},
	}
	for _, c := range cases {
		k := Key{Secret: EncodeSecret([]byte(seeds[c.alg])), Algorithm: c.alg, Digits: 8, Period: 30}
		got, _, err := k.Code(time.Unix(c.ts, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s @%d: got %s want %s", c.alg, c.ts, got, c.want)
		}
	}
}

func TestRFC4226(t *testing.T) {
	want := []string{"755224", "287082", "359152", "969429", "338314"}
	for i, w := range want {
		got, err := HOTPCode([]byte("12345678901234567890"), uint64(i), 6, SHA1)
		if err != nil || got != w {
			t.Errorf("counter %d: got %s want %s (%v)", i, got, w, err)
		}
	}
}

func TestParseURI(t *testing.T) {
	k, err := ParseURI("otpauth://totp/ACME%20Co:john.doe@email.com?secret=hxdm vjec jjws rb3h wizr 4ifu gftm xboz&issuer=ACME%20Co&algorithm=SHA256&digits=8&period=60")
	if err != nil {
		t.Fatal(err)
	}
	if k.Issuer != "ACME Co" || k.Account != "john.doe@email.com" || k.Digits != 8 || k.Period != 60 || k.Algorithm != SHA256 {
		t.Fatalf("unexpected key: %+v", k)
	}
	if k.Secret != "HXDMVJECJJWSRB3HWIZR4IFUGFTMXBOZ" {
		t.Fatalf("secret not normalized: %s", k.Secret)
	}
	back, err := ParseURI(k.URI())
	if err != nil || *back != *k {
		t.Fatalf("roundtrip mismatch: %+v vs %+v (%v)", back, k, err)
	}
}

func TestParseURIErrors(t *testing.T) {
	for _, s := range []string{
		"https://example.com",
		"otpauth://foo/x?secret=JBSWY3DPEHPK3PXP",
		"otpauth://totp/x",
		"otpauth://totp/x?secret=!!!",
		"otpauth://totp/x?secret=JBSWY3DPEHPK3PXP&digits=3",
	} {
		if _, err := ParseURI(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func protoBytes(field int, b []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3|2))
	out = binary.AppendUvarint(out, uint64(len(b)))
	return append(out, b...)
}

func protoVarint(field int, v uint64) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3))
	return binary.AppendUvarint(out, v)
}

func TestMigration(t *testing.T) {
	var p1 []byte
	p1 = append(p1, protoBytes(1, []byte("12345678901234567890"))...)
	p1 = append(p1, protoBytes(2, []byte("GitHub:alice"))...)
	p1 = append(p1, protoBytes(3, []byte("GitHub"))...)
	p1 = append(p1, protoVarint(4, 1)...)
	p1 = append(p1, protoVarint(5, 1)...)
	p1 = append(p1, protoVarint(6, 2)...)

	var p2 []byte
	p2 = append(p2, protoBytes(1, []byte("abcdefghij"))...)
	p2 = append(p2, protoBytes(2, []byte("bob@example.com"))...)
	p2 = append(p2, protoVarint(4, 2)...)
	p2 = append(p2, protoVarint(5, 2)...)
	p2 = append(p2, protoVarint(6, 1)...)
	p2 = append(p2, protoVarint(7, 42)...)

	payload := append(protoBytes(1, p1), protoBytes(1, p2)...)
	payload = append(payload, protoVarint(2, 1)...)
	uri := "otpauth-migration://offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(payload))

	keys, err := ParseAny(uri)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys", len(keys))
	}
	if keys[0].Issuer != "GitHub" || keys[0].Account != "alice" || keys[0].Type != TOTP || keys[0].Digits != 6 {
		t.Errorf("key0: %+v", keys[0])
	}
	if keys[1].Account != "bob@example.com" || keys[1].Type != HOTP || keys[1].Counter != 42 || keys[1].Digits != 8 || keys[1].Algorithm != SHA256 {
		t.Errorf("key1: %+v", keys[1])
	}
}

func TestMigrationRoundTrip(t *testing.T) {
	var keys []*Key
	for i := 0; i < 23; i++ {
		k := &Key{Issuer: "Svc", Account: string(rune('a' + i)), Secret: EncodeSecret([]byte{byte(i), 1, 2, 3, 4, 5, 6, 7, 8, 9}), Digits: 6, Period: 30, Algorithm: SHA1, Type: TOTP}
		keys = append(keys, k)
	}
	uris := EncodeMigration(keys, 10)
	if len(uris) != 3 {
		t.Fatalf("got %d batches", len(uris))
	}
	var got []*Key
	for _, u := range uris {
		ks, err := ParseAny(u)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ks...)
	}
	for i := range keys {
		if *got[i] != *keys[i] {
			t.Fatalf("key %d: %+v != %+v", i, got[i], keys[i])
		}
	}
}
