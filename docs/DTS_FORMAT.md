# Malvern DTS interoperability: partial, scientifically unvalidated

The adapter reads local Microsoft Compound File Binary containers. It does
not require Malvern software, Excel, Python, R or an external converter.
Container and record inspection are implemented; scientific quantities,
distributions, correlation curves and measurement dates are not normalized.
The file extension does not identify ZETA, DLS or any analysis method.

## Supported layers

1. Content dispatch uses the CFB signature before filename hints. A damaged
   `.dts` does not fall through to the text parser. Valid, unsupported CFB
   structures can be preserved as `unknown_compound` originals.
2. CFB decoding uses `github.com/richardlehane/mscfb` 1.0.7, with a bounded
   directory/FAT preflight. CFB v3/v4 structural checks follow the public
   [Microsoft MS-CFB specification](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cfb/).
   The private real corpus currently exercises v3; this does not establish
   coverage of every v4 file or vendor serialization.
3. The observed envelope adapter recognizes `Header` and `REC<n>` streams.
   Little-endian fields are checked structurally: 16-bit tag, two 32-bit raw
   codes, another 16-bit tag, a 32-bit identifier matching the stream suffix,
   followed by a length/encoding-tagged UTF-16LE version-like string.
   Offsets within this envelope are a checked grammar, not guessed absolute
   offsets into a sample. Raw code meanings are unknown. The string's role
   is not confirmed as instrument software or acquisition version.
4. Further length-prefixed, terminated UTF-16LE strings can be investigated
   with the development-only `cmd/dtsinspect` CLI. Their roles remain unmapped.
   Normal File Library metadata omits these unlabelled strings, which can
   include private names and historical paths. Such paths are never opened.
5. `unknown_malvern_dts` describes observed record-envelope evidence only.
   Analysis method remains `UNKNOWN`; no SOP filename, generic protein
   label, type code or extension silently assigns a scientific method/module.

The observed version-string families cover several 5.x, 6.x and 7.x records.
A new version string permits only safe structural metadata and raises a
warning; it does not inherit scientific coverage. Unlabelled floats, dates,
quality codes, sample properties and numeric arrays remain untouched bytes.

## Persistence and integration

Read-only inspection issues a receipt bound to the actual adapter/spec,
original SHA-256, filename, Account/Profile and reviewed selection. Explicit
confirmation saves an encrypted original and coverage/provenance metadata.
Duplicate handling, scoped File Library access, backup and original export
reuse the shared infrastructure. Import time is separate from measurement
time, which is unknown for these records.

Smart Export offers byte-exact original export and a JSON `source_metadata`
report containing source checksum, parser/spec, raw structural metadata,
coverage and warnings. It does not present an empty table as scientific data.
CSV/TSV/TXT/XLSX scientific exports are blocked when no supported quantities
exist. Metadata-only files produce no graph, cycle or ANOVA observations.
Global Sign Export/Verify remains a separate unimplemented feature.

| Capability | Implementation | Scientific validation |
|---|---|---|
| Original preservation, CFB directory/stream inventory and hashes | Implemented/tested | Not a scientific calculation |
| Observed record envelopes and raw version/code metadata | PARTIAL/tested | UNVALIDATED semantics |
| DLS cumulants summary, intensity/volume/number distributions, correlation | Unavailable | PENDING |
| Zeta potential, mobility, quality and distributions | Unavailable | PENDING |
| Scientific timestamps/settings and module/method classification | Unavailable | PENDING |

Limits include 32 MiB input, 2,048 directory entries, bounded reads, cycle and
size checks, strict UTF-16 length/terminator/surrogate checks, and stable errors.
Embedded executables, macros and external paths are not executed. Synthetic
malformed cases and fuzzing exercise rejection and resource bounds. A
development-only independent `olefile` reader agrees on every stream name,
size and SHA-256 of the private reference. This verifies container extraction,
not scientific validity. Public fixtures are wholly invented containers.

## Evidence still required

- Version-specific native record serialization or equivalent paired official
  exports that unambiguously identify each field, unit, type and record role.
- Official DLS field/method documentation plus matching summary, cumulants,
  intensity, volume, number and correlation exports for the same records.
  Cumulants Z-average and distribution peaks must remain different concepts.
- For ZETA, its own potential/mobility/quality/settings documentation and
  native exports with trustworthy numerical results for the same records.
- Reproducible field-by-field and array-by-array comparisons with documented
  tolerances, including actual source-version coverage. Opening a file or
  producing a plot does not satisfy these requirements.

The supplied analysis-methods PDF concerns DLS/size analysis; it does not
establish potential-Zeta semantics. The private compiled Zeta workbook does
not supply native record serialization or instrument provenance. Real files,
stream dumps, source identifiers, reports and screenshots stay outside Git,
installers, releases and demos.
