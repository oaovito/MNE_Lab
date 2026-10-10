package app

import (
	"io"
	"net/http"

	"github.com/oaovito/mne_lab/internal/signature"
)

// verifySignatureUpload is a read-only, session-authenticated operation. It
// reads exactly two bounded multipart files in memory. Names are never used as
// paths; no upload, certificate, original or result enters File Library, stores,
// logs, backups, recovery or sync. No multipart temporary files are created.
func verifySignatureUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	r.Body = http.MaxBytesReader(w, r.Body, signature.MaxContentBytes+signature.MaxCMSBytes+(1<<20))
	defer r.Body.Close()
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, signature.ErrInvalid
	}
	var content, cms []byte
	seen := map[string]bool{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, signature.ErrInvalid
		}
		name := part.FormName()
		if seen[name] || part.FileName() == "" || name != "content" && name != "signature" {
			return nil, signature.ErrInvalid
		}
		seen[name] = true
		limit := signature.MaxContentBytes
		if name == "signature" {
			limit = signature.MaxCMSBytes
		}
		raw, err := io.ReadAll(io.LimitReader(part, int64(limit)+1))
		if len(raw) > limit {
			return nil, signature.ErrLimit
		}
		if err != nil {
			return nil, signature.ErrInvalid
		}
		if name == "content" {
			content = raw
		} else {
			cms = raw
		}
	}
	if !seen["content"] || !seen["signature"] {
		return nil, signature.ErrInvalid
	}
	return signature.VerifySignature(content, cms, signature.VerifyOptions{})
}
