# Invented detached CMS interoperability fixture

These public bytes contain only an invented CSV, a public self-signed certificate
and a detached RSA/SHA-256 CMS signature. They are not instrument data or a
trusted identity, release signature, timestamp or scientific reference.

Generated independently by OpenSSL 3.5.7 with `req -x509 -newkey rsa:2048
-days 3650 -sha256 -subj "/CN=Invented CMS interoperability fixture"` and
`cms -sign -binary -md sha256 -outform DER` over exact CRLF CSV bytes. The
newly generated unencrypted test key existed only in a temporary directory,
which was removed immediately. No private key, PFX or password is retained.
SHA-256 and sizes of the three public files are recorded in manifest.json.
Regenerating an ephemeral key/certificate changes fixture hashes; tests use the
recorded fixed public bytes, not a claim of deterministic certificate generation.
Live bidirectional RSA/ECDSA OpenSSL tests generate new credentials per run.
