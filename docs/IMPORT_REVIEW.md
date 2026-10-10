# Import review

Desktop file selection and drag/drop inspect the original before saving it.
Inspection does not write a record, blob, synchronization operation or backup.
It reports detected scientific fields, original precision and units, warnings,
unknown fields and alternative distributions. A preview is limited to ten
measurements, fifty fields and twenty-five distribution points; confirmation
reparses the complete original. Individual preview strings are also bounded
without splitting UTF-8 characters; truncation is explicitly identified. No distribution candidate is chosen implicitly.

For XLSX, inspection lists every worksheet, its original row/column extent and
literal first nonempty cells. Users can select worksheets and rectangular A1
ranges. The range must include its own headers and units; excluded cells are
not used to infer scientific values or sample metadata. Changing the selection
requires a new preview. Formula/macro validation and resource limits still
apply to the whole workbook, including excluded sheets.

Authenticated `POST /api/import/inspect` and `/api/import/confirm` require the
chooser's account/profile headers. Confirmation also requires a process-local
HMAC receipt, bound to the account, profile, filename, original SHA-256,
parser/specification versions and canonical selection. Receipts expire after
thirty minutes or a process restart. They are not cached, persisted or synced.
Changed bytes, selection or scope are rejected before storage.

The original workbook stays byte-identical. File records retain the explicit
selection; measurements retain source sheet, range and physical source rows.
Graph/export provenance copies that recipe, and XLSX provenance includes the
source range. Duplicate detection compares original SHA-256 and selection,
including a check inside the write transaction for concurrent confirmation.
Importing another subset of the same workbook produces a separate dataset.

Users can explicitly save a reviewed XLSX worksheet/range selection in the
current encrypted Profile and apply it to another workbook. Saved selections
contain configuration only, without source bytes, hashes or review receipts.
Applying one requires a fresh inspection before confirming any import. Names
are unique within the Profile; deleting a selection leaves imported datasets
intact. The authenticated `/api/import/profiles` endpoints enforce the same
account/profile scope as inspection. Recipe schema 1 supports XLSX worksheet
selections; it does not map scientific columns or supply missing units.

The legacy `/api/files` endpoint remains available for existing programmatic
clients; the desktop chooser uses the inspect/confirm sequence. Native XLS,
scientific ODS import, manual scientific column mapping and richer import recipes remain pending.
Scientific interpretation remains provisional.

## Literal ODS review

ODS 1.3 ZIP packages have a native bounded, read-only preview. MIME content
identifies the container even with a generic ZIP filename. This reader is
separate from the DLS parser: familiar headers do not produce quantities,
measurements, a scientific module or a review receipt. The reader is
`ods-literal-reader/1.0.0`, without a DLS scientific specification identity.
The desktop offers no confirmation for these cells. Existing programmatic import can still archive
an unsupported original, with failed scientific status and no measurements.

The preview preserves cell addresses, expanded row/column repetitions and
plain paragraph text under ODF whitespace rules. Declared `office:value` and
string/boolean/date/time attributes are shown separately from display text,
without floating-point conversion, percentage scaling or locale/date inference.
Missing, empty and zero remain distinct. Units and scientific fields are not
assigned. ODS selections and saved XLSX recipes are not applied to it.

The supported subset is ODF 1.3 with UTF-8 XML, plain paragraphs/spans and explicit spaces,
tabs and line breaks. Currency, merged/covered cells, richer embedded content,
tracked changes and other versions remain unsupported. This is partial format
interoperability, not general ODS import or scientific validation.

The complete package is checked before showing cells, including XML beyond
visible tables. Formulas and cached formula results, scripts, external sources,
encrypted parts, directives, duplicate/traversing ZIP paths and malformed XML
are rejected. Source bytes remain unchanged. Limits are 32 MiB input, 64 MiB
expanded ZIP contents and repeated cell text, 4,096 ZIP entries, 256 tables,
100,000 total expanded rows, 16,384 columns per row, one million total expanded
cells, 128 XML depth and two million tokens per XML part. Visible output remains
at most ten tables, twenty rows and twenty columns per table, and 256 UTF-8 bytes
per cell string; truncation is explicit.

The reader follows [OASIS OpenDocument 1.3](https://docs.oasis-open.org/office/OpenDocument/v1.3/os/):
Part 2 §3.3 (MIME package part), Part 3 §§6.1.2–6.1.5 (text), §19.389 (declared
values), §19.679.3 and §19.681 (cell/row repetitions). Independently generated
invented fixtures were checked against the official schemas. No real ODS
instrument export has been used as a scientific reference.
