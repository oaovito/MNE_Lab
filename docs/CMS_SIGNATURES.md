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

## PFX/P12 credential worker

Status: **PARTIAL backend**. `internal/signature/pfx` opens a PKCS#12
(`.pfx`/`.p12`) credential for `SignExport` without exposing the decoder to
HTTP or keeping the private key in the application process. No endpoint, UI,
Profile preference or Smart Export option uses it yet.

1. **Preflight without the password.** At most 256 KiB, definite minimal DER
   (no BER, high tags, trailing bytes), depth 32 and 4,096 objects. Version 3
   with a required MAC (SHA-1/SHA-256/SHA-512 or PBMAC1 with HMAC-SHA-1/256/512);
   at most two safes of plain or password-encrypted data; certificate,
   key and shrouded-key bags only; one key; X.509 certificates parsed. PBES1
   (3DES, RC2) and PBES2 (PBKDF2 with AES-CBC and a 16-byte IV). Each visible
   key derivation is limited to 1,000,000 iterations. Their work, counted per
   hash output block as the decoder runs it (a PBES1 3DES key and IV take three
   SHA-1 blocks, AES-256 with the default SHA-1 PRF two, and an empty
   password checks the MAC twice), is limited to 4,000,000 iterations.
   Derivations inside encrypted content cannot be seen before decryption; the
   deadlines bound them.
2. **Worker.** The application's own executable is started with the single
   argument `--pkcs12-worker-v1`, an empty environment (only `SystemRoot` on
   Windows), the temporary folder as working folder and no standard error.
   File and password travel only over its standard input; the frame that
   carried them is wiped in both processes. The decoder takes the password
   as a Go string, a copy that cannot be wiped and lasts as long as the
   worker. The worker writes no file, log or setting.
3. **Bounds.** At most two workers at once. The parent kills a worker that
   misses the 20 s load or 10 s signature deadline, breaks the protocol, or
   whose caller cancels. The worker exits by itself after 5 s without a
   handshake, 30 s of loading, 5 min idle, 15 min in total or 4,096
   signatures, and after 60 s of processor time (`RLIMIT_CPU` with its
   signal handled on Unix, the job object on Windows), which allows fewer
   signatures per session with large RSA keys. Writable mappings are limited to
   512 MiB on Linux (`RLIMIT_DATA`; a worker maps about 150 MiB, and race-detector
   test builds skip it) and process memory to 512 MiB on Windows (the job, which
   also kills the worker when the application's handle closes); elsewhere only the soft
   256 MiB Go memory limit applies. A worker whose application ended reads
   end of file or misses a deadline. Worker slots return when a worker exits,
   even if its credential is never closed. The parent creates the pipes itself
   so a worker's last frame is read even after it exits.
4. **Identity.** The worker must echo a fresh 32-byte nonce, then sign a
   challenge derived from it with the key of the certificate it reports. The
   leaf is the certificate that matches the key. Only RSA 2048–8192 and ECDSA
   P-256/P-384/P-521 are accepted. The parent sends only 32-byte SHA-256
   digests (PKCS #1 v1.5 or ECDSA; PSS and other hashes are refused) and
   verifies every signature against the certificate before returning it.
5. **Errors.** Codes only (`pfx.malformed`, `pfx.unsupported`, `pfx.limit`,
   `pfx.wrong_password`, `pfx.credential`, `pfx.worker`, `pfx.timeout`,
   `pfx.closed`); no decoder message, path or file content leaves the worker.

Tests generate every key, certificate and file in memory: Go encoders
(Modern2023, Modern2026, LegacyDES, LegacyRC2) and OpenSSL (default,
`-legacy`, SHA-512 MAC, unencrypted key, `-nomac` refused) sign through
`SignExport` and verify with `VerifySignature`. They cover wrong passwords,
malformed, truncated, trailing, BER and oversized files, cost limits, weak
keys, a certificate that does not match the key, deadlines, cancellation,
busy slots, a parent kept busy while workers exit, slots freed by a worker
that exits, work counted per block, an empty password, hanging, slow and
stalled workers, a wrong nonce, forged signatures,
a substituted certificate and another program; on Linux they read the
worker's `/proc` arguments, environment and open files. A fuzz target covers
the preflight. Dependency: `software.sslmate.com/src/go-pkcs12` v0.7.3,
BSD-3-Clause, checksum-pinned in go.sum.

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
local key access; a PFX/P12 endpoint and UI on the bounded worker above,
OS store/token adapters;
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

Development pre-releases are built by `.github/workflows/release.yml` and are
not Stable. Distribution licensing and actual clean Windows/USB testing remain
release gates in READINESS.md.
