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
scientific ODS import, distribution mapping and richer import recipes remain pending.
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

## Explicit delimited DLS mapping (PARTIAL / UNVALIDATED)

CSV/TSV can be mapped explicitly to the five existing DLS scalar fields:
Effective Diameter, Polydispersity, Current Count Rate, Average Count Rate and
Baseline Index. The UI never assigns these from an unknown header. The user
selects one-based columns, a header record, first/last data records, delimiter
(comma/semicolon/tab), decimal separator (dot/comma), optional sample column,
and units. A unit string is a declaration, without recognition or conversion.
An empty unit remains unknown even when a header happens to contain `(nm)`.
A conflicting header is retained literally; it is never rewritten to match a
user choice. This workflow does not establish instrument compatibility or
scientific validity. ZETA/NTA fields and distributions cannot be mapped here.

`ImportSelection.mapping` has schema 1 and module `lightscattering`. Mapping
recipes cannot also select workbook sheets. Record indices count CSV records
returned by the delimited reader (blank physical lines do not count); quoted
multiline records count once. Quantity provenance uses the actual physical line
and one-based source column. Explicit last-record selection never truncates the
validation of the remaining source. File/line/column/cell limits match the
bounded text reader: 32 MiB input, 100,000 physical lines/records, 1,024 columns,
1,000,000 cells. Confirmation reparses the entire original independently of the
20-row/20-column literal preview and 10-measurement scientific preview.

Mapped values preserve decoded raw cell text, exact header, physical line,
source column, assigned unit and `unitOrigin: user_mapping`. CSV quoting remains
in the immutable original. Numeric parsing trims surrounding whitespace only;
accepts decimal numbers/exponents in the chosen syntax; rejects grouping,
formulas, invalid/nonfinite/overflow/underflow values. Empty and ragged missing
cells stay absent, including selected rows without quantities, rather than
becoming zero or disappearing. A mapping with no quantities cannot be confirmed.
There are no inferred times, distributions, replicate identities or physical
experimental units. Current and average count rate stay separate, with no scale
conversion. Any invalid mapped number rejects the complete parse atomically.

The reader identity is `dls-column-reader/1.0.0`, with contract
`dls-user-column-mapping/1-unvalidated`. Imported sources remain `partial` and
`USER_DECLARED / PARTIAL / UNVALIDATED`; this is distinct from the native
NanoBrook parser and its provisional contract. Review receipts bind every
canonical choice, original SHA, name, reader/spec, account/profile and expiry.
Editing a choice requires fresh inspection. Duplicate detection includes the
recipe, including its transactional check. Encrypted saved-profile schema 2
stores only a reviewed CSV/TSV recipe; schema 1 XLSX selections remain compatible.
Neither saved recipe contains source bytes, original hash or a cached receipt.
Applying a saved recipe to any file requires a new review.

Graph/cycle source snapshots copy recipes and assigned quantities independently;
analysis snapshots retain the declaration and warn about unvalidated sources.
Measurement/cycle/analysis exports preserve raw values, unit origin and source
columns/labels/lines. These extra flat columns appear only for mapped data;
existing native-only export layouts stay compatible. Structured provenance
retains original SHA and recipe. Tests of these mechanics validate software
behavior, not the scientific meaning chosen by a user. ODS
scientific import, distribution mapping, additional adapters and instrument-reference comparisons
remain pending. No historical quantity is relabelled or migrated.


## Explicit XLSX DLS mapping (PARTIAL / UNVALIDATED)

Workbook mapping is a separate `ImportSelection.mapping` schema **2**:
`sheet`, `headerRow`, `firstRow`, `lastRow`, optional `sampleColumn`, the same
five scalar `columns`, assigned units and a text-cell `decimal` choice.
Delimited `headerRecord/firstRecord/lastRecord` must be zero and `delimiter`
must be empty. Sheet-selection recipes cannot be combined with it. Each
selected physical row is an observation, including missing/omitted rows. The
source range of each observation is its original A1 row extent, with the
original sheet, quantity column and physical row retained. The original blob
and its SHA remain complete and immutable.

Numeric OOXML cells are read with `RawCellValue: true`: declared `<v>` digits
are preserved, including trailing zeros; locale/display formatting does not
replace them. Their decimal syntax is always dot as specified by OOXML.
The user's dot/comma choice applies to shared/inline text cells only. Boolean,
date, error and formula-result cells are rejected as assigned numeric values.
No dates, physical experimental units, units from headers or scale conversion
are inferred. Invalid numeric values reject the whole source without partial
persistence. Original source headers remain literal even when they conflict
with user-assigned units.

All parts and all sheets are validated before confirmation, including excluded
sheets. Formulas/macros, merged cells, external relationships and ambiguous,
duplicate or conflicting physical coordinates are rejected. Rows/cells without
explicit coordinates are outside this supported subset. Limits are 32 MiB
input, 64 MiB expanded package, 4,096 ZIP entries, 256 sheets, 100,000 physical
rows across sheets, 16,384 columns and one million cells. The bounded literal
and scientific previews never supply the confirmed observations.

The reader is `dls-workbook-column-reader/1.0.0`, with specification
`dls-user-workbook-mapping/1-unvalidated`. Saved-profile schema **3** contains
only the reviewed XLSX recipe. Existing schema 1 native workbook selections,
schema 2 saved CSV/TSV recipes and their parser identities remain unchanged.
Every sheet/row/column/key/unit/decimal/sample choice is bound to the scoped
review receipt and duplicate identity; applying a saved recipe requires fresh
inspection. Failed review retains worksheet choices in the editor so the user
can correct the recipe without importing anything.

Graph, Cycle, Statistics and CSV/TSV/JSON/XLSX data export use the same copied
user-declaration provenance. Sources always remain `USER_DECLARED / PARTIAL /
UNVALIDATED`, with `unitOrigin: user_mapping`. Tests compare an independently
constructed invented XML fixture, immutable originals, receipt changes and
snapshots, including localized light/dark browser workflows. These are software
checks, not validation of scientific interpretation or instrument semantics.
