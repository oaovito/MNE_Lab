// Package signature implements bounded, detached CMS signatures for finalized
// export bytes. Credentials are caller-owned, ephemeral crypto.Signer handles;
// this package has no credential storage, networking, or subprocesses.
package signature

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"reflect"
	"time"

	"github.com/digitorus/pkcs7"
)

var (
	ErrInvalid     = errors.New("signature.invalid")
	ErrUnsupported = errors.New("signature.unsupported")
	ErrLimit       = errors.New("signature.limit")
	ErrCredential  = errors.New("signature.credential")
)

const (
	MaxContentBytes     = 64 << 20
	MaxCMSBytes         = 4 << 20
	maxCertificates     = 32
	maxCertificateBytes = 64 << 10
	maxDERDepth         = 32
	maxDERObjects       = 16384
)

// VerifyOptions uses only explicitly supplied trust roots. A nil pool means
// trust unknown, never implicitly trusting a certificate carried in the CMS.
// CurrentTime defaults to UTC now; claimed signingTime is never a trust clock.
type VerifyOptions struct {
	Roots       *x509.CertPool
	CurrentTime time.Time
}

// Verification keeps mathematical validity, byte correspondence, certificate
// time and chain trust separate. Revocation and trusted timestamps are not
// implemented. No state here establishes scientific validation.
type Verification struct {
	Status             string     `json:"status"`
	Signature          string     `json:"signature"`
	Content            string     `json:"content"`
	Trust              string     `json:"trust"`
	CertificateTime    string     `json:"certificateTime"`
	CertificateSHA256  string     `json:"certificateSHA256,omitempty"`
	Subject            string     `json:"subject,omitempty"`
	ClaimedSigningTime *time.Time `json:"claimedSigningTime,omitempty"`
	Revocation         string     `json:"revocation"`
	TrustedTimestamp   string     `json:"trustedTimestamp"`
}

// SignExport signs the exact finalized bytes with SHA-256 using RSA (2048 to
// 8192 bits) or ECDSA (P-256/P-384/P-521). It returns detached CMS DER (.p7s),
// not embedded PDF/PAdES, a visible mark, or an official release signature.
// The caller retains ownership of its key; only public certificates enter CMS.
func SignExport(content []byte, certificate *x509.Certificate, signer crypto.Signer, chain []*x509.Certificate) ([]byte, error) {
	if len(content) > MaxContentBytes || len(chain) >= maxCertificates {
		return nil, ErrLimit
	}
	if certificate == nil || signer == nil || isNilSigner(signer) {
		return nil, ErrCredential
	}
	if len(certificate.Raw) > maxCertificateBytes {
		return nil, ErrLimit
	}
	cert, err := x509.ParseCertificate(certificate.Raw)
	if err != nil || !supportedPublicKey(cert.PublicKey) || cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, ErrCredential
	}
	key := signer.Public()
	if !supportedPublicKey(key) {
		return nil, ErrCredential
	}
	public, err := x509.MarshalPKIXPublicKey(key)
	if err != nil || !bytes.Equal(public, cert.RawSubjectPublicKeyInfo) {
		return nil, ErrCredential
	}
	now := time.Now().UTC()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, ErrCredential
	}
	parents := make([]*x509.Certificate, len(chain))
	total := len(cert.Raw)
	for i, parent := range chain {
		if parent == nil {
			return nil, ErrCredential
		}
		if len(parent.Raw) > maxCertificateBytes {
			return nil, ErrLimit
		}
		total += len(parent.Raw)
		if total > MaxCMSBytes/2 {
			return nil, ErrLimit
		}
		parents[i], err = x509.ParseCertificate(parent.Raw)
		if err != nil {
			return nil, ErrCredential
		}
	}
	if total > MaxCMSBytes/2 {
		return nil, ErrLimit
	}
	sd, err := pkcs7.NewSignedData(content)
	if err != nil {
		return nil, ErrInvalid
	}
	sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256) // upstream defaults to SHA-1
	if err := sd.AddSignerChain(cert, signer, parents, pkcs7.SignerInfoConfig{}); err != nil {
		return nil, ErrCredential
	}
	sd.Detach()
	der, err := sd.Finish()
	if err != nil {
		return nil, ErrInvalid
	}
	// A provider returning the wrong signature must not produce a success.
	result, err := VerifySignature(content, der, VerifyOptions{CurrentTime: now})
	if err != nil || result.Signature != "valid" || result.Content != "matches" {
		return nil, ErrCredential
	}
	return der, nil
}

func isNilSigner(s crypto.Signer) bool {
	v := reflect.ValueOf(s)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func supportedPublicKey(key any) bool {
	switch k := key.(type) {
	case *rsa.PublicKey:
		return k != nil && k.N != nil && k.N.BitLen() >= 2048 && k.N.BitLen() <= 8192
	case *ecdsa.PublicKey:
		if k == nil || k.Curve == nil || k.X == nil || k.Y == nil {
			return false
		}
		bits := k.Curve.Params().BitSize
		return (bits == 256 || bits == 384 || bits == 521) && k.Curve.IsOnCurve(k.X, k.Y)
	}
	return false
}

// VerifySignature verifies one detached DER CMS signer without modifying input.
// Invalid cryptographic signatures and changed content are report states; unsafe,
// malformed, oversized or unsupported structures return a bounded error.
// Digest mismatch is reported as FileModified only after authenticating the
// signed attributes, so attacker-controlled digest bytes cannot imply validity.
func VerifySignature(content, der []byte, opt VerifyOptions) (Verification, error) {
	r := Verification{Status: "UnableToVerify", Signature: "unknown", Content: "unknown", Trust: "unknown", CertificateTime: "unknown", Revocation: "not_checked", TrustedTimestamp: "not_verified"}
	if len(content) > MaxContentBytes || len(der) > MaxCMSBytes {
		return r, ErrLimit
	}
	wire, err := inspectCMS(der)
	if err != nil {
		return r, err
	}
	p7, err := pkcs7.Parse(der)
	if err != nil || len(p7.Signers) != 1 || len(p7.Certificates) == 0 || len(p7.Certificates) > maxCertificates {
		return r, ErrInvalid
	}
	s := p7.Signers[0]
	if s.Version != 1 || s.IssuerAndSerialNumber.SerialNumber == nil {
		return r, ErrUnsupported
	}
	var cert *x509.Certificate
	for _, candidate := range p7.Certificates {
		if !supportedPublicKey(candidate.PublicKey) {
			return r, ErrUnsupported
		}
		if bytes.Equal(candidate.RawIssuer, s.IssuerAndSerialNumber.IssuerName.FullBytes) && candidate.SerialNumber.Cmp(s.IssuerAndSerialNumber.SerialNumber) == 0 {
			if cert != nil { // ambiguous issuer/serial, including duplicate certs
				return r, ErrInvalid
			}
			cert = candidate
		}
	}
	if cert == nil || !supportedPublicKey(cert.PublicKey) {
		return r, ErrUnsupported
	}
	hash, algorithm, ok := supportedAlgorithm(s.DigestAlgorithm.Algorithm, s.DigestEncryptionAlgorithm.Algorithm, cert.PublicKey)
	if !ok {
		return r, ErrUnsupported
	}
	if len(s.AuthenticatedAttributes) == 0 {
		return r, ErrUnsupported // this contract requires contentType + messageDigest
	}
	attrs, err := asn1.MarshalWithParams(s.AuthenticatedAttributes, "set")
	if err != nil || len(attrs) == 0 || !bytes.Equal(attrs[1:], wire.SignedAttributes.FullBytes[1:]) {
		return r, ErrInvalid // original implicit SET must be canonical and complete
	}
	seen := map[string]bool{}
	var digest []byte
	for _, attr := range s.AuthenticatedAttributes {
		key := attr.Type.String()
		if seen[key] || attr.Value.Class != 0 || attr.Value.Tag != asn1.TagSet || !attr.Value.IsCompound {
			return r, ErrInvalid
		}
		seen[key] = true
		switch {
		case attr.Type.Equal(pkcs7.OIDAttributeMessageDigest):
			if !decodeExact(attr.Value.Bytes, &digest) || len(digest) != hash.Size() {
				return r, ErrInvalid
			}
		case attr.Type.Equal(pkcs7.OIDAttributeContentType):
			var oid asn1.ObjectIdentifier
			if !decodeExact(attr.Value.Bytes, &oid) || !oid.Equal(pkcs7.OIDData) {
				return r, ErrInvalid
			}
		case attr.Type.Equal(pkcs7.OIDAttributeSigningTime):
			var claimed time.Time
			if !decodeExact(attr.Value.Bytes, &claimed) {
				return r, ErrInvalid
			}
			r.ClaimedSigningTime = &claimed
		}
	}
	if digest == nil || !seen[pkcs7.OIDAttributeContentType.String()] {
		return r, ErrInvalid
	}
	fingerprint := sha256.Sum256(cert.Raw)
	r.CertificateSHA256 = hex.EncodeToString(fingerprint[:])
	r.Subject = cert.Subject.String()
	if cert.CheckSignature(algorithm, attrs, s.EncryptedDigest) != nil {
		r.Signature, r.Status = "invalid", "InvalidSignature"
		return r, nil
	}
	r.Signature = "valid"
	h := hash.New()
	_, _ = h.Write(content)
	if subtle.ConstantTimeCompare(digest, h.Sum(nil)) != 1 {
		r.Content, r.Status = "modified", "FileModified"
		return r, nil
	}
	r.Content = "matches"
	now := opt.CurrentTime
	if now.IsZero() {
		now = time.Now().UTC()
	}
	r.CertificateTime = "valid"
	if now.Before(cert.NotBefore) {
		r.CertificateTime = "not_yet_valid"
	} else if now.After(cert.NotAfter) {
		r.CertificateTime = "expired"
	}
	if opt.Roots != nil {
		intermediates := x509.NewCertPool()
		for _, c := range p7.Certificates {
			if !bytes.Equal(c.Raw, cert.Raw) {
				intermediates.AddCert(c)
			}
		}
		_, chainErr := cert.Verify(x509.VerifyOptions{Roots: opt.Roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
		r.Trust = "untrusted"
		if chainErr == nil && (cert.KeyUsage == 0 || cert.KeyUsage&x509.KeyUsageDigitalSignature != 0) {
			r.Trust = "verified"
		}
	}
	r.Status = "SignatureValidTrustUnknown"
	if r.Trust == "verified" {
		r.Status = "SignatureValid"
	}
	if r.CertificateTime == "expired" {
		r.Status = "CertificateExpired"
	} else if r.CertificateTime == "not_yet_valid" {
		r.Status = "CertificateNotYetValid"
	}
	return r, nil
}

func supportedAlgorithm(digest, signature asn1.ObjectIdentifier, key any) (crypto.Hash, x509.SignatureAlgorithm, bool) {
	var h crypto.Hash
	var rsaOID, ecOID asn1.ObjectIdentifier
	var ra, ea x509.SignatureAlgorithm
	switch {
	case digest.Equal(pkcs7.OIDDigestAlgorithmSHA256):
		h, rsaOID, ecOID, ra, ea = crypto.SHA256, pkcs7.OIDEncryptionAlgorithmRSASHA256, pkcs7.OIDDigestAlgorithmECDSASHA256, x509.SHA256WithRSA, x509.ECDSAWithSHA256
	case digest.Equal(pkcs7.OIDDigestAlgorithmSHA384):
		h, rsaOID, ecOID, ra, ea = crypto.SHA384, pkcs7.OIDEncryptionAlgorithmRSASHA384, pkcs7.OIDDigestAlgorithmECDSASHA384, x509.SHA384WithRSA, x509.ECDSAWithSHA384
	case digest.Equal(pkcs7.OIDDigestAlgorithmSHA512):
		h, rsaOID, ecOID, ra, ea = crypto.SHA512, pkcs7.OIDEncryptionAlgorithmRSASHA512, pkcs7.OIDDigestAlgorithmECDSASHA512, x509.SHA512WithRSA, x509.ECDSAWithSHA512
	default:
		return 0, 0, false
	}
	switch key.(type) {
	case *rsa.PublicKey:
		return h, ra, signature.Equal(pkcs7.OIDEncryptionAlgorithmRSA) || signature.Equal(rsaOID)
	case *ecdsa.PublicKey:
		return h, ea, signature.Equal(ecOID)
	}
	return 0, 0, false
}

func decodeExact(raw []byte, into any) bool {
	rest, err := asn1.Unmarshal(raw, into)
	return err == nil && len(rest) == 0
}

type cmsContent struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue `asn1:"optional,explicit,tag:0"`
}

type cmsSigned struct {
	Version      int
	Digests      asn1.RawValue
	Content      cmsContent
	Certificates asn1.RawValue `asn1:"optional,tag:0"`
	CRLs         asn1.RawValue `asn1:"optional,tag:1"`
	Signers      asn1.RawValue
}

type cmsSigner struct {
	Version          int
	Identifier       asn1.RawValue
	Digest           pkix.AlgorithmIdentifier
	SignedAttributes asn1.RawValue `asn1:"optional,tag:0"`
	Algorithm        pkix.AlgorithmIdentifier
	Signature        []byte
	Unsigned         asn1.RawValue `asn1:"optional,tag:1"`
}

// Preflight precedes the upstream BER transcoder: forbid indefinite lengths,
// trailing bytes, deep trees, excessive objects and attached/non-data content.
func inspectCMS(der []byte) (cmsSigner, error) {
	var signer cmsSigner
	count := 0
	if len(der) == 0 {
		return signer, ErrInvalid
	}
	if err := inspectDER(der, 0, &count, true); err != nil {
		return signer, err
	}
	var envelope cmsContent
	var signed cmsSigned
	if !decodeExact(der, &envelope) || !envelope.Type.Equal(pkcs7.OIDSignedData) || !decodeExact(envelope.Content.Bytes, &signed) {
		return signer, ErrInvalid
	}
	if signed.Version != 1 || !signed.Content.Type.Equal(pkcs7.OIDData) || len(signed.Content.Content.FullBytes) != 0 || len(signed.CRLs.FullBytes) != 0 {
		return signer, ErrUnsupported
	}
	if signed.Digests.Class != 0 || signed.Digests.Tag != asn1.TagSet || signed.Signers.Class != 0 || signed.Signers.Tag != asn1.TagSet || !decodeExact(signed.Signers.Bytes, &signer) {
		return signer, ErrUnsupported // exactly one signer, issuer + serial only
	}
	if len(signer.SignedAttributes.FullBytes) < 2 || len(signer.SignedAttributes.Bytes) == 0 {
		return signer, ErrUnsupported
	}
	if signer.SignedAttributes.Class != 2 || signer.SignedAttributes.Tag != 0 || !signer.SignedAttributes.IsCompound || signer.Identifier.Class != 0 || signer.Identifier.Tag != asn1.TagSequence || !signer.Identifier.IsCompound {
		return signer, ErrUnsupported
	}
	if !nullOrAbsent(signer.Digest.Parameters) || !nullOrAbsent(signer.Algorithm.Parameters) {
		return signer, ErrUnsupported
	}
	var digests []pkix.AlgorithmIdentifier
	rest, err := asn1.UnmarshalWithParams(signed.Digests.FullBytes, &digests, "set")
	if err != nil || len(rest) != 0 || len(digests) != 1 || !digests[0].Algorithm.Equal(signer.Digest.Algorithm) {
		return signer, ErrInvalid
	}
	if !nullOrAbsent(digests[0].Parameters) {
		return signer, ErrUnsupported
	}
	certificates := 0
	for raw := signed.Certificates.Bytes; len(raw) > 0; {
		var certificate asn1.RawValue
		raw, err = asn1.Unmarshal(raw, &certificate)
		if err != nil {
			return signer, ErrInvalid
		}
		certificates++
		if len(certificate.FullBytes) > maxCertificateBytes {
			return signer, ErrLimit
		}
		if certificates > maxCertificates {
			return signer, ErrLimit
		}
	}
	return signer, nil
}

func nullOrAbsent(value asn1.RawValue) bool {
	return len(value.FullBytes) == 0 || bytes.Equal(value.FullBytes, []byte{5, 0})
}

func inspectDER(raw []byte, depth int, count *int, single bool) error {
	if depth > maxDERDepth {
		return ErrLimit
	}
	var previous []byte
	objects := 0
	for len(raw) > 0 {
		(*count)++
		objects++
		if *count > maxDERObjects {
			return ErrLimit
		}
		var value asn1.RawValue
		rest, err := asn1.Unmarshal(raw, &value)
		if err != nil || len(rest) >= len(raw) || value.Class == 0 && value.Tag == 0 {
			return ErrInvalid
		}
		if value.IsCompound {
			if err := inspectDER(value.Bytes, depth+1, count, false); err != nil {
				return err
			}
			if value.Class == 0 && value.Tag == asn1.TagSet {
				for children := value.Bytes; len(children) > 0; {
					var child asn1.RawValue
					children, _ = asn1.Unmarshal(children, &child)
					if previous != nil && bytes.Compare(previous, child.FullBytes) > 0 {
						return ErrInvalid
					}
					previous = child.FullBytes
				}
				previous = nil
			}
		}
		raw = rest
	}
	if single && objects != 1 {
		return ErrInvalid
	}
	return nil
}
