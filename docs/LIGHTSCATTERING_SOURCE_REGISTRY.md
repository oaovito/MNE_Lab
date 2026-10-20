# LIGHTSCATTERING source registry

Reviewed/attempted: 2026-10-10 UTC. Stable IDs identify evidence, not a
scientifically validated parser. Public sources are linked and summarized;
manuals, private project documents, experimental originals and per-file hashes are not
redistributed in this repository. Private receipts retain URLs, byte counts,
SHA-256, document/page locators and unsuccessful requests.

## Sources actually obtained

| ID | Organization / title / source type | URL or private location policy | Date / revision | Equipment / software scope | Review and confidence limits |
|---|---|---|---|---|---|
| BIC-PS-23-QSG | Brookhaven Instruments, “Particle Solutions” Software v. 2.3 Quick Start Guide; Tier 1 manufacturer document | [Official PDF](https://brookhaveninstruments.com/wp-content/uploads/2023/08/Particle-Solutions-Quick-Start.pdf) | Publication date unstated; v. 2.3 on title page; 18 PDF pages; retrieved 2026-10-10. URL upload directory is not a publication date | Particle Solutions 2.3; instrument configuration must be checked separately | File menu p16, distribution/graph controls p12, reports p12, layouts p6. HIGH CONFIDENCE for those documented capabilities; no matching genuine PS2.3 export fixture |
| BIC-PS-40-QSG | Brookhaven Instruments, “Particle Solutions” Software v. 4.0 Quick Start Guide; Tier 1 | [Official PDF](https://www.brookhaveninstruments.com/wp-content/uploads/2026/06/Particle-Solutions-4-Software-Quick-Start.pdf) | Publication date unstated; v. 4.0 on title page; 17 pages; retrieved 2026-10-10. PDF creation metadata says 2024-01-22, URL directory says 2026/06; neither establishes publication/release date | NanoBrook family / PS4.0; no assumption that every optional measurement is licensed on 90Plus | File menu p14, reports p11, layouts p5, data/SOP migration p2. HIGH CONFIDENCE for stated capabilities; exact Excel extension, binary serialization and matching real export unverified |
| BIC-NB-SERIES-13DC | Brookhaven Instruments, Instruction Manual for the NanoBrook Series; Tier 1 | [Official PDF](https://brookhaveninstruments.com/wp-content/uploads/2023/08/Instruction-Manual-for-the-NanoBrook-Series-Rev1.3DC.pdf) | “Rev1.3DC” is in the URL; no publication date or software build established; 72 pages; retrieved 2026-10-10 | p2 explicitly describes all Omni features and warns that other configurations may lack them. Software build unspecified | p19 file menu; p29 NNLS/CONTIN; p68 Baseline Index / legacy Sample Quality; p70 Effective/Mean Diameter; p71 `.bak`. HIGH CONFIDENCE for these statements within document scope; not a universal 90Plus/ZETA contract |
| BIC-DLS-QSG | Brookhaven Instruments, DLS Measurement Quick Start Guide; Tier 1 | [Official PDF](https://brookhaveninstruments.com/wp-content/uploads/2023/08/Particle-Solutions-DLS-Measurement-Quick-Start.pdf) | No publication date/software version established; 24 pages; retrieved 2026-10-10 | Particle Solutions DLS measurement/SOP workflow; configuration unspecified | SOP loading/saving p4; duration/sequence settings p6; time/temperature automation pp7–8. No serialization/export schema supplied. Text extraction emitted an optional-content warning; images were not used as scientific datasets |
| BIC-NB-INSTALL | Brookhaven Instruments, Installing your NanoBrook; Tier 1 | [Official PDF](https://brookhaveninstruments.com/wp-content/uploads/2023/08/Installing-your-NanoBrook.pdf) | No publication/revision date established; 11 pages; retrieved 2026-10-10 | Physical NanoBrook installation | Reviewed for instrument/setup context. Does not establish a data-file schema or model-specific export list |
| LOCAL-LS-SPEC-CURRENT | Owner-supplied consolidated Lightscattering/Zeta project document; project requirements | Private recovery package; no original filename/path/hash exposed here | Supplied CURRENT/LATEST label; publication date unspecified | Project identifies NanoBrook 90Plus / Particle Solutions; does not give an exact software build | Relevant DLS sections reviewed; project intent is preserved. Manufacturer definitions and actual files take separate roles; conflicts in WEB_RESEARCH are unresolved |
| LOCAL-LS-SPEC-PREVIOUS | Owner-supplied previous project document; historical requirements | Private recovery package | Supplied PREVIOUS label; date unspecified | Same project context; cites legacy 90Plus/BI-MAS material | Relevant DLS sections reviewed. No silent replacement by current internet documentation |
| LOCAL-MALVERN-DLS-TN101124 | Malvern Panalytical, Zetasizer Nano Analysis Methods Explained; supplied manufacturer technical note | Private supplied PDF; [public reference](https://www.malvernpanalytical.com/br/learn/knowledge-center/technical-notes/tn101124zetasizernanoanalysismethodsexplained) | Identifier tn101124; supplied copy copyright 2025, not necessarily initial publication date; four pages | Zetasizer Nano size/DLS; text refers to DTS v6.XX | Reviewed size-analysis context. Does not define Brookhaven NNLS/CONTIN or Lognormal/Multimodal export semantics; does not validate Zeta potential |
| REAL-LS-TXT-54 | Owner-provided genuine single-measurement text corpus; direct observed files | Private originals, manifests and independent readers | Instrument/software build not established from the checked explicit version labels | Owner's LIGHTSCATTERING context; no version inferred from filename | 54 reports cross-checked; four summary labels, Lognormal/Multimodal alternatives and d/G/C tables. CONFIRMED observed layout/extraction only; science remains PARTIAL |
| REAL-COMPILED-ZETA-XLSX | Owner-supplied compiled Zeta workbook; generic table corpus | Private development corpus | Native exporter/version unknown | ZETA development, not a genuine NanoBrook Excel-export reference | Does not establish native PS XLS/XLSX output or ZETA scientific validation |
| REAL-MALVERN-DTS | Owner-supplied `.dts`; private interoperability material | Private original and independent CFB stream comparison | Observed envelope strings include 5.x/6.x/7.x families; string role is not confirmed software version | Malvern record-envelope investigation; scientific module UNKNOWN | CONFIRMED container/envelope extraction, PARTIAL / UNVALIDATED semantics. Not evidence of a Brookhaven format |
| PROJECT-IMPORT-CONTRACT | Current audited MNE Lab source and [IMPORT_REVIEW.md](IMPORT_REVIEW.md), [DLS_VALIDATION.md](DLS_VALIDATION.md), [DTS_FORMAT.md](DTS_FORMAT.md) | Public synthetic fixtures and source; private numeric comparison receipts remain private | Parser 1.4.0 / ls-spec 0.3-provisional; column reader 1.0.0; ODS literal reader 1.0.0 | Software behavior, not instrument acquisition validation | Tests show bounded parsing, immutable originals and explicit mappings. Passing software tests does not authenticate an exporter or establish science |

The seven supplied PDF instances across the two packages collapse to three
unique documents by exact byte hash. They are not seven independent sources.
The five newly downloaded Brookhaven PDFs are also one manufacturer's body of
documentation, not independent laboratory replication.

## Discovery and standards references

| ID | Source / tier | Pointer and status | Intended use / unresolved scope |
|---|---|---|---|
| KENT-PS-QSG-DISCOVERY | Kent State University Liquid Crystal Institute host; expected Tier 2 hosting of a manufacturer manual | [PDF pointer](https://www.lcinet.kent.edu/organization/facility/characterization/Zeta/Zeta_Doc/Particle%20Solutions%20Quick%20Start.pdf), discovered through search; HTTPS tunnel403, body not retrieved | Search title suggests v2.0; actual title/version/hash unverified. Do not equate it with the obtained v2.3 PDF |
| BIC-LEGACY-90PLUS-BIMAS | Manufacturer manual citation in supplied project documents; primary document not obtained | Title: 90Plus/BI-MAS Multi Angle Particle Sizing Option Operation Manual; exact official URL/revision pending | Establish legacy exporter/layout history. A citation or embedded screenshot does not supply the full manual/schema |
| BIC-PS4-PRODUCT-DISCOVERY | Official product page; expected Tier 1 | [Particle Solutions v4](https://www.brookhaveninstruments.com/products/nanobrook-particle-solutions-software-v4/); HTTP403, body not obtained | Search snippet is discovery only; no release-note or Windows compatibility claim based solely on it |
| STANDARD-ODF13 | OASIS OpenDocument 1.3; Tier 1 format standard | [Specification](https://docs.oasis-open.org/office/OpenDocument/v1.3/os/); existing literal-reader contract/reference, not newly fetched in this round | Generic ODS package/cell rules only; no evidence that Particle Solutions exports ODS |
| STANDARD-MS-CFB | Microsoft MS-CFB; Tier 1 container standard | [Specification](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-cfb/); existing DTS container contract | CFB is not itself XLS/BIFF, Malvern science, or a Brookhaven archive schema |

Institutional sources, papers and historical versions remain research targets.
Search results, snippets, catalogs and repeated web claims never independently
validate a native format. No forum/blog rule was added to a parser.

## Public-document byte receipts

These hashes identify the privately consulted official PDF bytes, not their
scientific validity or permission to redistribute the full documents.

| ID | Bytes | SHA-256 |
|---|---|---|
| BIC-PS-23-QSG | 3032843 | `a0ad2d3778f52a631e69e2eaf7efa698ebf8dd24223daaf200803171a684e226` |
| BIC-PS-40-QSG | 4068180 | `6f862e8388f66f32792b50cc1ea2e820f99fe8cdd7106a265b5b9e0e72d5b3a2` |
| BIC-NB-SERIES-13DC | 1972659 | `7312bfb350d5812eadfc243416d6765b28712ac81ef6938b69a1d49a236a9ab3` |
| BIC-DLS-QSG | 1244186 | `1d25e1a6445fda720b5dacd5c093c88736cbca0a3c458f80081d6023d009297d` |
| BIC-NB-INSTALL | 1868364 | `54bc97002bd69d4434b09673c681c8f51d9926ab8eea42aa0fcbf7fb48acd12a` |
