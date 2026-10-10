package signature

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
)

func testIdentity(t *testing.T, key crypto.Signer, parent *x509.Certificate, issuer crypto.Signer) *x509.Certificate {
	t.Helper()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Invented test signer"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	if parent == nil {
		parent, issuer = template, key
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), issuer)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func testEC(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestDetachedCMSStates(t *testing.T) {
	for _, name := range []string{"RSA", "ECDSA", "empty"} {
		t.Run(name, func(t *testing.T) {
			var key crypto.Signer = testEC(t)
			if name == "RSA" {
				var err error
				key, err = rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
			}
			cert := testIdentity(t, key, nil, nil)
			content := []byte("invented,example\r\n1,2\r\n\x00\xff")
			if name == "empty" {
				content = nil
			}
			before := bytes.Clone(content)
			der, err := SignExport(content, cert, key, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(content, before) {
				t.Fatal("source modified")
			}
			parsed, err := pkcs7.Parse(der)
			if err != nil || len(parsed.Content) != 0 || !parsed.Signers[0].DigestAlgorithm.Algorithm.Equal(pkcs7.OIDDigestAlgorithmSHA256) {
				t.Fatal("not detached SHA256 CMS", err)
			}
			result, err := VerifySignature(content, der, VerifyOptions{})
			if err != nil || result.Status != "SignatureValidTrustUnknown" || result.Signature != "valid" || result.Content != "matches" || result.Trust != "unknown" || result.CertificateTime != "valid" || result.Revocation != "not_checked" || result.TrustedTimestamp != "not_verified" || result.ClaimedSigningTime == nil {
				t.Fatalf("unknown trust: %+v %v", result, err)
			}
			pool := x509.NewCertPool()
			pool.AddCert(cert)
			result, err = VerifySignature(content, der, VerifyOptions{Roots: pool})
			if err != nil || result.Status != "SignatureValid" || result.Trust != "verified" {
				t.Fatalf("explicit trust: %+v %v", result, err)
			}
			result, err = VerifySignature(content, der, VerifyOptions{Roots: x509.NewCertPool()})
			if err != nil || result.Trust != "untrusted" || result.Signature != "valid" {
				t.Fatalf("empty trust pool: %+v %v", result, err)
			}
			result, err = VerifySignature(content, der, VerifyOptions{Roots: pool, CurrentTime: cert.NotAfter.Add(time.Second)})
			if err != nil || result.Status != "CertificateExpired" || result.Signature != "valid" || result.Content != "matches" || result.Trust == "verified" {
				t.Fatalf("expiry must not use claimed signing time: %+v %v", result, err)
			}
			result, err = VerifySignature(content, der, VerifyOptions{CurrentTime: cert.NotBefore.Add(-time.Second)})
			if err != nil || result.Status != "CertificateNotYetValid" || result.Signature != "valid" {
				t.Fatalf("not yet valid: %+v %v", result, err)
			}
			result, err = VerifySignature(append(bytes.Clone(content), '\n'), der, VerifyOptions{})
			if err != nil || result.Status != "FileModified" || result.Signature != "valid" || result.Content != "modified" {
				t.Fatalf("modified bytes: %+v %v", result, err)
			}
			broken := bytes.Clone(der)
			at := bytes.Index(broken, parsed.Signers[0].EncryptedDigest)
			if at < 0 {
				t.Fatal("signature not found")
			}
			broken[at+len(parsed.Signers[0].EncryptedDigest)-1] ^= 1
			result, err = VerifySignature(append(bytes.Clone(content), '\n'), broken, VerifyOptions{})
			if err != nil || result.Status != "InvalidSignature" || result.Signature != "invalid" || result.Content != "unknown" {
				t.Fatalf("tampered signature cannot imply FileModified: %+v %v", result, err)
			}
		})
	}
}

func TestCMSChainAndCredentialRejection(t *testing.T) {
	rootKey, leafKey := testEC(t), testEC(t)
	root := testIdentity(t, rootKey, nil, nil)
	leaf := testIdentity(t, leafKey, root, rootKey)
	der, err := SignExport([]byte("invented"), leaf, leafKey, []*x509.Certificate{root})
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	result, err := VerifySignature([]byte("invented"), der, VerifyOptions{Roots: pool})
	if err != nil || result.Trust != "verified" {
		t.Fatalf("chain: %+v %v", result, err)
	}
	for _, item := range []struct {
		name  string
		cert  *x509.Certificate
		key   crypto.Signer
		chain []*x509.Certificate
	}{
		{"nil certificate", nil, leafKey, nil},
		{"nil key", leaf, nil, nil},
		{"typed nil key", leaf, (*ecdsa.PrivateKey)(nil), nil},
		{"wrong key", leaf, rootKey, nil},
		{"nil parent", leaf, leafKey, []*x509.Certificate{nil}},
		{"wrong parent", leaf, leafKey, []*x509.Certificate{testIdentity(t, testEC(t), nil, nil)}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := SignExport(nil, item.cert, item.key, item.chain); !errors.Is(err, ErrCredential) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestCMSStructuralBounds(t *testing.T) {
	key := testEC(t)
	cert := testIdentity(t, key, nil, nil)
	der, err := SignExport([]byte("invented"), cert, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	deep := []byte{5, 0}
	for range maxDERDepth + 2 {
		deep, err = asn1.Marshal(asn1.RawValue{Class: 0, Tag: asn1.TagSequence, IsCompound: true, Bytes: deep})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		name string
		raw  []byte
		want error
	}{
		{"empty", nil, ErrInvalid},
		{"trailing", append(bytes.Clone(der), 5, 0), ErrInvalid},
		{"indefinite BER", []byte{0x30, 0x80, 5, 0, 0, 0}, ErrInvalid},
		{"truncated", der[:len(der)-1], ErrInvalid},
		{"length overflow", []byte{0x30, 0x84, 0x7f, 0xff, 0xff, 0xff}, ErrInvalid},
		{"deep", deep, ErrLimit},
		{"oversize", make([]byte, MaxCMSBytes+1), ErrLimit},
		{"too many objects", append([]byte{0x30, 0x82, 0x80, 0x02}, bytes.Repeat([]byte{5, 0}, 16385)...), ErrLimit},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := VerifySignature(nil, item.raw, VerifyOptions{}); !errors.Is(err, item.want) {
				t.Fatalf("got %v, want %v", err, item.want)
			}
		})
	}
	for _, item := range []struct {
		name   string
		change func(*pkcs7.SignedData) error
	}{
		{"attached", func(sd *pkcs7.SignedData) error { return sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}) }},
		{"multiple signers", func(sd *pkcs7.SignedData) error {
			sd.Detach()
			if err := sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{}); err != nil {
				return err
			}
			return sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{})
		}},
		{"SHA1", func(sd *pkcs7.SignedData) error {
			sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA1)
			sd.Detach()
			return sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{})
		}},
		{"no attributes", func(sd *pkcs7.SignedData) error {
			sd.Detach()
			return sd.SignWithoutAttr(cert, key, pkcs7.SignerInfoConfig{})
		}},
		{"duplicate digest", func(sd *pkcs7.SignedData) error {
			sd.Detach()
			return sd.AddSigner(cert, key, pkcs7.SignerInfoConfig{ExtraSignedAttributes: []pkcs7.Attribute{{Type: pkcs7.OIDAttributeMessageDigest, Value: make([]byte, 32)}}})
		}},
	} {
		t.Run(item.name, func(t *testing.T) {
			sd, _ := pkcs7.NewSignedData([]byte("invented"))
			sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
			if err := item.change(sd); err != nil {
				t.Fatal(err)
			}
			raw, err := sd.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if result, err := VerifySignature([]byte("invented"), raw, VerifyOptions{}); err == nil {
				t.Fatalf("accepted unsupported/ambiguous CMS: %+v", result)
			}
		})
	}
}

// OpenSSL is an independent development oracle, never a product dependency.
// Its key is generated here, stored only in t.TempDir with mode 0600, and
// deleted by the test. No committed fixture or output contains private keys.
func TestOpenSSLDetachedCMSInteroperability(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("independent OpenSSL oracle unavailable")
	}
	for _, name := range []string{"RSA", "ECDSA"} {
		t.Run(name, func(t *testing.T) {
			var key crypto.Signer = testEC(t)
			if name == "RSA" {
				key, err = rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
			}
			cert := testIdentity(t, key, nil, nil)
			content := []byte("invented,data\r\n0,1.2300\r\n\x00\xff")
			der, err := SignExport(content, cert, key, nil)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			write := func(name string, raw []byte) string {
				p := filepath.Join(dir, name)
				if err := os.WriteFile(p, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			}
			input := write("content.bin", content)
			sig := write("mnelab.p7s", der)
			certPath := write("certificate.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
			private, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			keyPath := write("ephemeral-key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(openssl, args...)
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("OpenSSL %v: %v (%s)", args, err, raw)
				}
			}
			verified := filepath.Join(dir, "verified.bin")
			run("cms", "-verify", "-binary", "-inform", "DER", "-in", sig, "-content", input, "-CAfile", certPath, "-out", verified)
			raw, err := os.ReadFile(verified)
			if err != nil || !bytes.Equal(raw, content) {
				t.Fatal("OpenSSL byte mismatch", err)
			}
			for _, digest := range []string{"sha256", "sha384", "sha512"} {
				t.Run(digest, func(t *testing.T) {
					foreign := filepath.Join(dir, "openssl-"+digest+".p7s")
					run("cms", "-sign", "-binary", "-md", digest, "-in", input, "-signer", certPath, "-inkey", keyPath, "-outform", "DER", "-out", foreign)
					raw, err := os.ReadFile(foreign)
					if err != nil {
						t.Fatal(err)
					}
					result, err := VerifySignature(content, raw, VerifyOptions{})
					if err != nil || result.Signature != "valid" || result.Content != "matches" {
						t.Fatalf("OpenSSL CMS: %+v %v", result, err)
					}
				})
			}
		})
	}
}

func FuzzVerifySignature(f *testing.F) {
	if raw, err := os.ReadFile("../../testdata/signature/invented.p7s"); err == nil {
		f.Add([]byte("invented,example\r\n0,1.2300\r\n"), raw)
	}
	f.Add([]byte("invented"), []byte{0x30, 0x80, 0, 0})
	f.Add([]byte{}, []byte{0x30, 0})
	f.Fuzz(func(t *testing.T, content, der []byte) {
		if len(content) > 1<<20 || len(der) > 1<<20 {
			t.Skip()
		}
		_, _ = VerifySignature(content, der, VerifyOptions{CurrentTime: time.Unix(0, 0)})
	})
}

func TestIndependentCMSFixture(t *testing.T) {
	content, err := os.ReadFile("../../testdata/signature/invented.csv")
	if err != nil {
		t.Fatal(err)
	}
	der, err := os.ReadFile("../../testdata/signature/invented.p7s")
	if err != nil {
		t.Fatal(err)
	}
	result, err := VerifySignature(content, der, VerifyOptions{CurrentTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil || result.Signature != "valid" || result.Content != "matches" || result.Trust != "unknown" || result.TrustedTimestamp != "not_verified" {
		t.Fatalf("independent fixture: %+v %v", result, err)
	}
	manifest, err := os.ReadFile("../../testdata/signature/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Files map[string]struct {
			Size   int
			SHA256 string
		}
	}
	if err := json.Unmarshal(manifest, &index); err != nil {
		t.Fatal(err)
	}
	for name, item := range index.Files {
		raw, err := os.ReadFile(filepath.Join("../../testdata/signature", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if len(raw) != item.Size || hex.EncodeToString(sum[:]) != item.SHA256 {
			t.Fatalf("fixture mismatch: %s", name)
		}
	}
}
