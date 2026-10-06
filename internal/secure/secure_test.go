package secure

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	k := NewKey()
	ct := Seal(k, []byte("hello"), []byte("ctx"))
	pt, err := Open(k, ct, []byte("ctx"))
	if err != nil || string(pt) != "hello" {
		t.Fatalf("round trip failed: %v %q", err, pt)
	}
	if _, err := Open(k, ct, []byte("other")); err == nil {
		t.Fatal("aad mismatch must fail")
	}
	ct[len(ct)-1] ^= 1
	if _, err := Open(k, ct, []byte("ctx")); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	if _, err := Open(NewKey(), Seal(k, []byte("x"), nil), nil); err == nil {
		t.Fatal("wrong key must fail")
	}
}

func TestNoncesDiffer(t *testing.T) {
	k := NewKey()
	if bytes.Equal(Seal(k, []byte("a"), nil), Seal(k, []byte("a"), nil)) {
		t.Fatal("two seals of same plaintext must differ")
	}
}

func TestDeriveKeyDeterministicAndSalted(t *testing.T) {
	p := DefaultKDF()
	p.MemoryKiB = 8 * 1024
	a, _ := DeriveKey([]byte("pw"), p)
	b, _ := DeriveKey([]byte("pw"), p)
	if a != b {
		t.Fatal("same input must derive same key")
	}
	p2 := p
	p2.Salt = RandomBytes(16)
	c, _ := DeriveKey([]byte("pw"), p2)
	if a == c {
		t.Fatal("different salt must derive different key")
	}
	if _, err := DeriveKey([]byte("pw"), KDFParams{Alg: "md5"}); err == nil {
		t.Fatal("unsupported params must be rejected")
	}
}

func TestRecoveryKeyFormatAndSeal(t *testing.T) {
	rk := NewRecoveryKey()
	s := rk.String()
	if len(strings.Split(s, "-")) != 13 {
		t.Fatalf("unexpected format %s", s)
	}
	for i := 0; i < 20; i++ {
		k := NewRecoveryKey()
		if p, err := ParseRecoveryKey(strings.ToLower(strings.ReplaceAll(k.String(), "-", " "))); err != nil || p.raw != k.raw {
			t.Fatalf("lowercase parse failed for %s", k)
		}
	}
	parsed, err := ParseRecoveryKey(s)
	if err != nil || parsed.raw != rk.raw {
		t.Fatalf("parse failed: %v", err)
	}
	pub, _ := rk.PublicKey()
	sealed, err := SealToRecovery(pub, "profile", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := OpenWithRecovery(parsed, "profile", sealed)
	if err != nil || string(pt) != "secret" {
		t.Fatalf("open failed: %v", err)
	}
	if _, err := OpenWithRecovery(NewRecoveryKey(), "profile", sealed); err == nil {
		t.Fatal("other recovery key must fail")
	}
	if _, err := ParseRecoveryKey("not-a-key"); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
