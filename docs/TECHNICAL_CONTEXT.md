# MNE Lab — technical context

This document records how MNE Lab is built, what is finished, what is partial
and what comes next. It is kept up to date with the code.

Version described: 0.1.0 (first development line). Data schema: 1.

## 1. Architecture

MNE Lab is one native executable with an embedded interface.

| Part | Where | What it does |
|---|---|---|
| Core | `internal/app` | Execution mode, unlocked account and profile, sessions, sync, libraries, exports, updates, mobile access and the safe lifecycle. Serves the interface on a private loopback connection. |
| Interface | `web/` (Preact + TypeScript, built with esbuild) | Desktop interface (`web/src/main.tsx`) and phone interface (`web/src/mobile/main.tsx`). Built into `web/dist` and embedded in the executable (`web/embed.go`). |
| Window | `internal/shell` | An application window of the Chromium-based browser already on the computer (Microsoft Edge on Windows), with a private, isolated profile inside MNE Lab's own folder. No browser engine is shipped, which keeps downloads small and memory use low on inexpensive computers. Tray icon through `fyne.io/systray`. |
| Launcher | `cmd/mnelab-launcher` | `MNE Lab.exe` at the root of a Portable USB install. Starts the selected build from `app/<version>/`, falls back to the previous build when a new one keeps failing to start, and exits so it never stays locked. |
| Release tool | `cmd/mnelab-release` | Creates the signing key, writes and signs the release manifest, verifies it. |

The interface talks to the core only through `127.0.0.1` with a session
cookie set from a one-time launch URL. Every request that changes something
must carry the `X-MNE-Lab: 1` header (so no other site can trigger it), and
errors come back as stable identifiers (`{"error":"area.code"}`) that the
interface translates.

### Package map

| Package | Responsibility |
|---|---|
| `internal/paths` | Directory layout for each execution mode. Portable USB Mode never stores absolute paths. |
| `internal/vault` | Accounts, profiles and their keys. |
| `internal/secure` | Wrappers around vetted primitives (XChaCha20-Poly1305, Argon2id, HKDF-SHA256, HPKE). Nothing is implemented by hand. |
| `internal/store` | Encrypted, transactional store of one profile (bbolt), with the sync queue and an audit trail in the same transaction. |
| `internal/syncer` | Replication of a profile store to the user's own cloud. |
| `internal/provider` | Google Drive / Google One, iCloud and Microsoft OneDrive, in that order. |
| `internal/backup` | Verified, rotating snapshots. |
| `internal/atomicfile` | Crash-safe file writes (temporary file, fsync, rename, directory fsync). |
| `internal/update` | Automatic Update, LockedBuild and the Stable build selector. |
| `internal/instance` | One running instance per data folder. |
| `internal/logging` | Local, size-bounded logs that redact secrets. Logs never leave the machine. |
| `internal/media` | Avatar validation and normalization. |
| `internal/i18n` | Translations shared by the interface and the core. |
| `internal/science/model` | Normalized scientific data model. |
| `internal/science/lightscattering` | NanoBrook 90Plus export parser. |
| `internal/science/graph` | Graph Engine. |
| `internal/science/cycle` | Cycles (temporal studies). |
| `internal/science/stats` | Descriptive statistics. |
| `internal/science/reference` | Scientific specifications used, for provenance. |
| `internal/plot` | Layout of a graph as a renderer-neutral display list; the same list is drawn on screen and written to SVG, PDF and raster files. |
| `internal/export` | Smart Export engine. |

## 2. Decisions

- **One executable, no bundled browser engine.** The window uses the
  Chromium-based browser that is already installed. This is the main reason
  MNE Lab stays small and light on low-end machines.
- **Go for the core.** A single static binary per platform (no runtime to
  install), fast start, low memory use, and simple cross-compilation for
  Windows on x86, x64 and ARM.
- **Preact for the interface.** About 450 KiB of script (about 135 KiB
  gzip-compressed) and 55 KiB of styles for the whole desktop interface.
- **No central server.** Accounts and data live on the drive and in the
  user's own cloud. MNE Lab never receives credentials or data.
- **Content-addressed sync.** Record versions are immutable files named by
  their hash and their parents, so no provider needs conditional writes and
  a concurrent edit becomes a visible conflict instead of a silent overwrite.
- **Same drawing on screen and in files.** The plot package produces one
  display list; the preview is exactly what gets exported.
- **Neutral keys everywhere.** The core stores and returns neutral keys
  (`param.effective_diameter`, `axis.intensity`, `point:days:7`), and both
  the interface and the export engine translate them with the same catalogs.

## 3. Implementation status

### Implemented

- Accounts (create, sign in from the drive, sign in from the cloud, change
  passphrase, recovery key), up to five profiles per account, optional
  profile password, avatars (PNG, JPEG, WebP, GIF with animation kept; at
  most 5 MB, at least 16×16 px, at most 40 million pixels; location and
  camera data removed), profile color, username (1–32 characters).
- Storage Modes: USB Drive + Cloud, USB Drive Only, Cloud Only.
- Run modes: Portable USB Mode and Temporary Machine Mode, with detection,
  cleanup and recovery.
- Sync engine, queue, conflicts, offline work; folder transport for the
  providers' desktop apps; API transports for Google Drive and OneDrive
  (OAuth 2.0 with PKCE) and for iCloud (CloudKit Web Services).
- Backups (automatic, manual, on exit), verification and restore.
- Save, Clean & Exit.
- Confirmed restart handoff over bounded, versioned stdin/stdout frames.
  The old HTTP session and window remain available until the replacement
  restores the existing profile and confirms server/window startup. Failed
  children are stopped and reaped before the original store and instance
  lock resume; Turbo preferences and updater activation are rolled back.
  Keys travel only through memory/private pipes. Replacement windows use
  separate app-owned browser profiles, and incompatible store schemas are
  rejected before any migration.
- Automatic Update with signed manifests, staged downloads and activation at
  a safe point; LockedBuild; Stable build selector; rollback by the launcher.
- Mobile access: QR pairing, viewer (status, graphs, cycles) and presentation
  controller.
- Interface: homepage, My Stuff, Change Account, Turbo, command palette,
  Help with every feature, guides and keyboard shortcuts, What's New,
  onboarding tour and first-use explanations, "made by oaovito" footer.
- Localization: English, Portuguese (Brazil) and Spanish.
- LIGHTSCATTERING: import, parser, file library, measurement review, size
  distribution graphs, comparisons, graphs over time, Cycles with statistics,
  Presentation Mode, Smart Export, research package.

### Partially implemented

- **Cloud API transports** are implemented against the providers' documented
  APIs and tested against local test servers, but no official app
  registration exists yet (Google and Microsoft client IDs, CloudKit
  container and token). Until they are injected at build time, only the
  folder transport is offered. iCloud has not been validated against the
  real service.
- **Scientific reference.** The owner's latest LIGHTSCATTERING/Zeta PDF
  and its previous revision have been recovered and consulted privately.
  Their checksums and review status are recorded in the scientific reference
  registry; the PDFs are not shipped with the product. The specification
  remains provisional (`ls-spec/0.3-provisional`) until instrument layouts,
  algorithm selection, units and scientific explanations are validated.
- **Real instrument files.** All test files are synthetic and are labeled as
  such (`testdata/lightscattering/README.md`). No real NanoBrook export has
  been scientifically validated against the owner's reference document yet.
  Experimental development data must stay outside the public checkout and
  release artifacts; public regression tests use synthetic data only.
- **Signed releases.** The update chain is complete and tested with keys
  generated in tests. The official public key is set at release time; no
  release has been published yet.

### Pending

- Official provider registrations and their release-time injection.
- Re-check every parser rule and help text against the recovered scientific
  references and instrument exports before removing "provisional".
- Validation with real NanoBrook 90Plus exports.
- First signed Stable release (signing key kept offline or in CI secrets).
- Windows code signing (Authenticode). The executables already carry the
  MNE Lab icon, version information and a manifest (per-monitor DPI, long
  paths), generated by `scripts/build.sh`.
- macOS builds need cgo (tray) and are produced on a macOS runner.

## 4. Storage

Portable USB Mode, everything relative to the install folder:

```
MNE Lab.exe            launcher
MNE Lab.portable       marker
app/<version>/         builds (Software Distribution)
data/                  accounts and profiles (User Storage)
data/logs/ data/tmp/ data/window/ data/recovery/
Exports/               default export destination
```

Temporary Machine Mode: a session folder `MNE Lab session-…` in the system
temporary directory, removed on exit. Encrypted recovery packages for work
not yet confirmed by the cloud go to a stable per-user location
(`%LOCALAPPDATA%\MNE Lab\Recovery` on Windows) so the next run can finish
the sync. Exports go to `Downloads/MNE Lab Exports`, outside the session, so
cleanup never deletes them.

A folder transport confirms only the copy in the provider's desktop-client
folder. It cannot prove cloud upload. Such a copy therefore does not authorize
deleting Cloud Only stores or encrypted recovery. Only a successful API sync,
with authenticated record/blob readback and exact account-file verification,
can establish remote confirmation.

Each profile store is a bbolt file. Every value is sealed with the profile
data key and bound to its record key as associated data; the file reveals
only random identifiers and sizes. Critical files are written with
`atomicfile`.

## 5. Authentication and keys

```
Account Key (AK, random)          seals the account index and avatars
  wrapped by Argon2id(passphrase)  → offline unlock anywhere
  sealed to the Recovery Key (HPKE) → passphrase reset
  optionally wrapped by a drive key → "keep this drive signed in" (portable only)
Profile Data Key (PDK, random)    seals all profile data
  wrapped by Argon2id(profile password) when the profile has one,
  otherwise wrapped by AK
  always sealed to the Recovery Key  → profile password reset
```

Argon2id parameters: 64 MiB, 2 passes, 2 lanes (sized for inexpensive
computers). A profile with a password cannot be opened with the Account Key:
isolation lives in the key material, not in the interface. The recovery key
is shown once at account creation and never stored in readable form.

## 6. Privacy

- No telemetry, analytics or crash reporting.
- Network calls: the user's own cloud provider (only when a cloud mode is
  used), the update manifest and update downloads (version, platform,
  architecture and channel only), and the phone on the local network when
  mobile access is on.
- Exports are written only where the user chooses and are not encrypted, so
  others can open them.
- Logs are local, bounded in size and redact secrets.

## 7. Cloud providers

Order is fixed: Google Drive / Google One, iCloud, Microsoft OneDrive.

- `api` transport: the provider's web API with OAuth 2.0 + PKCE (Google,
  Microsoft) or CloudKit Web Services sign-in (Apple). Only app-specific
  storage is requested. Credentials are sealed with a key derived from the
  profile key.
- `folder` transport: the folder of the provider's desktop app. MNE Lab
  writes only inside its own subfolder; the desktop app uploads the files.

Registration values (`GoogleClientID`, `GoogleClientSecret`,
`MicrosoftClientID`, `CloudKitContainer`, `CloudKitAPIToken`) are injected
with `-ldflags -X github.com/oaovito/mne_lab/internal/provider.<Name>=…` by
the release pipeline and are never committed. Development builds may read
`data/providers.json`, which is ignored by git.

## 8. Sync

Remote layout inside the provider's app area:

```
accounts/<aid>/profiles/<pid>/r/<rk>~<hash>~<parents>.rec   record versions
accounts/<aid>/profiles/<pid>/b/<blob>.blob                 immutable blobs
```

Uploads are idempotent and can be repeated after an interruption. Two
machines editing the same record produce two heads, shown as a conflict with
both versions kept. Last-write-wins is never used for record data. The queue
is persistent; the interface shows saved, syncing, pending, offline,
reconnect and conflict states.

## 9. Backup and recovery

- Snapshots of the encrypted store (values stay sealed), written atomically,
  registered in a checksummed manifest and re-opened to verify before they
  count. Eight are kept.
- Automatic backups run periodically and on exit (Portable USB Mode).
- Crash recovery: an unclean exit is detected at the next start and the
  interface offers what was recovered.
- Temporary Machine Mode keeps unsynchronized work encrypted in the recovery
  location until a later run confirms it in the cloud.
- Exit preserves encrypted work from accounts closed or switched earlier in
  the session. Recovery replacement retains previous generations, and restore
  verifies a staged copy before making it active. A recovery source is removed
  only when that exact, unchanged source was restored and all its profiles
  passed remote API verification; other accounts and generations remain intact.

## 10. Updater

- Releases are described by `mnelab-releases.json`, signed with Ed25519
  (`mnelab-releases.json.sig`). The application embeds the public keys
  (`internal/update.PublicKeys`, set at release time; several keys allow
  rotation) and refuses anything whose signature or SHA-256 does not match.
- Downloads are staged next to the current build, verified, and activated at
  a safe point (no import, export or sync in progress). The launcher returns
  to the previous build if the new one repeatedly fails to start.
- Data schema: each release states its schema and the minimum it can open,
  so a build that cannot read the data is never offered.
- The updater never touches User Data.

## 11. LockedBuild

Available in Portable USB Mode. When on, automatic updates stop and the drive
stays on the chosen Stable build. The Stable build selector lists published
Stable builds compatible with the data schema, including older ones, and
switches only when the user asks.

## 12. Temporary Machine Mode

Starts when the executable runs outside a Portable USB install. Everything
lives in the session folder; Save, Clean & Exit saves, syncs, verifies, ends
mobile access and removes the session. If the cloud could not confirm the
work, it stays encrypted in the recovery location and the exit says so.

## 13. Mobile

A phone on the same network scans a QR code and opens MNE Lab in its
browser. The QR carries a single-use pairing secret in the URL fragment,
which browsers never send. After pairing, every request and event is
encrypted and authenticated (XChaCha20-Poly1305) with a per-device session
key, numbered against replay and limited to viewing and presentation
control. Stopping mobile access, closing the profile or exiting ends every
phone at once, and phones are told the session ended. The phone interface is
plain HTTP on the local network with this application-layer encryption,
because a local HTTPS certificate would not be trusted by phones. On Windows,
the first start of mobile access may show the system firewall prompt.

## 14. Localization

- Languages: English (`en`), Portuguese (Brazil) (`pt-BR`), Spanish (`es`,
  formal "usted"). The app follows the device language until the person
  picks one; the choice belongs to the profile.
- Interface catalogs: `web/src/i18n/*.ts` (`core`, `ls`, `export`, `help`,
  `errors`), each entry `[en, pt-BR, es]`. The desktop loads all of them; the
  phone loads only what it shows.
- Core catalogs: `internal/i18n/locales/*.json`, shared with the interface
  for scientific labels, figures, exported tables, tray and notifications.
- Plurals: `{n:one/other}`. Placeholders: `{name}`.
- Names not translated: MNE Lab, LockedBuild, Turbo, LIGHTSCATTERING, Stable.
- `web/tests/i18n.test.mjs` fails the build when a text is missing in any
  language, when placeholders differ, when an error code has no message, or
  when the phone shows a text it does not load.

## 15. Design

Dark and light themes with tokens in `web/src/styles/tokens.css`; Inter and
JetBrains Mono embedded (no network fonts). Layouts are checked at
1366×768 and 1024×600 and on a 390×844 phone. Below 1180 px the graph
editor's inspector becomes a drawer opened by the Details button, so the
graph keeps the full width. Turbo turns off cosmetic animations and smooth
scrolling for older computers.

## 16. Feature discovery and onboarding

- Every feature is registered once (`web/src/lib/features.ts`) and appears in
  Help, in the command palette (Ctrl K) and, when new, in What's New.
- A short tour runs on first use; the first use of LIGHTSCATTERING, Turbo,
  mobile access, Save, Clean & Exit and LockedBuild shows a one-time
  explanation. Both can be shown again from Settings.
- Onboarding state is stored only in the person's own profile.

## 17. Scientific module: LIGHTSCATTERING

### Parser

`internal/science/lightscattering` reads NanoBrook 90Plus text, CSV and TSV
exports in UTF-8, UTF-16 and legacy Windows encodings, with tab, semicolon,
comma or space-aligned columns, and dot or comma decimals. It recognizes only
the fields named by the specification (Effective Diameter, Polydispersity,
Current Count Rate, Average Count Rate, BaseLine Index, Diameter / Particle Size, Intensity, and
Volume and Number when present). Every other labeled value is kept untouched
as an unrecognized field. A value absent from the file stays absent.
Ambiguous dates (day/month order) are never guessed; they are flagged for
confirmation. Several measurements in one file are split into blocks.

Quoted CSV, TSV and semicolon records preserve embedded separators and physical
source line numbers. Invalid clock readings are rejected. A damaged recognized
distribution or multiple independent tables in one measurement produces a
warning instead of silently selecting the longest numeric segment.

XLSX workbooks are read with the existing native Go spreadsheet dependency;
no Excel, LibreOffice or conversion subprocess is used. Original bytes and
checksums remain unchanged. Measurements retain source sheet names and row
numbers and explicit selected ranges. The authenticated import wizard inspects
files before storage; changing a worksheet/range selection requires a new
preview, bound to the original hash and account/profile. See IMPORT_REVIEW.md.
Cell values are read without locale-specific number formatting.
Formulas (including cached results), macros and unsupported XLS/ODS containers
are explicitly rejected. Compressed input, expanded workbook bytes, rows and
cells have bounded limits; large worksheet XML stays in memory rather than
being copied to a shared temporary file.

Native Brookhaven reports can mix wrapped `d(nm) G(d) C(d)` tables and
comma-separated spreadsheet sections. Each Lognormal/Multimodal method and
representation is retained separately. No method is selected automatically:
the measurement review shows source values and requires confirmation before
plotting. Wrapped column groups retain a source line for every flattened point.
G(d) is kept as relative intensity with no assumed percentage unit or
normalization; C(d) remains an unmapped auxiliary field, including absent cells.
Unlabeled spreadsheet sections require the matching instrument column header.
An invalid candidate is reported in full rather than reduced to a fragment.
Saved graph definitions pin the selected candidate; changing the measurement's
current choice does not silently change an existing graph. Source provenance
includes the method, representation and workbook sheet when present.

### Provenance

The current scientific rule revision is `ls-spec/0.3-provisional`, recording
reference incorporation, native method/layout review, separate current and
average count rates, and explicit timezone compatibility for cycle association.
Declared units and original precision remain unchanged; combining differing
units in graph/export statistics is rejected. See [SCIENTIFIC_RULES.md](SCIENTIFIC_RULES.md)
for the remaining scientific limitations. Existing imports keep
the specification identifier they were originally parsed with; they are not
relabeled as scientifically revalidated.

Every measurement keeps the original file bytes, their SHA-256, the parser
version and the specification version. Graph results list their sources and
the rules applied; exports carry a manifest with checksums. Graph and export
source timestamps preserve raw date text, known-zone, ambiguity and confirmation
flags. A wall clock without a stated zone is serialized without an offset;
known offsets and fractional seconds are retained. The snapshot is independent
of later measurement edits. XLSX provenance also includes the original sheet,
distribution method and representation.

### Graph Engine

`internal/science/graph` turns a reproducible graph definition plus
normalized measurements into series, axes, statistics and provenance:

- size distribution (intensity, volume or number weighting, never converted
  into each other; logarithmic diameter axis when the data allows);
- comparison of several measurements;
- parameter over time for a cycle (mean ± sample standard deviation);
- distribution over the points of a cycle (replicates of one point share a
  color and differ by line pattern).

A measurement without the needed distribution or weighting is left out with
a note naming it; the graph fails only when nothing remains.

### Cycles

A cycle is a plan (start, interval, duration, unit) and the measurements
associated with its points. Association is automatic only when unambiguous
(closest planned point by date); otherwise it asks for confirmation.
Statistics per point: n, mean, sample standard deviation (n − 1) with n ≥ 2.

### Export

Formats: PNG, SVG, PDF, TIFF, JPEG, WebP, CSV, Excel (XLSX), TSV, text,
JSON, ZIP and the original file. Presets: Quick (PNG 1600×1000 px at
144 DPI), Presentation (1920×1080), Publication (140×95 mm, 600 DPI, SVG and
TIFF, no title), Raw data, Custom. Names are suggested from sample, subject
and date (spaces become `_`). A typed name is kept as typed: only characters
Windows refuses (`<>:"/\|?*`, control characters, a trailing dot or space,
reserved names) are replaced, and the field says when that happened.
Nothing is overwritten silently. CSV with a decimal comma uses a
semicolon as column separator, and cells that a spreadsheet would run as a
formula are neutralized. The research package for a cycle is one ZIP with a
README, a manifest with checksums, the original files, normalized data,
statistics, graphs (SVG, PDF, PNG) and provenance.

### Statistical analyses

LIGHTSCATTERING includes profile-scoped, reviewed ANOVA definitions and
immutable source/result snapshots. Bundled webR/R runs offline in a disposable
worker: One-Way, Welch, complete Two-Way interaction and balanced complete
repeated measures, with Tukey/simple effects and Mauchly/GG/HF. The shared
graph engine renders original observations, explicit SD/SEM/CI and selected
adjusted comparisons. Smart Export and cycle research packages retain
statistical tables, diagnostics, snapshots and graph annotations.

See [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md) for numerical oracles,
source/experimental-unit review, supported designs and limits. Mixed effects,
effect-size confidence intervals, global export signatures and clean
Windows installation validation remain pending. Runtime acquisition is local
to the build; users need no R/Python/Excel. ZETA/NTA variables are rejected
until their own scientific definitions and exports are validated.

### Scientific capability status

Validation means adequate field/method documentation, reproducible numerical
results and comparison with an independent reliable reference. Opening a
file or drawing a graph does not establish scientific validity.

| FEATURE | IMPLEMENTED | TESTED | SCIENTIFICALLY VALIDATED | BLOCKER |
|---|---|---|---|---|
| Statistical Analysis: One-Way / Welch / complete Two-Way / balanced complete repeated | Yes | Backend, independent R vs SciPy/statsmodels/Pingouin, offline browser | Yes, within these documented designs and tolerances | No claim for unsupported methods/designs |
| Tukey / conditional simple effects / Mauchly / GG / HF | Yes | Independent numeric fixtures, explicit pair identities | Yes, within documented scope | Broader instrument/design fixtures remain useful; R quantile precision is documented |
| Dunnett: two-sided single-step, classical independent One-Way, explicit control | Yes | Independent refined SciPy multivariate-t p/CI fixtures; seeded repeatability; scoped persistence and offline worker | Yes, documented designs and numerical tolerances only | Other designs, one-sided tests and more than eight treatments remain unsupported |
| Mixed effects / effect-size confidence intervals | No | Private ML random-intercept preparation only | No | Libraries and corresponding package sources acquired; validate marginal F/denominator df, converge/singular cases, then integrate product; effect-size CIs separately pending |
| LIGHTSCATTERING / NanoBrook DLS | Yes, continuing | Synthetic and private export corpus | PARTIAL; parser rules remain provisional | Finish instrument field/method documentation cross-check and quantitative reference comparisons |
| ZETA | Implementation allowed; scientific parser pending | Private workbook structure reviewed | PENDING / UNVALIDATED | Official potential/mobility/quality documentation; raw instrument exports with matching reference results; units, timestamps, settings and methodology provenance; independent parser/numerical comparisons |
| NTA | Architecture/preparation allowed | No real NTA result fixture | PENDING / UNVALIDATED | Official NanoSight export/software schema and settings documentation; real tracking/summary/distribution exports with corresponding trusted results; independent field/unit/numerical comparisons |
| Malvern DTS | PARTIAL: bounded container/record metadata, reviewed import, File Library, original/metadata export | Independent stream inventory/hash comparison, malformed cases/fuzzing, scoped import/export | UNVALIDATED scientific semantics; no normalized quantities | Identify version-aware field serialization and authoritative field meanings; reproduce summaries/distributions against official exports; ZETA requires its own matching real evidence |
| Literal CSV/TSV/XLSX review | PARTIAL: bounded read-only cells/physical positions; no manual mapping yet | Quoted/multiline/ragged cells, truncation, workbook selections, persistence rejection | Not a scientific calculation | Versioned manual mapping with explicit field/unit choices and provenance remains pending |
| Shared storage / File Library / Graph / Cycle / Statistics / Export | Yes for existing supported data | Isolation, receipts, snapshots, native rendering and exports | Per scientific adapter; never inherited by a new module | New adapters must supply documented quantity and provenance contracts |
| Sign Export / Verify Signature | No | No | Not applicable | Implement global signing/verification, canonical manifest, certificate/key handling and integrity tests |

The existing Zeta spreadsheet and DTS sample are private development corpora.
No real fixture, identifier, original binary or screenshot belongs in a public
repository or shipped product. Missing docs are explicitly unavailable:
MAN0485/MAN0486 and NanoSight MAN0514/Pro manuals are currently catalog links,
not acquired complete manuals. ISO full standards listed in the catalog are
not supplied. No purchase, paid API or official-software dependency is assumed.
The supplied “Zetasizer Nano Analysis Methods Explained” technical note is a
DLS/size reference, not potential-Zeta documentation. It distinguishes
cumulants from inverse-distribution results and does not identify unknown
DTS field codes. Custom workbook mV columns do not prove a native instrument
schema, acquisition settings, independence or a distribution. Preserve such
uncertainty rather than inventing a field or an algorithm.

## 18. Scientific references

`internal/science/reference` lists the specifications rules come from:

| Identifier | Status |
|---|---|
| `ls-spec/0.3-provisional` | Product field list; owner references consulted, instrument layout and semantic validation ongoing. |
| `lightscattering-parser/1.4.0` | Parser version recorded in provenance. |
| `graph-engine/1.1.0` | Graph Engine version recorded in provenance. |

## 19. Tests

- Core: `go test ./...` (including authenticated import review and explicit sheet/range selection) covering paths, keys and recovery,
  encrypted store and queue, sync with conflicts and interrupted uploads,
  provider transports against local servers, backups, atomic writes, updater
  (signature, checksum, staging, LockedBuild, rollback), mobile pairing and
  replay protection, parser (encodings, delimiters, decimals, ambiguous dates,
  several measurements), graph engine, cycles, statistics, plot layout and
  every export format.
- Interface: `npm run typecheck` and `npm test` (translation coverage and
  consistency).
- Browser: `scripts/e2e.sh` (or `npm run e2e` in `web/`) starts a seeded
  local MNE Lab (`TestBrowserHarness` in `internal/app/browser_test.go`,
  skipped unless `MNELAB_E2E_DIR` is set) and runs `web/tests/e2e` with
  Chromium: every main screen in English, Portuguese and Spanish, light at
  1366×768 and dark at 1024×600, in Portable USB Mode and Temporary Machine
  Mode, plus the phone flow (pairing, viewer, controller, end of session, a
  used link refused). It fails on script errors, failed requests, raw
  translation keys, cut text, a page that scrolls, an empty graph or a
  command-palette search that misses a feature, and saves screenshots to
  `out/e2e/` for visual review.

## 20. Known issues

- In Chromium-based windows, the first Esc in Presentation Mode leaves full
  screen before it ends the presentation.
- Cloud export writes to the desktop client's folder only (no direct upload
  of exports).
- EPS and Parquet are not offered.
- XLS and ODS import, interactive column mapping, and export signing with
  certificates/PFX/P12 (including PDF/CMS verification) are not implemented.
- The desktop window still needs a supported installed Chromium browser and
  normal operating-system services. Native science/render/export tests can run
  without development runtimes; this does not validate a clean desktop launch.

## 21. CI checks

`.github/workflows/ci.yml` runs on every push and pull request:

- interface: install, type check, tests, build;
- core: gofmt, `go vet`, `go test -race`;
- dependency audit: `govulncheck` and `npm audit` (production dependencies);
- builds for Windows (x64, x86, ARM64) and Linux (x64, ARM64) with
  `-trimpath` through `scripts/build.sh` (Windows builds get the icon,
  version information and manifest, and no console window); macOS on a
  macOS runner;
- browser tests (`scripts/e2e.sh`), with the screenshots kept as a build
  artifact;
- `scripts/audit.sh` on the tree, the history and the builds: secrets,
  private files, build-machine paths and the unwanted-terms list supplied
  through the `AUDIT_TERMS` secret.

## 22. Next step

1. Validate the parser, help and graph rules against the recovered private
   references. Preserve Lognormal and Multimodal results separately and require
   confirmation when a report offers several methods. Remove "provisional"
   only after the scientific validation is complete.
2. Register the official apps with Google, Microsoft and Apple and inject
   them in release builds.
3. Generate the release signing key offline, set the public key, and publish
   the first signed Stable build.


### Exact remaining evidence blockers: ZETA and NTA

| Module / capability | Missing documentation | Missing real files and trusted reference | Required validation |
|---|---|---|---|
| ZETA native result parser | Complete official Zetasizer Zeta-potential/mobility/quality field and unit definitions for the actual export/software version; timestamp, settings and selected calculation model definitions. Catalog URLs MAN0485/MAN0486 are not complete acquired manuals. | Native Zeta summary/quality/settings exports for the same measurements, with source version and official numerical results. The compiled private workbook supplies neither native schema nor complete acquisition provenance. | Reproduce each documented summary field and unit against the paired official result; preserve source precision and missing/unknown fields. |
| ZETA distributions / DTS | Version-specific binary field serialization, or unambiguous paired native exports; documented distribution axis/weighting and record identity. | Native mobility/potential distribution exports paired with the corresponding private DTS records and trusted values/arrays. | Verify record identity, summary/peak distinction, axes/units, arrays and tolerances. No guessed field code or SOP-path classification. |
| NTA summary / distribution parser | Complete NanoSight software/export schema and field/unit/settings definitions for the actual source version; MAN0514/Pro catalog links alone do not supply this. | Real native NTA summary/distribution exports with documented preparation/dilution/settings and matching official results. No such fixture is currently available. | Reproduce the documented summary and bin values/units, including missing data and relevant correction provenance, against the paired trustworthy result. |
| NTA tracking calculations, if implemented | Documented trajectory/timebase/calibration and calculation rules; an authoritative algorithm reference with assumptions. | Real trajectories and the associated native summary/settings plus a trustworthy numeric reference. | Independently reproduce the calculation with stated tolerances and assumptions; summary import alone does not validate tracking algorithms. |

These are evidence blockers, not a prohibition on modular infrastructure.
Each capability can advance independently when its own evidence is sufficient.
Neither module borrows validation from DLS, ANOVA, a working file reader or a
rendered graph. No unit, calculation model, quality threshold or missing value
is supplied by inference. See [DTS_FORMAT.md](DTS_FORMAT.md) for implemented
partial binary interoperability and its separate technical/scientific status.
