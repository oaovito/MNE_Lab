# LIGHTSCATTERING web research and scientific decisions

Research date: 2026-10-10 UTC. Status: **PARTIAL DISCOVERY**. Manufacturer
documentation was obtained and compared with local evidence; exhaustive output
coverage, exact 90Plus configuration/version and native-format validation are
not complete. This research adds no scientific parser and promotes no DLS,
ZETA or NTA validation status.

Source IDs resolve in [LIGHTSCATTERING_SOURCE_REGISTRY.md](LIGHTSCATTERING_SOURCE_REGISTRY.md).
Format/content/parser states are separate in [LIGHTSCATTERING_FORMAT_MATRIX.md](LIGHTSCATTERING_FORMAT_MATRIX.md).

## Reproducible search and access log

| Query / operation | Source and exact URL | Retrieved / attempted | Outcome, cross-check and decision |
|---|---|---|---|
| Manufacturer home | [Brookhaven](https://www.brookhaveninstruments.com/) | 2026-10-10 | Initial tunnel403; later HTTP403. Does not prove individual official PDFs are unavailable |
| NanoBrook 90Plus Particle Solutions export database native file format | [Bing query](https://www.bing.com/search?q=NanoBrook+90Plus+Particle+Solutions+export+database+native+file+format) | 2026-10-10, HTTP200 | Decoded results were unrelated insurance pages; excluded as irrelevant. HTTP200 alone is not useful evidence |
| Particle Solutions software manual release notes CSV Excel export Brookhaven | [DuckDuckGo query](https://html.duckduckgo.com/html/?q=Particle+Solutions+software+manual+release+notes+CSV+Excel+export+Brookhaven) | 2026-10-10, HTTP200 | Discovered official PS4 guide/product page and Kent manual. Only subsequently obtained full documents support claims |
| Historical Brookhaven 90Plus manual | [Internet Archive CDX query](https://web.archive.org/cdx/search/cdx?url=brookhaveninstruments.com%2F%2A90Plus%2A&output=json&filter=statuscode%3A200&collapse=urlkey) | 2026-10-10, HTTP200 | Empty result. Narrow domain/path query is not proof that historical manuals do not exist |
| site:brookhaveninstruments.com NanoBrook Instruction Manual Rev1.3DC file export | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=site%3Abrookhaveninstruments.com+NanoBrook+Instruction+Manual+Rev1.3DC+file+export), URL constructed with the exact query above | 2026-10-10, HTTP202 | Challenge page, no usable results; not recorded as a successful search |
| "90Plus" "BI-MAS" manual pdf export | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=%2290Plus%22+%22BI-MAS%22+manual+pdf+export) | 2026-10-10, HTTP202 | Challenge; legacy manual retrieval remains pending |
| "Particle Solutions" "database" "archive" export file format | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=%22Particle+Solutions%22+%22database%22+%22archive%22+export+file+format) | 2026-10-10, HTTP202 | Challenge; no claimed result |
| "Particle Solutions" "version 2" export Excel CSV manual | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=%22Particle+Solutions%22+%22version+2%22+export+Excel+CSV+manual) | 2026-10-10, HTTP202 | Challenge; actual official guide later obtained independently and identified as 2.3 |
| "NanoBrook" "90Plus" "Omni" software manual export | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=%22NanoBrook%22+%2290Plus%22+%22Omni%22+software+manual+export) | 2026-10-10, HTTP202 | Challenge; model scope checked directly in BIC-NB-SERIES-13DC p2 |
| "Particle Solutions" release notes v3 v4 software file format | [Exact DuckDuckGo query](https://html.duckduckgo.com/html/?q=%22Particle+Solutions%22+release+notes+v3+v4+software+file+format) | 2026-10-10, HTTP202 | Challenge; no release notes obtained; version3 remains unverified |
| "90Plus" "BI-MAS" manual pdf | [Google query](https://www.google.com/search?q=%2290Plus%22+%22BI-MAS%22+manual+pdf) | 2026-10-10 | Tunnel403; no body read |
| "NanoBrook" "Particle Solutions" "export" file | [Google query](https://www.google.com/search?q=%22NanoBrook%22+%22Particle+Solutions%22+%22export%22+file) | 2026-10-10 | Tunnel403; no body read |
| Follow supplied official manual/quick-start pointers | BIC-PS-23-QSG, BIC-NB-SERIES-13DC, BIC-DLS-QSG, BIC-NB-INSTALL; exact URLs in registry | 2026-10-10 | `www` variants returned403; official non-www HTTPS URLs returned200, valid PDF signatures and matching internal titles. TLS verification remained enabled |
| Follow newly discovered version4 guide | BIC-PS-40-QSG, exact official URL in registry | 2026-10-10, HTTP200 | 17-page title identifies4.0. Publication date not inferred from path or PDF creation metadata |
| Institutional triangulation | KENT-PS-QSG-DISCOVERY | 2026-10-10 | Tunnel403; title/version not independently checked. Search snippet remains discovery only |

Exact encoded URLs for every search, response status, private byte receipts,
document hashes and page locators are retained in the private research log.
No CAPTCHA was solved/bypassed; no authenticated vendor download, installer or
instrument program was executed; no external service received private corpus
contents. Manufacturer PDFs were consulted privately and are not committed.

## Claim/evidence matrix

Publication dates are unstated unless explicitly identified in the registry;
all external documents below were retrieved on 2026-10-10. Equipment and
version limits are part of each claim, not inferred from a filename.

| Claim | Evidence / source type / locator | Equipment / software version | Confidence / real-file cross-check | Conflicts / implementation impact |
|---|---|---|---|---|
| Selected measurements can be exported as Excel or CSV | BIC-PS-23-QSG p16; BIC-PS-40-QSG p14; BIC-NB-SERIES-13DC p19; manufacturer documents | Particle Solutions2.3/4.0 and NanoBrook-family manual of unspecified software build | HIGH CONFIDENCE documented capability; no paired genuine export from these versions | “Excel” does not establish `.xlsx` versus `.xls` or every column/unit. Keep explicit generic mappings PARTIAL; obtain native fixtures before a versioned PS schema |
| DLS distribution data can be exported as XLS, CSV or PDF | BIC-PS-23-QSG p12, explicit distribution-data control; manufacturer | PS2.3, DLS; model-specific applicability still needs actual configuration | HIGH CONFIDENCE documented capability; no genuine distribution XLS/CSV fixture | PS4 text does not explicitly enumerate the same distribution choices; absence does not prove removal. Native XLS reader/distribution mapping pending; PDF remains report/reference |
| Reports and graphs can be emitted as PDF | PS2.3 p12 identifies graph PDF and report PDF; PS4 p11 identifies report PDF | Versioned as stated, supported family configurations | HIGH CONFIDENCE for documented operations; actual export fixtures pending | Report/figure output is not automatically a scientific dataset or raw acquisition source |
| Selected measurements can be saved/loaded in a proprietary binary archive | PS2.3 p16, PS4 p14, family manual p19 | Those versions/family scope | HIGH CONFIDENCE for archive capability; no genuine archive supplied | Binary serialization and content coverage unknown; do not guess offsets, database engines, arrays or scientific fields |
| The family manual's archive save operation appends `.bak` | BIC-NB-SERIES-13DC p71, particle-sizing FAQ18 | Manual scope/unspecified software build; Omni warning p2 | HIGH CONFIDENCE documented extension; real fixture absent | PS4 guide does not identify its archive extension. `.bak` is also a generic extension: never identify Brookhaven by extension alone |
| Database maintenance and legacy measurement import exist | PS2.3 p16; PS4 p14; family manual p19 | Version-specific Particle Solutions operations | HIGH CONFIDENCE for operations, database serialization UNCONFIRMED | Re-index/import operations do not establish SQLite/Access/any other storage engine or compatibility with every old version |
| Column layouts and measurement SOPs are saved separately | PS2.3 p6/PS4 p5 layouts; DLS guide p4 SOP; PS4 p2 Data/SOP backup advice | PS2.3/4.0 layouts; DLS guide build unspecified | HIGH CONFIDENCE operations; extensions/schemas UNCONFIRMED | Configuration/SOP is not a measured dataset. No invented `.sop` extension or archive parser |
| Current corpus contains reported results and two named method alternatives | REAL-LS-TXT-54; independent extraction described in DLS_VALIDATION.md | Actual owner-supplied files; exporter build unknown | CONFIRMED observed layout, not every manufacturer export; PARTIAL science | Preserve exact labels/units/raw tokens, separate report/spreadsheet alternatives and C(d); never synthesize missing acquisition data |
| Family manual describes NNLS/CONTIN, while actual corpus headings say Lognormal/Multimodal | BIC-NB-SERIES-13DC p29; REAL-LS-TXT-54 | Manual/observed legacy-like text of unknown exporter version | CONFIRMED distinct labels; relationship UNRESOLVED | Do not map these method names onto each other or replace current raw-method provenance |
| The provided Malvern note discusses size/DLS analysis, not Zeta potential | LOCAL-MALVERN-DLS-TN101124, introduction and DLS sections | Zetasizer Nano / DTS6.XX context | CONFIRMED document scope | No Brookhaven algorithm equivalence or ZETA scientific-validation promotion |

## Conflicts and decision records

Each decision records reason, evidence, alternative, confidence, validation
and impact. “UNRESOLVED” identifies the disputed interpretation; it does not
discard the observed file or the owner's desired experimental workflow.

| Decision / reason | Evidence and alternative considered | Confidence / validation | Impact |
|---|---|---|---|
| Retain Average Count Rate and Current Count Rate as different fields | Real54 reports have Average and lack Current; project document discusses Current. Alternative: rename Average to Current | Observed label difference CONFIRMED; mapping between acquisition definitions UNRESOLVED | No relabeling; no kcps/Mcps supplied when source units are absent; no photon-to-particle-number conversion |
| Retain Effective Diameter separately from mean of runs and distribution peaks | Family manual p70 distinguishes cumulants-derived Effective Diameter from arithmetic Mean Diameter; project text calls diameter intensity-weighted and describes run averaging. Alternative: treat all as one z-average/peak/mean | Manufacturer explanation HIGH CONFIDENCE in its scope; exact exported/versioned field comparison pending; project/manufacturer interpretation conflict UNRESOLVED | Keep reported scalar and method identity; no averaging/recalculation or name substitution inferred from a heading |
| Retain G(d) and C(d) without assumed percent/volume/number semantics | Real reports contain d(nm), G(d), C(d); project generic plotting guidance expects intensity%/fraction; no obtained manual defines these exact serialized labels | Literal extraction CONFIRMED; scientific mapping/scaling UNRESOLVED | No normalization or conversion. Current intensity key stays provisional; C remains unmapped |
| Keep native method names distinct across product generations | Actual Lognormal/Multimodal versus family manual NNLS/CONTIN; project figures cite legacy90Plus/BI-MAS while project names Particle Solutions | Labels CONFIRMED, version/algorithm correspondence UNRESOLVED | Need legacy manual, exact installed build/settings and paired exports; no universal method adapter |
| Record Baseline Index manufacturer context without implementing its calculation/threshold as a universal rule | Family manual p68 explains normalization/dust context and earlier Sample Quality name; project gives a shorter quality description; actual files supply Baseline Index | Document explanation HIGH CONFIDENCE; acquisition-to-value reproduction not done | Preserve field/unit absence. No synthetic correlation inputs, universal quality cutoff or mass migration |
| Do not equate PDI to charge, component count or a universal acceptability threshold | Project discussion links PDI to sample families/charge; Malvern DLS note discusses cumulants distribution breadth, with its own model. Alternative: universal cross-instrument PDI classification | Cross-source context differs; Brookhaven export/version contract not fully reproduced; UNRESOLVED | Existing numbers retained; no new automatic charge/quality claim or cross-vendor calculation |
| Treat software-origin and carrier/reader as separate provenance | Current SourceFile.Origin is import/sync/package; parser/spec, recipe and unitOrigin record processing. None proves the original creator/exporter | Existing code behavior tested; exporter-origin declaration NOT IMPLEMENTED | Next schema must explicitly record unknown versus user-declared instrument/export versus user-created table, with device/software/version/evidence. Never label every matching CSV/XLSX a genuine vendor export |
| Keep safe preservation separate from scientific import | Existing unknown CFB/ODS behavior preserves metadata/originals with limited UI; no genuine Brookhaven `.bak` schema | Container mechanisms tested; `.bak` recognition/preservation workflow pending | Archive immutable originals within existing size/security limits; require evidence/user declaration rather than extension-based manufacturer detection. No graph/ANOVA from unknown bytes |

## Remaining research and exact next action

1. Retry institutional/historical searches after the network configuration is
   reviewed/saved/published; distinguish tunnel denial, HTTP403 and search
   challenge pages. Obtain the cited legacy90Plus/BI-MAS manual and version2.0,
   version3, version4 release notes or equivalent authoritative change records.
2. Confirm the actual90Plus configuration, detector/options and installed
   Particle Solutions/legacy application build. The Omni-inclusive manual is
   not proof that 90Plus performs PALS/ELS.
3. Obtain genuine selected-measurement CSV/Excel, distribution XLS/CSV, `.bak`
   archive, SOP/layout and paired PDF reference from that installation. Record
   the menu/action, version/settings and actual content; preserve them privately.
4. Establish native database/archive/configuration schema or documented paired
   behavior before decoding; investigate whether raw correlations, measurement
   parameters, units, distributions and time series are actually retained.
5. Add explicit producer-origin provenance and bounded XLS/distribution/ODS
   adapters only with the necessary contract, synthetic regressions and real
   comparison evidence. Origin is a declaration/evidence claim, never inferred
   from extension, a graph, software tests or a successful signature.

The phase is not “mature/complete”: historical/independent corroboration,
native fixtures/schema and several format/version/model details remain open.
Current DLS stays PARTIAL/PROVISIONAL; ZETA/NTA remain scientifically pending.
