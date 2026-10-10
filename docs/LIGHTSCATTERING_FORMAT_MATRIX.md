# LIGHTSCATTERING format, content and validation matrix

Date: 2026-10-10 UTC. Status: **PARTIAL / VERSION-SPECIFIC DISCOVERY**.
Stable format IDs join the tables below. “HIGH CONFIDENCE” documentation of an
export operation does not mean “CONFIRMED SUPPORTED FORMAT” in MNE Lab.
Scientific validation requires documented semantics, reproducible results and
comparison with trustworthy references. The table is an inventory of known
outputs and explicit research gaps, not a claim to enumerate every software
version/configuration.

Sources: [SOURCE_REGISTRY](LIGHTSCATTERING_SOURCE_REGISTRY.md).
Queries, claims, conflicts and decisions: [WEB_RESEARCH](LIGHTSCATTERING_WEB_RESEARCH.md).

## Identity and producer

MIME/magic are carrier inspection cues, never sufficient to authenticate a
vendor/exporter, identify a scientific module or assign a unit. “Unknown” means
not established by the currently obtained evidence.

| ID / name | Extension | MIME / magic cue | Origin / native or generic | Equipment / software / versions | Category |
|---|---|---|---|---|---|
| LS-F01 observed labeled DLS text | `.txt`; related delimited layouts require their own content checks | `text/plain`; no unique magic; checked labels/tables and encoding | Genuine supplied reports; exporter-origin/build not independently authenticated | Owner identifies NanoBrook90Plus context; actual application build unknown | Reported scientific results, not demonstrated raw acquisition |
| LS-F02 selected-measurement CSV | `.csv` named in guides; delimiter/encoding unspecified | `text/csv`; no vendor magic established | Documented Particle Solutions export | PS2.3/4.0, NanoBrook family; configuration-specific applicability pending | Scientific tabular results |
| LS-F03 selected-measurement Excel | Exact extension unspecified by “Excel” file-menu text | Unknown actual Excel variant; OOXML ZIP or legacy BIFF must be checked on a real file | Documented Particle Solutions export | PS2.3/4.0; family manual software build unspecified | Scientific tabular results |
| LS-F04 distribution XLS | `.xls` explicitly stated | Legacy Excel carrier/BIFF must be established; CFB alone is insufficient | Documented DLS distribution export | PS2.3 p12; PS4 equivalent choices not established | Scientific distribution table |
| LS-F05 distribution CSV | `.csv` explicitly stated | `text/csv`; schema/encoding/locale unspecified | Documented DLS distribution export | PS2.3 p12; PS4 equivalent choices not established | Scientific distribution table |
| LS-F06 native archive | `.bak` in family manual p71; PS4 archive extension unconfirmed | Proprietary binary; magic/MIME/schema unknown | Native Save/Load Archive operation | Family manual unspecified build; archive operation also PS2.3/4.0 | Native measurement archive |
| LS-F07 internal measurement database | Unknown | Unknown engine/container/magic | Native Particle Solutions storage | PS2.3/4.0 documented database operations; physical format/version compatibility unknown | Measurement database |
| LS-F08 measurement SOP | Unknown; do not invent `.sop` from its name | Unknown | Native saved measurement configuration | DLS quick-start build unspecified; PS4 refers to separate SOP directory | Configuration/SOP |
| LS-F09 column layout | Unknown | Unknown | Native saved display-column layout | PS2.3/4.0 | Presentation configuration |
| LS-F10 PDF report/graph/distribution presentation | `.pdf` | `application/pdf`, `%PDF-`; not vendor identification | Documented output | PS2.3 report/graph/distribution PDF; PS4 report PDF explicit | Report/figure/reference, not a primary dataset parser |
| LS-F11 PNG/JPEG graph outputs | `.png`, `.jpg`/`.jpeg` are research candidates, not confirmed PS exports | Standard PNG/JPEG carriers only | Native exporter capability UNCONFIRMED; user-created figures may exist | Model/software/version unknown | Figure/presentation |
| LS-F12 generic labeled text table | `.txt` | `text/plain`; compatible label/table structure required | User-created/edited compatible table; creator declaration pending | No vendor version implied | Generic scientific table |
| LS-F13 generic CSV/TSV scalar table | `.csv`, `.tsv` | `text/csv` / `text/tab-separated-values`; no vendor magic | Generic user table with explicit reviewed mapping | No instrument/software version inferred | Generic scientific table |
| LS-F14 generic XLSX workbook | `.xlsx` | OOXML ZIP, checked workbook parts; `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` | Generic user workbook or unknown-origin compatible labeled workbook | No native NanoBrook export claim | Generic scientific workbook |
| LS-F15 generic XLS workbook | `.xls` | Legacy BIFF/Excel requires its own reader; typical CFB carrier does not prove workbook/schema | Generic workbook; genuine PS2.3 distribution XLS is separately LS-F04 | Source/version unknown | Generic scientific workbook |
| LS-F16 generic ODS workbook | `.ods`; content MIME checked even with `.zip` filename | ODF ZIP `mimetype=application/vnd.oasis.opendocument.spreadsheet`; supported literal subset1.3 | Generic user workbook, not documented PS export | ODF1.3 literal reader; no instrument version assigned | Generic table/container, scientific import pending |
| LS-F17 Malvern DTS | `.dts` | Microsoft CFB signature + checked Header/REC envelopes; neither proves science | Genuine supplied Malvern interoperability original; not Brookhaven | Envelope version-like5.x/6.x/7.x strings; role unknown | Native result/container investigation; module UNKNOWN |
| LS-F18 other native project/result formats | Unknown | Unknown | Research slot, existence/schema not established beyond known archive/database operations | Exact90Plus/legacy/PS versions must be investigated | Native project/result capability UNCONFIRMED |

## Content and coverage

This table distinguishes what documentation says the software can produce from
what the supplied file actually contains. “Unknown” never becomes an empty
scientific array, zero, inferred timestamp or synthetic measurement.

| ID | Raw / derived / report / configuration content | Metadata / measurement parameters | Distributions | Scalar results | Timestamps / time series | Units |
|---|---|---|---|---|---|---|
| LS-F01 | Observed summary/results and named method tables; no reproduced raw-correlation calculation | Explicit source labels/raw tokens/physical lines; original retains unmapped content | Actual Lognormal/Multimodal report and spreadsheet alternatives, d/G/C arrays | Actual Effective Diameter, Polydispersity, Average Count Rate, Baseline Index in real54 corpus; Current absent there | Only explicitly supported source dates elsewhere; report order/import time is not acquisition time; raw timebase not established | `d(nm)` explicit; missing scalar/G/C units stay missing |
| LS-F02 | Selected processed measurement export documented; full/raw content unknown | Exact columns/settings/schema unknown | Not guaranteed by selected-measurement export description | Exact list/precision unknown | Unknown retention/schema | Unknown until real headers/schema |
| LS-F03 | Selected processed measurement export documented; exact workbook variant/content unknown | Sheets, metadata/settings/cell types unknown | Not guaranteed | Exact list/precision unknown | Unknown | Unknown |
| LS-F04 | DLS distribution export documented | Actual workbook/schema/settings unknown | Documented distribution-data export; axes/weighting/arrays need genuine file | Possible other fields not asserted | Unknown | Must read explicit headers/reference; no inferred percent |
| LS-F05 | DLS distribution export documented | Actual columns/schema/settings unknown | Documented; exact weighting/arrays/scaling unknown | Unknown additional content | Unknown | Must read source headers/schema |
| LS-F06 | Measurements plus details unavailable in a printed report, according to manual; raw/derived coverage unknown | Unknown serialization; no assumption all settings are retained | Unknown | Unknown | Unknown | Unknown |
| LS-F07 | Stored measurements; native serialization and raw/derived completeness unknown | Unknown storage schema | Unknown | Unknown | Unknown | Unknown |
| LS-F08 | Configuration/SOP; not an observation | Measurement settings documented in UI; serialized fields/schema unknown | Analysis settings may exist; no measured distribution implied | No measured result inferred | Time-dependent settings exist; not measured time-series data | Serialized unit representation unknown |
| LS-F09 | Display-column layout; not measurements | Display configuration only as documented; schema unknown | None implied | None implied | None implied | No scientific unit assigned |
| LS-F10 | Human-readable report, figure or distribution presentation | Only what the actual document shows; embedded data completeness unknown | May display distributions; faithful machine-readable arrays not established | May display results, not assumed complete | May display dates; no raw timebase guarantee | Visible labels may be reference evidence; no OCR-to-science parser |
| LS-F11 | Figure only; exporter capability unconfirmed | Presentation metadata, if present; no instrument metadata inferred | Rendered curves are not original numeric arrays | No measured value extracted | No acquisition time inferred from image metadata | No scientific unit inferred from pixels |
| LS-F12 | Compatible labeled data only; source bytes immutable | Reader-preserved labels/tokens; creator/exporter unknown | Only supported explicit tables, with provisional interpretation | Only recognized explicit fields | Only explicit supported date tokens; no replicate/time assumptions | Source units only; absent stays absent |
| LS-F13 | Explicit user-declared scalar mapping, PARTIAL/UNVALIDATED | Reviewed columns/rows/decimal syntax, originalSHA, source coordinates, unitOrigin=user_mapping | Mapping not implemented | Five supported scalar field choices; missing cells remain absent | Timestamp/replicate mapping not implemented | Assigned by user, retained as a declaration rather than a measured unit assertion |
| LS-F14 | Explicit mapped scalar workbook or compatible literal labeled data | Whole original, sheet/range/cell coordinates, raw cell tokens and reviewed recipe | Explicit distribution mapping pending; compatible labeled tables remain provisional | Recognized fields or five mapped scalar choices | Only supported explicit source behavior; no inferred experimental unit | Source units or explicit user-mapping provenance; no silent conversion |
| LS-F15 | No scientific XLS decoding currently | Unknown pending reader/fixture | Unavailable | Unavailable | Unavailable | Unavailable |
| LS-F16 | Literal cells/declared typed attributes only, bounded preview | Addresses, display text, declared values, repetitions; no scientific normalization | Not normalized | Not normalized | Declared date/time may be previewed separately; not normalized acquisition time | Not assigned |
| LS-F17 | Container/record envelopes only; original bytes preserved | Structural metadata/hashes; unlabelled paths never executed | Not decoded | Not decoded | Import time separate; measurement time unknown | Not assigned |
| LS-F18 | Unknown | Unknown | Unknown | Unknown | Unknown | Unknown |

## Implementation, evidence and validation

“Synthetic” fixtures test product mechanics. They cannot substitute for a
genuine native export or validate the manufacturer's measurement algorithms.

| ID | Parser / preservation status | Real fixture? / synthetic fixture? | External evidence | Validation / confidence / conflicts |
|---|---|---|---|---|
| LS-F01 | Existing parser1.4.0 / ls-spec0.3-provisional; immutable original, provenance, Library/Graph/Cycle integration | Yes: private54; yes: invented regressions | Legacy90Plus/BI-MAS reference still pending; obtained family manual supplies context, not exact Lognormal/Multimodal serialization | CONFIRMED observed extraction within documented corpus; PARTIAL/PROVISIONAL science; software-generation/method/unit conflicts open |
| LS-F02 | Explicit generic CSV mapping available; native versioned PS CSV adapter not established | No genuine paired PS export; yes generic synthetic mapping | PS2.3 p16 / PS4 p14 / family p19 | HIGH CONFIDENCE documented export capability; NOT CONFIRMED SUPPORTED native schema; generic user declaration UNVALIDATED |
| LS-F03 | Generic XLSX reader/mapping available; no assumption about native Excel variant | No genuine paired PS Excel export; yes invented XLSX | Same selected-measurement references | HIGH CONFIDENCE “Excel”; extension/schema UNCONFIRMED |
| LS-F04 | Native XLS and distribution mapping pending | Genuine no; successful scientific XLS fixture no | PS2.3 p12 | HIGH CONFIDENCE version-specific documented format; scientific/parser validation pending |
| LS-F05 | Generic scalar mapping is insufficient for distribution data; native distribution adapter pending | Genuine no; generic synthetic does not cover native distribution schema | PS2.3 p12 | HIGH CONFIDENCE version-specific capability; axes/weighting/scaling UNRESOLVED |
| LS-F06 | No binary decoder. Generic unsupported-original archival exists via backend; recognized Brookhaven archive workflow pending | Genuine `.bak` no; no invented scientific binary schema | Family p71; archive operation PS2.3 p16 / PS4 p14 | HIGH CONFIDENCE manual extension/operation; magic/schema/version compatibility UNCONFIRMED; `.bak` alone is ambiguous |
| LS-F07 | No native database reader | Genuine no / synthetic no | Same file-menu references | HIGH CONFIDENCE database operations; engine/physical format UNCONFIRMED |
| LS-F08 | No SOP reader/execution | Genuine no / synthetic no | DLS guide p4; PS4 p2 | HIGH CONFIDENCE file-based settings; extension/schema UNCONFIRMED |
| LS-F09 | No vendor layout reader | Genuine no / synthetic no | PS2.3 p6 / PS4 p5 | HIGH CONFIDENCE saved-layout operation; extension/schema UNCONFIRMED |
| LS-F10 | No primary scientific PDF/OCR parser; use legitimate paired reports as reference | Genuine exported paired report no; manuals are documentation, not an instrument export fixture | PS2.3 p12; PS4 p11 | HIGH CONFIDENCE documented presentation outputs; not scientific dataset validation |
| LS-F11 | No scientific image parser | No genuine PS image export; project figures do not prove export operation | No obtained authoritative PNG/JPEG export statement | UNCONFIRMED as PS export; generic presentation classification only |
| LS-F12 | Compatible labeled text reader only; arbitrary generic-column TXT adapter pending | Private labeled reports yes, but creator/build distinction unverified; invented layouts yes | No obtained native-TXT export enumeration for PS2.3/4.0 | Mechanical extraction partial; no native-exporter authenticity claim |
| LS-F13 | CSV/TSV scalar reader1.0.0; inspection/confirmation recipe and provenance implemented/tested | No verified genuine DLS CSV/TSV reference in this round; no genuine PS schema fixture; invented synthetic regressions yes | Generic transport syntax is separate from vendor capability | USER_DECLARED / PARTIAL / UNVALIDATED; distribution/time mapping pending |
| LS-F14 | Bounded XLSX review, selection and scalar mapping implemented/tested | Private Zeta compiled workbook is not a DLS native fixture; invented DLS workbook yes | Generic carrier support; no evidence of actual native PS XLSX extension | PARTIAL/UNVALIDATED DLS interpretation; explicit producer-origin field still pending |
| LS-F15 | Unsupported scientific import; original archival path does not decode BIFF | Genuine relevant fixture no / malformed rejection tests only | PS2.3 supports distribution XLS, but does not specify BIFF version/schema | Reader/paired-reference blocker; no support claim from opening a CFB container |
| LS-F16 | ODS1.3 literal preview1.0.0, no confirmation/science receipt; unsupported original backend archival | Genuine instrument ODS no; independent invented ODS fixture yes | OASIS generic standard; no native PS ODS evidence | PARTIAL container interoperability; scientific mapping pending |
| LS-F17 | PARTIAL CFB/envelope; safe original/library/provenance; no scientific observations | Private genuine file yes; wholly invented CFB/envelope fixtures yes | MS-CFB standard + independent stream comparisons; vendor science schema missing | Container extraction CONFIRMED; scientific semantics UNVALIDATED; not NanoBrook evidence |
| LS-F18 | No claimed recognition/decoder | No / no | Research gap; no explicit additional project/result format identified | UNCONFIRMED; do not fabricate an extension or imply exhaustive discovery |

Native and generic inputs remain different concepts. A compatible table can be
useful without being an authenticated instrument export. Current parser/spec,
recipe, source-coordinate and unit-origin provenance distinguish processing;
`SourceFile.Origin` currently records import/sync/package transport, not the
producer. A separate evidence/user-declared producer-origin contract remains
implementation work. Graph generation, signature verification and successful
container extraction never change scientific validation status.
