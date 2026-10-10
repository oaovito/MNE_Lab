# Detached export signatures and verification

Status: **PARTIAL**. `internal/signature.SignExport` is a native backend for
caller-owned `crypto.Signer` credentials. It signs finalized bytes as detached
CMS SignedData DER (`.p7s`) with SHA-256 and RSA 2048–8192 or ECDSA
P-256/P-384/P-521. Public certificates accompany the signature. It does not
modify the artifact, serialize/store credentials or invoke external programs.
It verifies the provider's returned signature before reporting success.

The global **Verify Signature** panel is available from the profile menu and
the account start screen. Choose the exact artifact and its `.p7s`. The
session-authenticated loopback endpoint accepts two bounded multipart files
in memory; filenames are not paths and uploads are not imported, persisted,
logged, synchronized, backed up or placed in recovery. Closing drops browser
file handles; changing either file clears the previous result and cancels a
pending request. Read-only verification needs no unlocked account/profile.

## Supported verification contract

One detached DER CMS signer with issuer/serial identification, signed
contentType=`data` and messageDigest attributes. RSA PKCS#1 v1.5 or ECDSA with
SHA-256/SHA-384/SHA-512 are supported. SHA-1/MD5, RSA-PSS, EdDSA, multiple
signers, attached/BER content, embedded CRLs and embedded PDF signatures are
explicitly unsupported. This bounded initial contract does not imply those
standards are inherently invalid. Content is limited to 64 MiB, CMS to 4 MiB,
32 public certificates of at most 64 KiB each, DER depth 32 and 16,384 objects.
DER framing, canonical signed-attribute encoding, duplicate attributes,
content type and digest algorithm agreement are checked before acceptance.

The certificate's own issuance-signature algorithm does not select the CMS
digest. Signing explicitly overrides the dependency's default SHA-1.
Mathematical verification uses Go's standard `x509.Certificate.CheckSignature`.
Standard ASN.1 signed SET encoding is authenticated before comparing the
messageDigest against artifact bytes. A corrupted signature cannot be labeled
merely "file modified" because its digest is not authenticated.

## Distinct evidence

| Field | Meaning |
|---|---|
| Signature | Mathematical signature is valid, invalid or unknown |
| Content | Exact bytes match the authenticated digest, differ, or cannot be assessed |
| Certificate time | Valid, expired or not yet valid at verification time |
| Trust | Unknown without explicit roots; verified/untrusted against supplied roots in the backend |
| Claimed signingTime | A signer-declared attribute, never an independent trusted timestamp |
| Revocation / trusted timestamp | Not checked / not verified in this implementation |

The UI supplies no trust roots and therefore reports trust unknown. A
self-signed certificate is never promoted to an independently trusted identity
just because it accompanies CMS. The backend verifies a chain only against
an explicit caller-supplied pool, at the caller's verification clock (UTC now
by default), with separate certificate-time reporting. It never uses claimed
signingTime to make an expired certificate currently valid. No revocation,
RFC 3161, PAdES or scientific-validation result is invented.

## Independent tests and remaining integration

An independently generated OpenSSL RSA/SHA-256 fixture contains only invented
CSV, public certificate and detached signature. Its ephemeral private key was
deleted; fixed public hashes are in `testdata/signature/manifest.json`.
Bidirectional live OpenSSL comparisons cover RSA/ECDSA and incoming
SHA-256/SHA-384/SHA-512, including exact CRLF/binary content. OpenSSL is a
development oracle, never a user/runtime dependency. Native tests exercise
modified bytes, invalid signatures, explicit/absent trust, expiry, wrong
credentials, ambiguous structures and limits. Browser tests cover three
languages, light/dark themes and normal/small screens without library writes.

Still required: Profile-scoped signing identity/preferences and legitimate
local key access; safe PFX/P12 import and cleanup, OS store/token adapters;
Smart Export None/Visible/Digital/Complete and final-bytes ordering;
interoperable PDF/PAdES; trust-root selection, revocation and trusted timestamps;
signed research-package integration. None of these is established by the
backend or global detached verification panel alone. The release/update
Ed25519 identity is separate and is never a research signing credential.

Dependency: `github.com/digitorus/pkcs7`
`v0.0.0-20250730155240-ffadbf3f398c`, MIT, checksum-pinned in go.sum. This
compatible revision supports the current Go 1.26.9 toolchain; the newer module
requires Go 1.27. Limits/preflight precede its BER transcoder and trailing-input
normalization. Selection does not claim a blanket security certification.

Windows owner-test delivery is a separate draft-only workflow. It deliberately
builds the earlier published `8a12ff6` checkpoint, which predates this feature;
it never publishes Stable or a public binary release. Runtime corresponding
source, distribution licensing and actual clean Windows/USB testing remain
release gates in READINESS.md.
