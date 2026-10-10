# File producer origin

File Library records an optional `producerOrigin` declaration separately from
`SourceFile.origin`, which remains the import/sync/package transport. A parser,
filename, extension, container signature or familiar column label cannot prove
who produced the file. Existing and newly imported files default to unknown.

The person can declare `instrument_export` or `user_created_table`, with optional
manufacturer, instrument/model, producing software, software version/build and
a source/reference note. These are verbatim bounded text, not a predefined
manufacturer schema. The application neither fetches the note nor treats it as
executable content. Switching to unknown clears all producer details.

Every explicit change is marked `user_declared` and gets a server UTC timestamp.
The timestamp records the declaration, not acquisition or export time. The API
rejects caller-supplied verification status, timestamps, scientific validation,
unsupported kinds, control characters and oversized fields. It requires the
current Account/Profile identity in addition to the normal local authenticated
session and mutation protections. Trashed sources cannot be edited.

Original bytes/checksum, parser/specification, import recipe, transport and
scientific status remain unchanged. Declarations live in the Profile's encrypted
file record, alongside its existing metadata, and follow its ordinary encrypted
backup/sync behavior. They contain no signing credentials.

Graph provenance, Cycle dataset and research-package provenance, dataset JSON,
TXT and XLSX provenance, statistics source snapshots and metadata-only DTS JSON
carry the declaration. XLSX appends a JSON text column without moving existing
columns. CSV/TSV remain value tables; provenance belongs in the existing JSON,
TXT/XLSX or research-package outputs. Old statistics retain their saved source
snapshot; editing a declaration marks the source changed and invalidates stale
review receipts. A new analysis requires a fresh source review.

Software verification uses invented sources: rejection and Profile isolation,
encrypted close/reopen, immutable originals/results, independent graph snapshots,
JSON/TXT/XLSX round trips and three-language browser workflows in both themes.
This feature does not authenticate a native instrument export or validate any
scientific result. DLS remains PARTIAL/PROVISIONAL; ZETA and NTA validation remains
pending. Native format decisions still require the evidence described in
[LIGHTSCATTERING_FORMAT_MATRIX.md](LIGHTSCATTERING_FORMAT_MATRIX.md).
