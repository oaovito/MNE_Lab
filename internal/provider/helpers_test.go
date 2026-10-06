package provider

import (
	"crypto/sha256"
	"encoding/base64"
)

func b64sha(v string) string {
	s := sha256.Sum256([]byte(v))
	return base64.RawURLEncoding.EncodeToString(s[:])
}
