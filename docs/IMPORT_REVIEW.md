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

The legacy `/api/files` endpoint remains available for existing programmatic
clients; the desktop chooser uses the inspect/confirm sequence. Native XLS/ODS,
manual scientific column mapping and reusable Profile-scoped import recipes
remain pending. Scientific interpretation remains provisional.
