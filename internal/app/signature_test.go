package app

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/signature"
)

func TestSignatureVerificationUpload(t *testing.T) {
	a, c := portableApp(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "Invented local test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("invented,values\r\n1,2\r\n")
	cms, err := signature.SignExport(content, cert, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	type part struct {
		name, filename string
		raw            []byte
	}
	upload := func(parts []part, headers map[string]string) (int, []byte) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for _, p := range parts {
			var err error
			if p.filename == "" {
				field, e := writer.CreateFormField(p.name)
				err = e
				if err == nil {
					_, err = field.Write(p.raw)
				}
			} else {
				field, e := writer.CreateFormFile(p.name, p.filename)
				err = e
				if err == nil {
					_, err = field.Write(p.raw)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if headers == nil {
			headers = map[string]string{}
		}
		headers["Content-Type"] = writer.FormDataContentType()
		return c.do("POST", "/api/signatures/verify", body.Bytes(), headers)
	}
	valid := []part{{"content", "../../original.csv", content}, {"signature", "../detached.p7s", cms}}
	if status, _ := upload(valid, nil); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", status)
	}
	response, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	before, err := os.ReadDir(a.L.Temp)
	if err != nil {
		t.Fatal(err)
	}
	if a.State().Account != nil || a.State().Profile != nil {
		t.Fatal("expected no account/profile")
	}
	status, raw := upload(valid, nil)
	var result signature.Verification
	if status != 200 || json.Unmarshal(raw, &result) != nil || result.Status != "SignatureValidTrustUnknown" || result.Content != "matches" {
		t.Fatalf("verify: %d %s", status, raw)
	}
	if a.State().Account != nil || a.State().Profile != nil {
		t.Fatal("verification created or selected user storage")
	}
	after, err := os.ReadDir(a.L.Temp)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("verification created temporary files", err)
	}
	if !bytes.Equal(valid[0].raw, content) || !bytes.Equal(valid[1].raw, cms) {
		t.Fatal("inputs modified")
	}
	for _, tc := range []struct {
		name    string
		parts   []part
		headers map[string]string
		status  int
	}{
		{"changed file", []part{{"content", "changed.csv", []byte("changed")}, {"signature", "sig.p7s", cms}}, nil, 200},
		{"missing signature", valid[:1], nil, 400},
		{"duplicate content", append(append([]part{}, valid...), valid[0]), nil, 400},
		{"unknown field", append(append([]part{}, valid...), part{"privateKey", "key", []byte("never persisted")}), nil, 400},
		{"field instead of file", []part{{"content", "", content}, valid[1]}, nil, 400},
		{"oversized CMS", []part{valid[0], {"signature", "sig.p7s", make([]byte, signature.MaxCMSBytes+1)}}, nil, 400},
		{"malformed CMS", []part{valid[0], {"signature", "sig.p7s", []byte("bad")}}, nil, 400},
		{"foreign origin", valid, map[string]string{"Origin": "https://example.invalid"}, 403},
		{"missing CSRF marker", valid, map[string]string{"X-MNE-Lab": ""}, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, raw := upload(tc.parts, tc.headers)
			if status != tc.status {
				t.Fatalf("%d %s", status, raw)
			}
			if tc.name == "changed file" {
				var result signature.Verification
				if json.Unmarshal(raw, &result) != nil || result.Status != "FileModified" || result.Signature != "valid" {
					t.Fatalf("modified result: %s", raw)
				}
			}
		})
	}
	if status, _ := c.do("POST", "/api/signatures/verify", []byte("{}"), map[string]string{"Content-Type": "application/json"}); status != 400 {
		t.Fatalf("wrong content type: %d", status)
	}
}
