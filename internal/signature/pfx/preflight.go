// Package pfx opens PKCS#12 (.pfx/.p12) signing credentials in a separate,
// short-lived worker process and signs SHA-256 digests with them.
//
// A file is first checked without its password: size, DER framing, the
// algorithms it names and every key derivation cost that is visible before
// decryption. Only then does a worker started from the application's own
// executable receive the file and password over a private pipe. The private
// key never leaves the worker, which signs nothing but 32-byte digests and
// is killed when it exceeds its deadlines. Nothing here writes files, logs
// or passes secrets in arguments or the environment.
package pfx

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
)

var (
	ErrMalformed     = errors.New("pfx.malformed")
	ErrUnsupported   = errors.New("pfx.unsupported")
	ErrLimit         = errors.New("pfx.limit")
	ErrWrongPassword = errors.New("pfx.wrong_password")
	ErrCredential    = errors.New("pfx.credential")
	ErrWorker        = errors.New("pfx.worker")
	ErrTimeout       = errors.New("pfx.timeout")
	ErrClosed        = errors.New("pfx.closed")
)

const (
	// MaxBytes bounds a credential file; a key with a chain of a few
	// certificates is a few kilobytes.
	MaxBytes         = 256 << 10
	MaxPasswordBytes = 1024
	// MaxIterations bounds one key derivation and MaxTotalIterations all
	// derivations visible before decryption. Common exports use 2,048 to
	// 600,000 iterations.
	MaxIterations      = 1_000_000
	MaxTotalIterations = 4_000_000
	maxSafes           = 2
	maxBags            = 32
	maxAttributes      = 8
	maxCertificate     = 64 << 10
	maxKeyBag          = 16 << 10
	maxSalt            = 64
	maxDERDepth        = 32
	maxDERObjects      = 4096
)

// Cost is what a file asks of a worker before any password is tried.
type Cost struct {
	// Derivations counts the key derivations visible before decryption.
	// Iterations is their work in hash-function iterations: a derivation
	// runs its iteration count once per output block of its hash.
	Derivations int
	Iterations  int64
	// macWork is the work of the MAC check, which the decoder repeats for an
	// empty password.
	macWork int64
	// Hidden reports encrypted content, which may hold further
	// derivations; the worker's deadlines bound those.
	Hidden bool
}

var (
	oidData                = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidEncryptedData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}
	oidKeyBag              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 1}
	oidShroudedKeyBag      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidCertBag             = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidX509Certificate     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}
	oidSHA1                = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256              = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA512              = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidPBMAC1              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 14}
	oidPBES2               = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACSHA1            = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACSHA256          = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACSHA512          = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}
	oidPBEWithSHA3DES      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	oidPBEWithSHA128BitRC2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 5}
	oidPBEWithSHA40BitRC2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 6}
	oidAES128CBC           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC           = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	supportedMACDigests    = []asn1.ObjectIdentifier{oidSHA1, oidSHA256, oidSHA512}
	supportedHMACs         = []asn1.ObjectIdentifier{oidHMACSHA1, oidHMACSHA256, oidHMACSHA512}
	supportedPBES1         = []asn1.ObjectIdentifier{oidPBEWithSHA3DES, oidPBEWithSHA128BitRC2, oidPBEWithSHA40BitRC2}
	supportedPBES2Ciphers  = map[string]int{oidAES128CBC.String(): 16, oidAES192CBC.String(): 24, oidAES256CBC.String(): 32}
	errTrailing            = errors.New("trailing data")
)

type pfxPDU struct {
	Version  int
	AuthSafe contentInfo
	MacData  macData `asn1:"optional"`
}

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
}

type macData struct {
	Mac        digestInfo
	MacSalt    []byte
	Iterations int `asn1:"optional,default:1"`
}

type digestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

type pbmac1Params struct {
	KDF    pkix.AlgorithmIdentifier
	MacAlg pkix.AlgorithmIdentifier
}

type pbes2Params struct {
	KDF              pkix.AlgorithmIdentifier
	EncryptionScheme pkix.AlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt       asn1.RawValue
	Iterations int
	KeyLength  int                      `asn1:"optional"`
	PRF        pkix.AlgorithmIdentifier `asn1:"optional"`
}

type pbeParams struct {
	Salt       []byte
	Iterations int
}

type encryptedData struct {
	Version              int
	EncryptedContentInfo encryptedContentInfo
}

type encryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           []byte `asn1:"tag:0,optional"`
}

type safeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue   `asn1:"tag:0,explicit"`
	Attributes []asn1.RawValue `asn1:"set,optional"`
}

type certBag struct {
	ID   asn1.ObjectIdentifier
	Data []byte `asn1:"tag:0,explicit"`
}

type encryptedPrivateKeyInfo struct {
	Algorithm     pkix.AlgorithmIdentifier
	EncryptedData []byte
}

// Preflight checks a credential file without its password. It accepts only
// the structure the worker's decoder reads: a version 3 PFX protected by a
// MAC, with at most two safes of plain or password-encrypted content, one
// private key and X.509 certificates. It reports the visible derivation
// cost, which must stay within MaxIterations and MaxTotalIterations.
func Preflight(der []byte) (Cost, error) {
	var cost Cost
	if len(der) == 0 {
		return cost, ErrMalformed
	}
	if len(der) > MaxBytes {
		return cost, ErrLimit
	}
	objects := 0
	if err := checkDER(der, 0, &objects); err != nil {
		return cost, err
	}
	var pfx pfxPDU
	if err := exact(der, &pfx); err != nil {
		return cost, ErrMalformed
	}
	if pfx.Version != 3 || !pfx.AuthSafe.ContentType.Equal(oidData) {
		return cost, ErrUnsupported
	}
	// Files without a MAC are not integrity protected.
	if len(pfx.MacData.Mac.Algorithm.Algorithm) == 0 {
		return cost, ErrUnsupported
	}
	if err := cost.mac(pfx.MacData); err != nil {
		return cost, err
	}
	var authSafe []byte
	if err := exact(pfx.AuthSafe.Content.Bytes, &authSafe); err != nil {
		return cost, ErrMalformed
	}
	if err := checkDER(authSafe, 0, &objects); err != nil {
		return cost, err
	}
	var safes []contentInfo
	if err := exact(authSafe, &safes); err != nil {
		return cost, ErrMalformed
	}
	if len(safes) == 0 || len(safes) > maxSafes {
		return cost, ErrUnsupported
	}
	keys, certs, bags := 0, 0, 0
	for _, safe := range safes {
		switch {
		case safe.ContentType.Equal(oidData):
			var data []byte
			if err := exact(safe.Content.Bytes, &data); err != nil {
				return cost, ErrMalformed
			}
			if err := checkDER(data, 0, &objects); err != nil {
				return cost, err
			}
			var contents []safeBag
			if err := exact(data, &contents); err != nil {
				return cost, ErrMalformed
			}
			for _, bag := range contents {
				if bags++; bags > maxBags || len(bag.Attributes) > maxAttributes {
					return cost, ErrLimit
				}
				k, c, err := cost.bag(bag)
				if err != nil {
					return cost, err
				}
				keys, certs = keys+k, certs+c
			}
		case safe.ContentType.Equal(oidEncryptedData):
			var enc encryptedData
			if err := exact(safe.Content.Bytes, &enc); err != nil {
				return cost, ErrMalformed
			}
			info := enc.EncryptedContentInfo
			if enc.Version != 0 || !info.ContentType.Equal(oidData) {
				return cost, ErrUnsupported
			}
			if len(info.EncryptedContent) == 0 || len(info.EncryptedContent)%8 != 0 {
				return cost, ErrMalformed
			}
			if err := cost.encryption(info.ContentEncryptionAlgorithm); err != nil {
				return cost, err
			}
			cost.Hidden = true
		default:
			return cost, ErrUnsupported
		}
	}
	if keys > 1 || !cost.Hidden && (keys != 1 || certs == 0) {
		return cost, ErrCredential
	}
	return cost, nil
}

// bag checks one visible safe bag and reports the keys and certificates in it.
func (c *Cost) bag(bag safeBag) (keys, certs int, err error) {
	switch {
	case bag.ID.Equal(oidCertBag):
		var cert certBag
		if exact(bag.Value.Bytes, &cert) != nil {
			return 0, 0, ErrMalformed
		}
		if !cert.ID.Equal(oidX509Certificate) {
			return 0, 0, ErrUnsupported
		}
		if len(cert.Data) > maxCertificate {
			return 0, 0, ErrLimit
		}
		if _, err := x509.ParseCertificate(cert.Data); err != nil {
			return 0, 0, ErrMalformed
		}
		return 0, 1, nil
	case bag.ID.Equal(oidKeyBag):
		if len(bag.Value.Bytes) > maxKeyBag {
			return 0, 0, ErrLimit
		}
		return 1, 0, nil
	case bag.ID.Equal(oidShroudedKeyBag):
		var key encryptedPrivateKeyInfo
		if exact(bag.Value.Bytes, &key) != nil {
			return 0, 0, ErrMalformed
		}
		if len(key.EncryptedData) == 0 || len(key.EncryptedData) > maxKeyBag || len(key.EncryptedData)%8 != 0 {
			return 0, 0, ErrMalformed
		}
		return 1, 0, c.encryption(key.Algorithm)
	}
	// CRL, secret and nested safe-contents bags are not read by the worker.
	return 0, 0, ErrUnsupported
}

func (c *Cost) mac(m macData) error {
	if m.Mac.Algorithm.Algorithm.Equal(oidPBMAC1) {
		var params pbmac1Params
		if exact(m.Mac.Algorithm.Parameters.FullBytes, &params) != nil {
			return ErrMalformed
		}
		if !oneOf(params.MacAlg.Algorithm, supportedHMACs) || !params.KDF.Algorithm.Equal(oidPBKDF2) {
			return ErrUnsupported
		}
		kdf, err := pbkdf2(params.KDF)
		if err != nil {
			return err
		}
		if kdf.KeyLength < 20 || kdf.KeyLength > 64 {
			return ErrUnsupported
		}
		before := c.Iterations
		err = c.derive(kdf.Iterations, blocks(kdf.KeyLength, kdf.PRF))
		c.macWork = c.Iterations - before
		return err
	}
	if !oneOf(m.Mac.Algorithm.Algorithm, supportedMACDigests) {
		return ErrUnsupported
	}
	if len(m.MacSalt) == 0 || len(m.MacSalt) > maxSalt || len(m.Mac.Digest) > 64 {
		return ErrMalformed
	}
	// The MAC key is one block of the MAC's own hash.
	err := c.derive(m.Iterations, 1)
	c.macWork = int64(max(m.Iterations, 0))
	return err
}

// EmptyPassword adds the second MAC check the decoder runs when the
// password is empty, and checks the total again.
func (c *Cost) EmptyPassword() error {
	c.Iterations += c.macWork
	if c.Iterations > MaxTotalIterations {
		return ErrLimit
	}
	return nil
}

// encryption checks a password-based encryption algorithm. PBES1 derives
// the key and the IV separately.
func (c *Cost) encryption(alg pkix.AlgorithmIdentifier) error {
	if oneOf(alg.Algorithm, supportedPBES1) {
		var params pbeParams
		if exact(alg.Parameters.FullBytes, &params) != nil {
			return ErrMalformed
		}
		if len(params.Salt) == 0 || len(params.Salt) > maxSalt {
			return ErrMalformed
		}
		// The key and the IV are derived separately from 20-byte SHA-1
		// blocks: a 3DES key takes two, an RC2 key and each IV one.
		keyBlocks := 1
		if alg.Algorithm.Equal(oidPBEWithSHA3DES) {
			keyBlocks = 2
		}
		if err := c.derive(params.Iterations, keyBlocks); err != nil {
			return err
		}
		return c.derive(params.Iterations, 1)
	}
	if !alg.Algorithm.Equal(oidPBES2) {
		return ErrUnsupported
	}
	var params pbes2Params
	if exact(alg.Parameters.FullBytes, &params) != nil {
		return ErrMalformed
	}
	size, ok := supportedPBES2Ciphers[params.EncryptionScheme.Algorithm.String()]
	if !ok || !params.KDF.Algorithm.Equal(oidPBKDF2) {
		return ErrUnsupported
	}
	var iv []byte
	if exact(params.EncryptionScheme.Parameters.FullBytes, &iv) != nil || len(iv) != 16 {
		return ErrMalformed
	}
	kdf, err := pbkdf2(params.KDF)
	if err != nil {
		return err
	}
	if kdf.KeyLength != 0 && kdf.KeyLength != size {
		return ErrUnsupported
	}
	return c.derive(kdf.Iterations, blocks(size, kdf.PRF))
}

// blocks is the number of PBKDF2 output blocks for a key: SHA-1 (the
// default PRF) gives 20 bytes per block, SHA-256 32 and SHA-512 64.
func blocks(keyLength int, prf pkix.AlgorithmIdentifier) int {
	size := 20
	switch {
	case prf.Algorithm.Equal(oidHMACSHA256):
		size = 32
	case prf.Algorithm.Equal(oidHMACSHA512):
		size = 64
	}
	return (keyLength + size - 1) / size
}

func pbkdf2(alg pkix.AlgorithmIdentifier) (pbkdf2Params, error) {
	var params pbkdf2Params
	if exact(alg.Parameters.FullBytes, &params) != nil {
		return params, ErrMalformed
	}
	if params.Salt.Class != asn1.ClassUniversal || params.Salt.Tag != asn1.TagOctetString || params.Salt.IsCompound {
		return params, ErrUnsupported
	}
	if len(params.Salt.Bytes) == 0 || len(params.Salt.Bytes) > maxSalt {
		return params, ErrMalformed
	}
	if len(params.PRF.Algorithm) != 0 && !oneOf(params.PRF.Algorithm, supportedHMACs) {
		return params, ErrUnsupported
	}
	return params, nil
}

// derive adds a derivation of the given iterations and output blocks.
func (c *Cost) derive(iterations, blocks int) error {
	if iterations < 1 {
		return ErrMalformed
	}
	if iterations > MaxIterations {
		return ErrLimit
	}
	c.Derivations++
	c.Iterations += int64(iterations) * int64(blocks)
	if c.Iterations > MaxTotalIterations {
		return ErrLimit
	}
	return nil
}

func oneOf(id asn1.ObjectIdentifier, set []asn1.ObjectIdentifier) bool {
	for _, s := range set {
		if id.Equal(s) {
			return true
		}
	}
	return false
}

// exact decodes one value and rejects trailing bytes.
func exact(der []byte, into any) error {
	rest, err := asn1.Unmarshal(der, into)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errTrailing
	}
	return nil
}

// checkDER walks definite, minimally encoded DER and bounds its depth and
// object count before any structure is decoded. Constructed values are
// walked; primitive values, including OCTET STRINGs that wrap further DER,
// are checked separately where they are decoded.
func checkDER(raw []byte, depth int, objects *int) error {
	if depth > maxDERDepth {
		return ErrLimit
	}
	for len(raw) > 0 {
		if *objects++; *objects > maxDERObjects {
			return ErrLimit
		}
		if len(raw) < 2 || raw[0]&0x1f == 0x1f {
			return ErrMalformed
		}
		constructed := raw[0]&0x20 != 0
		length, header := int(raw[1]), 2
		if raw[1]&0x80 != 0 {
			n := int(raw[1] & 0x7f)
			// Indefinite (BER) lengths and lengths beyond the file size
			// are refused.
			if n == 0 || n > 3 || len(raw) < 2+n || raw[2] == 0 {
				return ErrMalformed
			}
			length = 0
			for _, b := range raw[2 : 2+n] {
				length = length<<8 | int(b)
			}
			if length < 0x80 {
				return ErrMalformed
			}
			header = 2 + n
		}
		if length > len(raw)-header {
			return ErrMalformed
		}
		if constructed {
			if err := checkDER(raw[header:header+length], depth+1, objects); err != nil {
				return err
			}
		}
		raw = raw[header+length:]
	}
	return nil
}
