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
| `internal/science/design` | Experimental designs: units, conditions, time points, replicate kinds, pairing, immutable revisions, structure detection and name suggestions that need confirmation. |
| `internal/science/series` | Series catalog: deterministic SeriesIDs for scalar, curve and instrument-reading series, resolved against a design. |
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
- Sync engine, queue, conflicts, offline work. Periodic passes upload,
  download and verify only what changed; exits, profile switches and Sync
  Now also download and authenticate every copy the cloud should hold, and
  an unchanged account index is not uploaded again; folder transport for the
  providers' desktop apps; API transports for Google Drive and OneDrive
  (OAuth 2.0 with PKCE) and for iCloud (CloudKit Web Services).
- Backups (automatic, manual, on exit), verification and restore. In a
  cloud profile a restore marks every restored record as a pending local
  version: synchronization keeps those the cloud holds unchanged, turns any
  the cloud changed since into conflicts for the person to resolve, and
  brings back records created after the backup. Removed profiles are kept
  as tombstones in the synchronized account index.
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
  Builds that change the data schema are listed but neither staged nor
  installable, because the restart handoff accepts only the same schema. A
  build whose restart failed is recorded as refused and automatic updates
  skip it, so no backup or restart repeats at every check; choosing it
  explicitly clears the refusal.
- Mobile access: QR pairing, viewer (status, graphs, cycles) and presentation
  controller.
- Interface: homepage, My Stuff, Change Account, Turbo, command palette,
  Help with every feature, guides and keyboard shortcuts, What's New,
  guided tutorial of every screen (shown once, reopenable by chapter from
  Help) and first-use explanations, "made by oaovito" footer.
- Localization: English, Portuguese (Brazil) and Spanish.
- LIGHTSCATTERING: import, parser, file library, measurement review, size
  distribution graphs, comparisons, graphs over time, condition × time and
  paired design graphs, groups and saved selections, Combine with lineage,
  Graph → Analyze, statistical annotations from saved analyses, Cycles with
  statistics, Presentation Mode, Smart Export, research package.

### Partially implemented

- **Cloud API transports** are implemented against the providers' documented
  APIs and tested against local test servers, but no official app
  registration exists yet (Google and Microsoft client IDs, CloudKit
  container and token). Until they are injected at build time, only the
  folder transport is offered. iCloud has not been validated against the
  real service; a local CloudKit test server covers sign-in, token rotation,
  multi-part files, listing and deletion. CloudKit's sign-in carries no
  caller-chosen `state` (Apple documents only `ckWebAuthToken` on the
  callback), so the loopback callback cannot prove which sign-in it answers.
  It accepts only the first GET of its registered path while a sign-in
  started in MNE Lab is open; another page that sends a token in that window
  could connect a different Apple Account. Files stay encrypted with the
  profile keys either way. This limit is accepted until Apple offers a
  binding value.
- **Scientific reference.** The owner's latest LIGHTSCATTERING/Zeta PDF
  and its previous revision have been recovered and consulted privately.
  Their checksums and review status are recorded in the scientific reference
  registry; the PDFs are not shipped with the product. The specification
  remains provisional (`ls-spec/0.3-provisional`) until instrument layouts,
  algorithm selection, units and scientific explanations are validated.
- **Real instrument files.** Public repository fixtures are synthetic and labeled as
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

Temporary Machine Mode: a session folder `MNE Lab session-…`, removed on
exit, and encrypted recovery packages for work not yet confirmed by the cloud,
both in a per-user state folder that survives a reboot and is not a cache:
`%LOCALAPPDATA%\MNE Lab` on Windows, `~/Library/Application Support/MNE Lab`
on macOS and `$XDG_STATE_HOME/MNE Lab` (default `~/.local/state/MNE Lab`)
elsewhere, under `Sessions` and `Recovery`. The system temporary folder can
be memory cleared at restart, and cache folders may be emptied, so neither
would let the next run find interrupted work. Sessions and packages that
earlier builds left in the temporary or cache folder are still found. Exports go to `Downloads/MNE Lab Exports`, outside the session, so
cleanup never deletes them.

A folder transport confirms only the copy in the provider's desktop-client
folder. It cannot prove cloud upload. Such a copy therefore does not authorize
deleting Cloud Only stores or encrypted recovery. Only a successful API sync,
with authenticated record/blob readback and exact account-file verification,
can establish remote confirmation.

Each profile store is a bbolt file. Every value is sealed with the profile
data key and bound to its record key as associated data; the file reveals
only random identifiers and sizes. Critical files are written with
`atomicfile`, which keeps the previous version to recover from damage. Key
files (account header, profile key files, drive key) keep no previous version
and are never read from one: a superseded wrapping would still open with an
old passphrase or, for a profile given a password since, with the Account Key.

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

Each passphrase change increases the header's generation, which AK seals
together with the new wrapping and which travels inside the sealed index.
A machine that still holds an earlier header adopts the later one from the
index when it unlocks or synchronizes, before it publishes a header, so the
earlier passphrase stops opening the account everywhere. Two changes made
from the same generation on different machines are ordered by the sealed
time of the change, so every machine settles on the same passphrase. A
header fetched at Sign In replaces the local one only after the Account Key
it opens vouches for it; until then it is kept apart as a candidate. The
recovery wrapping, which never changes, is also kept in its own file, so the
Recovery Key still opens an account whose header is damaged. The index also
records when each profile's key file last changed; a superseded key file
from the cloud is never installed, even when the local one is missing, a
key file without a password never replaces one the index says has a
password, and a local key file the index no longer records is not uploaded.

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
  count. Eight are kept; the snapshot being restored is pinned, so no
  backup made during the restore can rotate it out.
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
JetBrains Mono embedded (no network fonts). The brand palette, set by the
owner on 2026-10-15, is #F4F8FA (base), #2C7FB8 (primary accent) and #143A52
(deep accent). Light is the default theme and uses the three colors exactly:
base background, accent for selection, focus, icons, toggles and bars, deep
accent for text, tooltips and the start-screen panel. The dark theme is
derived from #143A52. Small accent text (`--accent-text`) and filled controls
with white labels (`--accent-fill`) use slightly deeper shades of the accent,
because white on #2C7FB8 is 4.3:1, below WCAG AA for normal text. Surfaces
are flat in both themes: no decorative gradients, glows or neon; the dark
theme uses muted status colours (decision D10 of 2026-10-21, owner addenda
A §49-51 and B §54-57). Scientific figures keep white paper and their own
series palette (`internal/plot/style.go`): #2C7FB8, restrained rust, muted
teal, #143A52, ochre, graphite, plum, wine, pale steel and slate indigo, in
an order where the first six differ by at least 7 L* in grayscale and all
ten stay apart under simulated protanopia, deuteranopia and tritanopia;
past ten series, restrained colours are generated in OKLCH (golden-angle
hues, no green band). Series are never told apart by colour alone: curves
get distinct line patterns (solid, dashed, dotted, dash-dot, long dash,
dash-dot-dot) and, when two visible curves would share a pattern, a few
identity markers on real data points; means and replicates of temporal
graphs get distinct marker shapes (circle, square, triangle, diamond,
inverted triangle, cross) and patterns. Saved per-series `marker` and
`lineStyle` override these defaults. The legend lists the visible series
(default), all series (hidden ones muted) or only `visual.legendSeries`
(`visual.legendMode` `selected`); what does not fit, or is not listed, is
counted as "+N more", so many series never take the plot area. The
icon, set by the owner on 2026-10-15, is a hammerhead shark seen from above
in #143A52 on a light #F4F8FA tile, inside a fine goniometer-style graticule
(the angle scale of light-scattering instruments) with one #2C7FB8 accent;
the 16–32 px version drops the graticule, and the monochrome mark used for
generic profile avatars is the shark alone. The
raster icons (PNG/ICO and the copies in `internal/brand`) are regenerated
from the SVG masters with `node ../scripts/brand_icons.mjs` from `web/`,
after `npm ci` and `npx playwright-core install chromium` (or with
`MNELAB_CHROMIUM` pointing to a Chromium build).
Layouts are checked at 1366×768 and 1024×600 and on a 390×844 phone. Below
1180 px the Graph Builder's settings panel becomes a drawer opened by the
Settings button in the page header, so the preview keeps the width; the
Series Explorer and the action bar stay in place. Turbo turns off cosmetic
animations and smooth scrolling for older computers.

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

- values per series (`series_values`): one summary quantity or instrument
  reading per selected series on a categorical axis, refused when the
  selection mixes parameters or units (several units only with an exact
  `displayUnit`, which Combine offers).
- condition × time (`design_time`): a saved design revision drawn by
  experimental unit. Each condition is one mean series (mean ± SD by
  default, SEM on request, CI only from a linked saved analysis) with its own
  colour, marker and line pattern on a shared integer time axis (labels from
  the design), each unit's value as an individual point (shown unless
  `showIndividual: false`) and optional unit trajectories
  (`unitTrajectories`, tinted thin lines out of the legend). Technical
  readings of a unit in a cell are averaged first; n counts units.
- paired view (`paired_condition`): at one time point (`pairedAt`, default
  the first), each unit's values under the conditions joined by a line, the
  mean of each condition beside it (display-only offset), and a note when a
  unit has both conditions but no declared pair.

Graph definitions are stored with `schema: 2` (engine `graph-engine/1.3.0`).
Schema 2 only adds fields; a schema 1 definition renders exactly as before
(`internal/plot/compat_test.go` compares against a golden file):

- `seriesIds`: the selection by SeriesID. The core derives `measurements`,
  `weighting`/`param` and the pinned distribution candidates from it on
  save, so older readers still see a valid definition. A SeriesID that no
  longer resolves (file trashed, candidate removed) is left out with the
  warning `graph.missing_series`, never a failure.
- display options: `focusSeries` (others drawn muted, out of the legend),
  `subtitle`, `xLabel`/`yLabel`, `yScale` (log only with positive values),
  `displayUnit` (shown unit with an exact factor only: nm/µm/m,
  cps/kcps/Mcps, s/min/h/d; the stored values do not change and the axis
  records `conversion {from, to, factor}`), `showIndividual`,
  `connectMeans`, `textNotes`, `visual.margin` and `visual.preset`.
- error bars are always named on the figure (legend, metadata or a note):
  SD by default, SEM on request; a confidence interval only for a
  statistical graph from a saved analysis.
- `derivedFrom` (lineage, written only by the core), `sourceFiles`,
  `parserVersions` and `calculationVersion` (snapshots taken on save).
- design graphs: `designId` (a design revision, pinned; the editor offers a
  newer head but never moves to it alone), `conditions` (subset, in design
  order), `pairedAt`, `groups` (saved groups restricting the members) and
  `unitTrajectories`. On save the core derives `measurements` from the
  design's assignments. `analysisId` must name a saved analysis of the same
  design root and variable (`graph.analysis_incompatible`); without one a
  CI request falls back to SD and annotations are dropped.
- annotations of design graphs: up to 12 comparison IDs of the linked
  analysis, styles `exact` (adjusted p), `stars` (from the adjusted p) or
  `effect` (stored difference, its interval and the adjusted p). Brackets,
  labels and values are read from the saved comparisons and never
  recalculated; a paired view draws only comparisons between conditions at
  its time point; a graph whose conditions are stored in different units
  draws none (`graph.annotations_mixed_units`). Bracket labels are kept
  inside the plot area.

`POST /api/graphs/{id}/duplicate` (optional `title`) stores a copy with a
new ID and `derivedFrom: [{graphId, relation: "duplicate", title,
definitionHash, at}]`, where the hash is the SHA-256 of the stored original;
the original record is not touched and a graph in the trash is refused.
Combine (`POST /api/graphs/combine/check` and `POST /api/graphs/combine`,
`{ids, title, displayUnit}`, account and profile headers) reads 2–8 saved
graphs of one combinable kind (`design_time`, `series_values`,
`dls_distribution`) and the same variable, and refuses different variables,
designs or revisions, X domains (variable or unit), units without an exact
conversion, empty graphs and graphs in the trash, each with a stable code
(`graph.combine_*`). When units differ it lists the units every parent
converts to with an exact factor; only an accepted one is applied as
`displayUnit`. The new definition is built from the parents' definitions
(the union of their conditions or series, never images), with lineage
`derivedFrom: [{graphId, relation: "combine", title, definitionHash, at}]`
for every parent. The parents are re-read inside the write transaction and
the combination is refused with `graph.combine_conflict` when one changed
after the check; parents are never written. Linked analyses and annotations
are not carried over. A parent's later edits never propagate: relations
report `changedParents` when a parent's current hash differs from the one
recorded.

Relations list `derivedFrom`/`derived` for each graph, plus `design`,
`groups`, `analysis` and `changedParents`, and the files, graphs and cycles
of each group. The render handler
passes the request context down to the series loading and stops with
`graph.render_cancelled` when the request is cancelled; the interface aborts
a preview as soon as a newer one supersedes it and shows a Stop control
when one takes noticeable time.

### Graph Builder

The graph screen (`web/src/screens/ls/graph.tsx`) has the Series Explorer
(`series-explorer.tsx`), Data and Groups on the left, the live preview in
the centre, the settings (type and variables, design revision, conditions
and paired time point, axes and units, error bars and points, statistical
annotations, lines and markers, legend, labels and notes, publication
preset), per-series style and details (provenance with lineage both ways:
Derived From and Used By, changed-parent notes and version snapshots,
comparison, notes) on the right, and Save, Duplicate, Combine
(`combine.tsx`), Analyze, Present and Export below. The figure tooltip shows
each point's own name (time, unit · condition) for design graphs. Analyze
opens `/ls/statistics/new` prefilled from the graph (`design`, `variable`,
`conditions`, `group`, `graph`, or `cycle`); the editor still shows the
design review card and applies the pseudoreplication guard, and after
saving it offers Annotate, which opens the graph with `?analysis=` linked
for the person to choose comparisons and save. The Explorer reads `GET /api/series`, draws only the rows in
view (fixed row height), searches and filters by file, condition, time,
replicate kind (instrument readings included) and unit, selects one, a
Shift range or everything listed, and offers per-series hide and focus.
Selecting series stores only their IDs; series left out and every record
stay unchanged.

### Groups and saved selections

`ls.group` (synced) holds groups (`kind: "group"`, measurement IDs) and saved
selections (`kind: "selection"`, SeriesIDs; their measurements are derived on
save). Both store references only, never values (`/api/groups`, account and
profile headers; at most 2000 members). A group feeds a design graph
(`groups`), a series graph (adds its series to the selection), a cycle
(`groupId` kept on the cycle) and an analysis (`/ls/statistics/new?group=`).
Deleting a group a saved graph draws is refused (`groups.in_use`); its
measurements, cycles and analyses are never touched. The Groups tab lists
them; the File Library saves a selection of measurements as a group and the
Graph Builder saves its selection of series.

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

A design graph exports its data as three tables (readings with their
replicate kind, unit values per condition × time, and the summary with n,
mean, SD, SEM and, from a linked analysis, the CI) plus metadata naming the
design revision, n, error bars, groups, analysis and every Derived From
parent (`internal/export/design.go`; golden files in
`testdata/design-export/`). A single graph can also be exported as a
research package (`package` preset): figures, normalized data, original
files, `graphs/definitions.json` with parents and lineage hashes,
`design/design.json` with units, conditions, times, pairs and revision
history, `groups/<id>.json`, the linked analyses under `statistics/analyses/`
and provenance. A cycle package adds the design and group it was built from.
A group exports its members as tables (kind `group`).

### Experimental designs and series

Designs live in the synced collection `science.design`; every save is a new
immutable record (`root`, `revision`, `previousId`, `changes`), so two
machines that edit the same revision keep both heads and show them as
diverged instead of overwriting one. Instrument readings of a runs table live
in `ls.reading`, one record per file, written at import and backfilled from
the stored original for earlier imports. Neither collection changes a
measurement. `GET /api/series` lists the series of the library (optionally
`design=`, `file=`, `q=`); a SeriesID is `m:<measurement>/p:<parameter>` for a
summary quantity, `m:<measurement>/d:<distribution>/w:<weighting>` for a
distribution curve and `m:<measurement>/r:<run>/p:<parameter>` for an
instrument reading, built only from stable identifiers, never from labels.
Design endpoints (`/api/designs`, `/api/designs/review`,
`/api/designs/suggest`, `PATCH /api/experimental-units/{id}`) require the
account and profile headers, like statistical analyses. See
[SCIENTIFIC_RULES.md](SCIENTIFIC_RULES.md) for the rules.

### Statistical analyses

LIGHTSCATTERING includes profile-scoped, reviewed ANOVA definitions and
immutable source/result snapshots. Bundled webR/R runs offline in a disposable
worker: One-Way, Welch, complete Two-Way interaction and balanced complete
repeated measures, with Tukey/simple effects and Mauchly/GG/HF. The shared
graph engine renders original observations, explicit SD/SEM/CI and selected
adjusted comparisons. Smart Export and cycle research packages retain
statistical tables, diagnostics, snapshots and graph annotations.

See [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md) for numerical oracles,
source/experimental-unit review, supported designs and limits. ML random
intercepts for explicit repeated units add marginal Wald F tests with both
degrees of freedom, model variances and coefficients. Effect-size confidence
intervals for other designs/effect sizes, Smart Export signing integration and
clean Windows installation validation remain pending. Detached CMS signing and
global detached verification are PARTIAL; see CMS_SIGNATURES.md. Runtime acquisition is local
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
| Dunnett: two-sided single-step, classical independent One-Way, explicit control | Yes | Independent refined SciPy multivariate-t p/CI fixtures; seeded repeatability; scoped persistence and offline worker | Yes, documented designs and numerical tolerances only | Other designs, one-sided tests and more than three treatments remain unsupported |
| Mixed: ML Gaussian random intercept, categorical repeated B, optional between A | Yes | Five independent Statsmodels/SciPy synthetic designs; exact df; boundary/design rejections; scoped model persistence; local worker | Yes, restricted documented scope and tolerances | Random slopes, alternative residual covariance, REML choice and mixed post-hoc unsupported |
| Mixed individual approximate fixed-coefficient CIs | Yes, nlme intervals.lme in engine /2 | 80 independent Statsmodels conditional GLS / SciPy t endpoints in five designs; exact coefficient df; model persistence/export and offline worker | Yes, documented restricted model and approximate interval convention | Simultaneous mixed post-hoc, variance-component CIs and broader covariance/random structures unsupported; no empirical coverage claim |
| Population eta squared CI: optional fixed-effect independent One-Way | Yes, source subset MBESS 5.0.1 | Seven independent SciPy endpoint fixtures, boundary/non-estimable handling, source integrity, persistence/export and local worker | Yes, available intervals within the documented scope/tolerances | Partial eta/omega and Two-Way/Welch/RM/Mixed effect intervals require separate estimators and reliable oracles |
| LIGHTSCATTERING / NanoBrook DLS | Yes, continuing | Public synthetic regressions; independent literal comparison of 54 private reports, 216 quantities and 216 distribution candidates | PARTIAL; parser rules remain provisional | Instrument/version-specific summary units and acquisition definitions; G/C meaning, scaling and method documentation; paired trustworthy numerical results and acquisition data for any recalculation. See DLS_VALIDATION.md |
| ZETA | Implementation allowed; scientific parser pending | Private workbook structure reviewed | PENDING / UNVALIDATED | Official potential/mobility/quality documentation; raw instrument exports with matching reference results; units, timestamps, settings and methodology provenance; independent parser/numerical comparisons |
| NTA | Architecture/preparation allowed | No real NTA result fixture | PENDING / UNVALIDATED | Official NanoSight export/software schema and settings documentation; real tracking/summary/distribution exports with corresponding trusted results; independent field/unit/numerical comparisons |
| Malvern DTS | PARTIAL: bounded container/record metadata, reviewed import, File Library, original/metadata export | Independent stream inventory/hash comparison, malformed cases/fuzzing, scoped import/export | UNVALIDATED scientific semantics; no normalized quantities | Identify version-aware field serialization and authoritative field meanings; reproduce summaries/distributions against official exports; ZETA requires its own matching real evidence |
| Literal CSV/TSV/XLSX review | PARTIAL: bounded read-only cells/physical positions | Quoted/multiline/ragged cells, truncation, workbook selections, persistence rejection | Not a scientific calculation | Additional workbook formats and supported mappings have independent contracts |
| Explicit CSV/TSV DLS mapping | PARTIAL: versioned user choices for five existing scalar fields, units, record extent and separators; scoped review/confirmation and encrypted saved recipes | Invalid/changed choices, missing/zero/full-source fidelity; graph/cycle/analysis snapshots and export; localized browser workflow | UNVALIDATED field/unit declarations; software tests do not validate measurements or instrument semantics | Distribution mappings, adequate instrument documentation and trustworthy paired reference results; no ZETA/NTA mapping permitted |
| Explicit XLSX DLS mapping | PARTIAL: versioned sheet/physical-row/column declarations, preserved XML precision and user-unit provenance; encrypted recipe schema 3 | Independent invented XML fixture; unsafe excluded sheets, coordinates, repeated/shared-string and encoded payload bounds, missing/zero/full-source, receipts, graph/cycle/analysis/export and localized browser workflow | UNVALIDATED declarations; opening/plotting/software tests do not validate instrument semantics | Adequate instrument documentation and paired trustworthy reference results; merged/linked workbooks, distributions and ZETA/NTA mappings unsupported |
| Literal ODS 1.3 review | PARTIAL: native bounded plain-cell preview; display and declared values separate | Independently generated schema-checked invented fixture; repetitions, positions, unsafe/malformed sources, scoped read-only HTTP and localized browser review | Not a scientific calculation; no quantities or import receipt | Scientific mapping/import, other ODF versions, merged/richer cells and real instrument oracles remain unsupported |
| File producer origin | Yes: unknown or explicit user-declared instrument export/user-created table, separate from transport and parser | Scoped API rejection/isolation/reopen, immutable originals and statistics, graph snapshots, JSON/TXT/XLSX and localized browser | Not applicable; declaration remains UNVERIFIED and never promotes scientific status | Genuine native authenticity/reference evidence still required; see FILE_PRODUCER_ORIGIN.md |
| Shared storage / File Library / Graph / Cycle / Statistics / Export | Yes for existing supported data | Isolation, receipts, snapshots, native rendering and exports | Per scientific adapter; never inherited by a new module | New adapters must supply documented quantity and provenance contracts |
| Export file collisions | Yes: atomic exclusive publication; explicit replacement only | Concurrent keep-both/ask, payload preservation, existing files and symlinks; local Linux and hosted Windows/macOS checks tracked separately | Not applicable | Clean-system/FAT32/exFAT USB runtime tests still required |
| Research package checksum verification | Partial backend; package /2 covers README; /1 reports uncovered README | Independent invented ZIP/JSON fixtures, altered payload/index/CRC, unsafe paths and bounds; current checkpoint records completed checks | Not applicable; Authenticated=false, no signature or scientific validation | Package-integrity UI and authenticated manifest integration remain pending. Detached CMS backend/global Verify already exist separately; PFX/Smart Export/PDF integration is still pending |
| Sign Export / Verify Signature | PARTIAL: native detached CMS signer/verifier; global read-only detached Verify panel | Independent OpenSSL RSA/ECDSA bidirectional comparisons and public invented fixture; trust/time/content/structure tests, authenticated upload and localized browser workflow | Not applicable; no scientific validation, revocation or trusted timestamp claim | Smart Export integration, Profile identity/local credentials, PFX/P12 endpoint/UI (bounded worker backend exists), Visible/Complete ordering, PDF/PAdES, trust selection, revocation/timestamps and signed research packages; see CMS_SIGNATURES.md |
| LIGHTSCATTERING format discovery | PARTIAL: source registry, format/content matrix, external research log and scientific decision records | Five official Brookhaven PDFs retrieved/hash-pinned; PS2.3/4.0 and family/SOP/setup references cross-checked with private corpus and current reader contracts | No validation promotion; observed extraction and documented export capability remain separate | Exact90Plus configuration/build, legacy/historical/independent corroboration, native CSV/Excel/distribution/archive/SOP/layout fixtures and serialization; native authenticity is not established by the separately recorded user declaration; see LIGHTSCATTERING_FORMAT_MATRIX.md |

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
| `lightscattering-parser/1.7.0` | Parser version recorded in provenance. |
| `experimental-design/1` | Experimental design record version. |
| `series-catalog/1` | Series catalog (SeriesID scheme) version. |
| `graph-engine/1.3.0` | Graph Engine version recorded in provenance; graph definitions are stored as schema 2. |

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
  `out/e2e/` for visual review. `builder.mjs` runs the Graph Builder on twenty
  synthetic series (`testdata/lightscattering/synthetic-twenty-series.txt`):
  search, filter, select three, edit, save, reopen, duplicate with lineage
  and stopped previews, checking that the other seventeen series and the
  file's records do not change. `paired.mjs` imports 24 synthetic
  measurements (units A, B, C; F-SR and C-SR; days 0, 7, 14, 21), saves the
  design, draws Graph 1 (F-SR) and Graph 2 (C-SR), combines them into Graph 3
  (refusing a graph of another variable; parents byte-identical; lineage
  both ways), runs the classical paired analysis offline from Analyze,
  annotates Graph 3 from it, exports the research package and reopens the
  graph with its pairing intact, in every language and both themes.

## 20. Known issues

- In Chromium-based windows, the first Esc in Presentation Mode leaves full
  screen before it ends the presentation.
- Cloud export writes to the desktop client's folder only (no direct upload
  of exports).
- EPS and Parquet are not offered.
- XLS and scientific ODS import, distribution column mapping, Smart Export
  signing integration, a PFX/P12 endpoint and UI, and PDF/PAdES
  verification are not implemented; the bounded PFX/P12 worker exists as a
  backend only. Native detached CMS signing and the global
  detached Verify panel are PARTIAL; see CMS_SIGNATURES.md.
- The desktop window still needs a supported installed Chromium browser and
  normal operating-system services. Native science/render/export tests can run
  without development runtimes; this does not validate a clean desktop launch.

## 21. CI checks

`.github/workflows/ci.yml` runs on pushes to every branch and on pull
requests. Its audit
checkout fetches the complete checked-out revision history without unrelated
branches/tags; the audit still inspects all reachable objects/revisions in that
checkout and rejects shallow histories:

- interface: install, type check, build, then tests (the numerical worker tests
  mount the generated, checksum-verified statistical package image);
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
  through the `AUDIT_TERMS` secret. Names are matched literally and after
  Unicode folding; binary blobs, UTF-16 text, ZIP-based files (also behind
  a prefix), tar archives and gzip, bzip2 and xz streams in the index, every
  revision and every annotated tag are opened, and archive formats it cannot
  read fail; tag messages are checked like commit messages; PKCS#12 key
  stores (`.p12`/`.pfx` names and PFX content) count as private; co-author
  and session trailers fail even without terms. The bundled R runtime is
  third-party content whose citations can contain the branding words, so only
  those words are not applied to its files. Without the secret CI shows a warning; the Windows test
  draft requires it (`AUDIT_REQUIRE_TERMS=1`).

## 22. Next step

1. Review [DEVELOPMENT_STATUS.md](DEVELOPMENT_STATUS.md) and
   [READINESS.md](READINESS.md) against the current source and requirements.
   Preserve supported Statistical Analysis and the existing DLS workflow.
2. Implement bounded credential handling before Smart Export signing:
   [CMS_SIGNATURES.md](CMS_SIGNATURES.md). Detached CMS verification is already
   PARTIAL; the bounded PFX/P12 worker exists as a backend. Profile-scoped
   credential choice, final-byte ordering and PDF/PAdES still need their own work.
3. Complete safe XLS/ODS/distribution adapters only with sufficient contracts,
   and continue version-aware format research. Preserve Lognormal/Multimodal
   choices and unknown units; remove provisional status only after adequate
   documentation, reproducibility and trustworthy reference comparisons.
4. Complete corresponding-source reconstruction, distribution licensing,
   bootstrap/offline USB and clean desktop validation. Verify real providers
   after owner-controlled registrations are available.
5. A release key alone does not establish readiness. Publish a signed Stable
   build only after all scientific, privacy, security, source, distribution and
   platform gates pass and the required owner authorization is available.


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

## 23. Readiness and owner intervention

See [READINESS.md](READINESS.md) for the explicit remaining owner actions and
separate development tasks. A tested development checkpoint is not a complete
Stable release.


### Concurrent export publication

Export completes and flushes a hidden partial file before claiming its final
name. “Keep both” retries collisions against the original requested name using
an atomic operation that refuses replacement. “Ask” has exactly one winner
when concurrent exports request the same absent path. Only an explicit
“Replace” policy uses replacement. This corrects a reproduced race in the
previous check-then-rename fallback, which could overwrite a concurrent export.

Linux uses `renameat2(RENAME_NOREPLACE)`; macOS uses
`renameatx_np(RENAME_EXCL)`; Windows uses `MoveFileExW` without
`MOVEFILE_REPLACE_EXISTING`. A hard-link fallback also refuses replacement.
If no supported exclusive operation is available, export fails safely rather
than substituting an ordinary replacing rename. These operations support
filesystems without hard links when the platform/filesystem provides exclusive
rename; actual FAT32/exFAT and clean USB behavior remain separate release gates.
Native Windows export/key tests are a dedicated hosted CI job, distinct from
cross-compilation. This is file publication safety, not Sign Export, certificate
handling or scientific validation.

Research package integrity contract and legacy coverage: [PACKAGE_INTEGRITY.md](PACKAGE_INTEGRITY.md). Checksums are not digital signatures.
