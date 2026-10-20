# Development readiness and remaining actions

The current development branch supports encrypted Accounts/Profiles, File
Library, immutable originals, provenance, graphs, Cycles, supported Statistical
Analysis, backups, sync infrastructure and export. File producer-origin declarations are recorded separately from
transport/parser recognition; see [FILE_PRODUCER_ORIGIN.md](FILE_PRODUCER_ORIGIN.md).
Explicit CSV/TSV/XLSX DLS
mapping is PARTIAL / UNVALIDATED. Statistical Analysis is IMPLEMENTED / TESTED /
VALIDATED only within [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md).
DLS remains provisional; ZETA and NTA scientific validation is pending.
The project is not yet a complete signed Stable release.

## Actions requiring the owner

| Action | What to provide or do | What it unblocks |
|---|---|---|
| DLS scientific evidence | Actual NanoBrook90Plus configuration/software build; exact exported-field/unit/acquisition and Lognormal/Multimodal G/C contracts; paired native summary/distribution exports and trustworthy reference results/settings | Five public Brookhaven documents are now consulted, but do not reproduce these exact private export semantics. Scientific validation still requires paired evidence; see DLS_VALIDATION.md and LIGHTSCATTERING_WEB_RESEARCH.md |
| Native LIGHTSCATTERING format fixtures | Genuine selected-measurement CSV/Excel, distribution XLS/CSV, native archive (the family manual documents `.bak`), SOP/layout and matching reference PDF from the actual installation, with export menu/action/version/settings | Version-aware native format/schema comparisons. Existing generic readers and private TXT corpus do not prove all native formats; see LIGHTSCATTERING_FORMAT_MATRIX.md |
| Remaining research network access | Review/save/publish the saved manufacturer/search/archive/institutional network draft in environment settings | Broader independent/historical research: five official PDFs already succeeded, but Kent/Google requests still report tunnel403 and some other endpoints reject requests. A saved draft is not proof of publication or access |
| ZETA scientific evidence | Complete Zeta-potential/mobility/quality field/unit/settings/model documentation for the actual software; native summary and distribution exports paired with trustworthy official results and corresponding DTS records where applicable | Native ZETA semantic parsing and reproducible validation. The existing compiled private workbook and DLS size-analysis PDF are insufficient |
| NTA scientific evidence | Complete NanoSight source-version export/schema/settings documentation and real summary/distribution files, preparation/dilution metadata and matching official values; trajectories/timebase/calibration/algorithm reference if tracking calculations are required | NTA implementation beyond architecture and scientific validation. Catalog links alone are insufficient |
| DTS interoperability evidence | Version-aware binary serialization documentation, or unambiguous paired native exports with record identities, axes, weighting, units and trusted arrays | Documented scientific fields beyond the currently supported container/envelope investigation |
| Official cloud apps | Register owner-controlled Google/Microsoft/Apple apps, consent/redirect routes and the app-specific storage; provision the required configuration securely through environment/release settings | Real-provider integration and tests. Existing local-server/folder tests do not prove API interoperability. Names: GoogleClientID, GoogleClientSecret where required by registration, MicrosoftClientID, CloudKitContainer, CloudKitAPIToken, CloudKitEnv. Never send secrets in chat or commit them |
| Official release identity | Choose the release signing identity and retain the private Ed25519 key offline or in secure CI; provide/inject the public key. Supply OS code-signing/notarization identities only for distribution that requires them | A trusted signed release/update channel after the remaining engineering gates pass. Test keys are not official release keys |
| Target-system validation access | Provide Windows/macOS runners or machines and representative clean-system/USB conditions for final installation and launch tests | Actual target-platform runtime validation. Cross-compilation and headless browser tests do not prove clean desktop installation |
| Distribution licensing decision | Choose the project's distribution license and a compatible method for providing the statistical runtime's complete Corresponding Source | An authorized public distribution after complete source/reconstruction and other release gates. The private Windows test draft is not Stable or a public release |
| Unwanted-terms list for CI | Add the repository secret `AUDIT_TERMS`, one expression per line (the private terms list). Never commit it | CI then checks the tree, every revision and the builds against it instead of only warning; the Windows test draft refuses to publish without it |
| Historical private Git data, if remediation is desired | Decide the permitted remediation scope for the pre-existing remote private-data history separately; current instruction continues to prohibit direct modification of main | Authorized history remediation. Development proceeds on the audited isolated branch; no force-push or main change is authorized here |
| Original continuity material from earlier sessions, if available | Supply the missing original checkpoint/transcripts privately | More complete historical recovery. Preserved Git commits remain available; absent transcripts are not fabricated and this does not block current development |

The scientific reference files should remain private development material.
Validation requires adequate documentation, reproducible extraction/calculation
and comparison with a trustworthy reference, with stated tolerances.

## Development still required; not an owner configuration problem

- Native XLS, scientific ODS import and distribution mapping with their own
  versioned contracts, safe reader behavior and appropriate reference evidence.
- Smart Export signing integration, a PFX/P12 endpoint and UI (the bounded
  worker backend exists) and
  interoperable PDF verification. Native detached CMS signing infrastructure
  and the global read-only detached Verify panel are PARTIAL, independently
  compared with OpenSSL; see CMS_SIGNATURES.md. Obtaining a certificate alone does not
  implement this feature; private-key exclusion from sync/backups/recovery and
  export packages must be tested. Package /2 checksum coverage and the native
  integrity verifier remain separate from authenticated signatures; see
  PACKAGE_INTEGRITY.md. Visible/Complete signing, local credential handling,
  PAdES, revocation and trusted timestamps still require implementation.
- Desktop distribution/bootstrap packaging and verified offline USB behavior
  on supported clean operating systems; browser/runtime acquisition must be
  reproducible and integrity checked.
- Complete reproducible corresponding-source packaging and licensing checks
  for distributed statistical runtime components; pinned source inventories
  and license notices alone do not prove a complete redistributable release.
- Live cloud-provider verification once owner-controlled registrations are
  configured, and a signed Stable artifact/manifest only after these gates.

No paid external API is used. No native persistent goal/scheduler has been found
and tested in the available tools; automatic restart after quota renewal is not
promised. Every published development checkpoint must retain a verified private
handoff and an exact next action. Main and the preserved earlier history remain
unchanged.

The current configuration readback reports an active base containing the saved
source-download domains and startup instructions, with a pending research-network draft (revision 11). Its additional manufacturer,
search, archive and institutional domains are saved but still require review/save/
publication in environment settings.
CRAN R 4.6.0 source download now succeeds: two official HTTPS mirrors returned
identical archives, and VERSION/COPYING and member paths were checked. This
resolves the earlier network blocker; it does not prove complete source
reconstruction. An independently published upstream checksum/signature, exact
release compiler/image inputs, remaining linked sources and the distribution
decision remain pending. Current-instance tests and source access do not prove
startup on another independent cloud instance or clean desktop/USB behavior.
