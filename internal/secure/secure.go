// Package secure wraps the vetted primitives MNE Lab uses for data protection.
//
// Nothing here implements cryptography itself: symmetric encryption is
// XChaCha20-Poly1305 (golang.org/x/crypto), password hashing is Argon2id
// (golang.org/x/crypto/argon2), key derivation is HKDF-SHA256 (crypto/hkdf)
// and public-key sealing for recovery is HPKE (RFC 9180, crypto/hpke).
package secure

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hpke"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// KeySize is the size in bytes of every symmetric key.
const KeySize = 32

// Key is a 256-bit symmetric key.
type Key [KeySize]byte

// ErrDecrypt is returned when authenticated decryption fails. It never
// reveals whether the key or the data was wrong.
var ErrDecrypt = errors.New("secure: authentication failed")

// RandomBytes returns n bytes from the operating system CSPRNG.
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("secure: system random source failed: " + err.Error())
	}
	return b
}

// NewKey returns a fresh random key.
func NewKey() Key {
	var k Key
	copy(k[:], RandomBytes(KeySize))
	return k
}

// KeyFromBytes copies b into a Key. b must be KeySize bytes.
func KeyFromBytes(b []byte) (Key, error) {
	var k Key
	if len(b) != KeySize {
		return k, fmt.Errorf("secure: key must be %d bytes", KeySize)
	}
	copy(k[:], b)
	return k, nil
}

// Wipe overwrites the key in place.
func (k *Key) Wipe() {
	for i := range k {
		k[i] = 0
	}
}

// IsZero reports whether the key is all zeros (unset).
func (k Key) IsZero() bool {
	var z Key
	return subtle.ConstantTimeCompare(k[:], z[:]) == 1
}

// Seal encrypts and authenticates plaintext with XChaCha20-Poly1305 using a
// random 192-bit nonce. aad binds the ciphertext to its context (for example
// the record key) so ciphertexts cannot be swapped between records.
// Output layout: version(1) || nonce(24) || ciphertext+tag.
func Seal(k Key, plaintext, aad []byte) []byte {
	aead, err := chacha20poly1305.NewX(k[:])
	if err != nil {
		panic(err)
	}
	out := make([]byte, 1+chacha20poly1305.NonceSizeX, 1+chacha20poly1305.NonceSizeX+len(plaintext)+aead.Overhead())
	out[0] = 1
	copy(out[1:], RandomBytes(chacha20poly1305.NonceSizeX))
	return aead.Seal(out, out[1:1+chacha20poly1305.NonceSizeX], plaintext, aad)
}

// Open reverses Seal.
func Open(k Key, sealed, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(k[:])
	if err != nil {
		return nil, err
	}
	if len(sealed) < 1+chacha20poly1305.NonceSizeX+aead.Overhead() || sealed[0] != 1 {
		return nil, ErrDecrypt
	}
	nonce := sealed[1 : 1+chacha20poly1305.NonceSizeX]
	pt, err := aead.Open(nil, nonce, sealed[1+chacha20poly1305.NonceSizeX:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}

// SubKey derives an independent key for a named purpose with HKDF-SHA256.
func SubKey(master Key, purpose string) Key {
	b, err := hkdf.Key(sha256.New, master[:], nil, "mnelab/"+purpose, KeySize)
	if err != nil {
		panic(err)
	}
	k, _ := KeyFromBytes(b)
	return k
}

// KDFParams describes an Argon2id derivation. It is stored next to the data
// it protects so parameters can be raised in later versions.
type KDFParams struct {
	Alg       string `json:"alg"`
	Time      uint32 `json:"t"`
	MemoryKiB uint32 `json:"m"`
	Threads   uint8  `json:"p"`
	Salt      []byte `json:"salt"`
}

// DefaultKDF returns Argon2id parameters tuned to stay under about a second
// on inexpensive dual-core machines while exceeding the OWASP minimum
// (19 MiB, t=2).
func DefaultKDF() KDFParams {
	return KDFParams{Alg: "argon2id", Time: 2, MemoryKiB: 64 * 1024, Threads: 2, Salt: RandomBytes(16)}
}

// DeriveKey derives a key from a secret (password or passphrase).
func DeriveKey(secret []byte, p KDFParams) (Key, error) {
	var k Key
	if p.Alg != "argon2id" || p.Time == 0 || p.MemoryKiB < 8*1024 || p.Threads == 0 || len(p.Salt) < 16 {
		return k, errors.New("secure: unsupported key derivation parameters")
	}
	copy(k[:], argon2.IDKey(secret, p.Salt, p.Time, p.MemoryKiB, p.Threads, KeySize))
	return k, nil
}

// Hash returns the SHA-256 of b.
func Hash(b []byte) [32]byte { return sha256.Sum256(b) }

// HashHex returns the lowercase hex SHA-256 of b.
func HashHex(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}

// B64 encodes with unpadded URL-safe base64.
func B64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// UnB64 decodes unpadded URL-safe base64.
func UnB64(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// NewID returns a random 128-bit identifier as 26 lowercase base32 chars.
func NewID() string {
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(RandomBytes(16)))
}

// Token returns a random 256-bit bearer token.
func Token() string { return B64(RandomBytes(32)) }

// EqualStrings compares secrets in constant time.
func EqualStrings(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ---- Recovery key -------------------------------------------------------

var recoveryAlphabet = base32.NewEncoding("ABCDEFGHJKLMNPQRSTUVWXYZ23456789").WithPadding(base32.NoPadding)

// RecoveryKey is a 256-bit secret shown to the user once, at account
// creation. It is never stored by MNE Lab.
type RecoveryKey struct{ raw Key }

// NewRecoveryKey generates a new recovery key.
func NewRecoveryKey() RecoveryKey { return RecoveryKey{raw: NewKey()} }

// String renders the key as 13 groups of 4 characters from an alphabet
// without ambiguous symbols (no 0/O, 1/I).
func (r RecoveryKey) String() string {
	s := recoveryAlphabet.EncodeToString(r.raw[:]) // 52 chars
	var parts []string
	for i := 0; i < len(s); i += 4 {
		parts = append(parts, s[i:min(i+4, len(s))])
	}
	return strings.Join(parts, "-")
}

// ParseRecoveryKey accepts the printed form, tolerating spaces, dashes and
// lowercase. The alphabet excludes 0, 1, O and I, so those are rejected
// rather than guessed.
func ParseRecoveryKey(s string) (RecoveryKey, error) {
	var r RecoveryKey
	clean := strings.Map(func(c rune) rune {
		if c == '-' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			return -1
		}
		return c
	}, strings.ToUpper(s))
	b, err := recoveryAlphabet.DecodeString(clean)
	if err != nil || len(b) != KeySize {
		return r, errors.New("secure: recovery key is not valid")
	}
	copy(r.raw[:], b)
	return r, nil
}

func (r RecoveryKey) privateKey() (*ecdh.PrivateKey, error) {
	seed := SubKey(r.raw, "recovery/x25519")
	return ecdh.X25519().NewPrivateKey(seed[:])
}

// PublicKey returns the X25519 public key bound to this recovery key.
func (r RecoveryKey) PublicKey() ([]byte, error) {
	pk, err := r.privateKey()
	if err != nil {
		return nil, err
	}
	return pk.PublicKey().Bytes(), nil
}

// Wipe clears the recovery key from memory.
func (r *RecoveryKey) Wipe() { r.raw.Wipe() }

// SealToRecovery encrypts plaintext so that only the holder of the recovery
// key can read it (HPKE, DHKEM-X25519, HKDF-SHA256, ChaCha20-Poly1305).
func SealToRecovery(recoveryPub []byte, info string, plaintext []byte) ([]byte, error) {
	pub, err := ecdh.X25519().NewPublicKey(recoveryPub)
	if err != nil {
		return nil, err
	}
	hp, err := hpke.NewDHKEMPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return hpke.Seal(hp, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte("mnelab/"+info), plaintext)
}

// OpenWithRecovery reverses SealToRecovery.
func OpenWithRecovery(r RecoveryKey, info string, sealed []byte) ([]byte, error) {
	priv, err := r.privateKey()
	if err != nil {
		return nil, err
	}
	hp, err := hpke.NewDHKEMPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	pt, err := hpke.Open(hp, hpke.HKDFSHA256(), hpke.ChaCha20Poly1305(), []byte("mnelab/"+info), sealed)
	if err != nil {
		return nil, ErrDecrypt
	}
	return pt, nil
}
