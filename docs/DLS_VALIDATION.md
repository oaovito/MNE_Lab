# DLS evidence and validation scope

The current parser is `lightscattering-parser/1.4.0`, with
`ls-spec/0.3-provisional`. The evidence below tests faithful extraction from
the available private reports. It does not validate the instrument's acquisition
or analysis algorithms. Original files, identifiers, values and per-file proofs
remain in the private recovery package, outside the repository and builds.

## Independent extraction comparison

The native Go parser and a separate Python reader processed the same 54
single-measurement UTF-8 text reports. Both verified the originals against the
private corpus manifest. The independent reader uses exact source labels,
Decimal-to-float conversion and explicit source tables; it does not call the
product parser. All 54 product results retained `partial` status.

| Source content | Extraction evidence | Scientific limit |
|---|---|---|
| Effective Diameter, Polydispersity, Average Count Rate, Baseline Index | 216 reported quantities; 1,080 exact comparisons of label, raw token, numeric value, explicit unit and physical source line | Three of these fields lack source units in all 54 reports. Missing units stay missing; extraction does not prove units or acquisition semantics. |
| Current Count Rate | Absent in these reports; remains absent | Average Count Rate does not establish a current reading or its acquisition definition. |
| Lognormal / Multimodal, each report / spreadsheet representation | 216 separate candidates, 5,616 points; 4,104 equality checks covering identities, headers, raw/value arrays, source lines and auxiliary fields | The original method names are preserved; fitting/inversion, method applicability and scientific performance are not independently reproduced. |
| `d(nm)` | Explicit source header retained as diameter in nm | No missing scalar unit is borrowed from this table header. |
| `G(d)` | Original values and missing unit retained | No percent normalization or conversion to volume/number weighting. The internal intensity key remains a provisional interpretation. |
| `C(d)` | 5,616 original auxiliary values and lines preserved | Unmapped; no cumulative/quality/other scientific meaning is assigned. |

For all 108 method pairs in this corpus, report and spreadsheet representations
have equal point counts, identical diameter tokens and identical G tokens and
numeric values. This observation applies to these files only. Both candidates
remain separate: neither is preferred automatically, and other exports can
have different precision. Invented regression fixtures exercise that case.

The private readers and reports are `compare-dls-literal.py`,
`compare-dls-distributions.py`, and the corresponding native extraction helpers
and JSON/log/exit files. Their scope is these explicit single-run layouts,
not all supported encodings, delimiters, workbooks or multi-run exports.
Public parser/graph/export regression fixtures cover additional layouts and
error handling; they contain invented data.

## Remaining scientific evidence

| Capability | Available evidence | Required before a broader validation claim |
|---|---|---|
| Summary quantities | Product field list, privately consulted owner references and literal exports | Instrument/version-specific definitions of Effective Diameter, reported Polydispersity, Baseline Index and current/average count rates; units and relevant acquisition/settings conventions; paired trustworthy reported results. |
| Lognormal / Multimodal interpretation | Explicit method headings and `d(nm) G(d) C(d)` layouts | Appropriate manufacturer method/export documentation, G/C meanings and scaling, settings and limitations, plus paired official result arrays. |
| Recalculation from acquisition data | No independently reproduced acquisition-to-result calculation | Real correlation/acquisition data with documented timebase, wavelength, angle, medium properties and the algorithm/settings actually used; a reliable reference result and stated numerical tolerances. No parameter is supplied by inference. |
| Quality and replication | Source fields and existing explicit physical-unit review | Instrument-specific quality definitions and experimental design evidence. Run counts, filenames and imported rows do not establish independent experimental replicates or universal PDI/quality thresholds. |

The supplied Zetasizer size-analysis technical note provides DLS context. It
does not define NanoBrook export fields or demonstrate NanoBrook numerical
equivalence, and it is not documentation of Zeta potential. ZETA and NTA retain
their separate pending scientific status and exact blockers in
[TECHNICAL_CONTEXT.md](TECHNICAL_CONTEXT.md).

Validation requires appropriate documentation, reproducible results and a
comparison with a trustworthy reference. Literal extraction comparisons are
one part of that evidence, not a substitute for the missing scientific parts.
