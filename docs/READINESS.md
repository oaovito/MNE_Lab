# Development readiness and remaining actions

The current development branch supports encrypted Accounts/Profiles, File
Library, immutable originals, provenance, graphs, Cycles, supported Statistical
Analysis, backups, sync infrastructure and export. Explicit CSV/TSV/XLSX DLS
mapping is PARTIAL / UNVALIDATED. Statistical Analysis is IMPLEMENTED / TESTED /
VALIDATED only within [STATISTICAL_ANALYSIS.md](STATISTICAL_ANALYSIS.md).
DLS remains provisional; ZETA and NTA scientific validation is pending.
The project is not yet a complete signed Stable release.

## Actions requiring the owner

| Action | What to provide or do | What it unblocks |
|---|---|---|
| DLS scientific evidence | Actual NanoBrook/software version and complete authoritative summary/unit/acquisition and Lognormal/Multimodal G/C definitions; paired native exports with corresponding trustworthy reference results/settings | Scientific validation of fields/distributions and any supported recalculation; see DLS_VALIDATION.md |
| ZETA scientific evidence | Complete Zeta-potential/mobility/quality field/unit/settings/model documentation for the actual software; native summary and distribution exports paired with trustworthy official results and corresponding DTS records where applicable | Native ZETA semantic parsing and reproducible validation. The existing compiled private workbook and DLS size-analysis PDF are insufficient |
| NTA scientific evidence | Complete NanoSight source-version export/schema/settings documentation and real summary/distribution files, preparation/dilution metadata and matching official values; trajectories/timebase/calibration/algorithm reference if tracking calculations are required | NTA implementation beyond architecture and scientific validation. Catalog links alone are insufficient |
| DTS interoperability evidence | Version-aware binary serialization documentation, or unambiguous paired native exports with record identities, axes, weighting, units and trusted arrays | Documented scientific fields beyond the currently supported container/envelope investigation |
| Official cloud apps | Register owner-controlled Google/Microsoft/Apple apps, consent/redirect routes and the app-specific storage; provision the required configuration securely through environment/release settings | Real-provider integration and tests. Existing local-server/folder tests do not prove API interoperability. Names: GoogleClientID, GoogleClientSecret where required by registration, MicrosoftClientID, CloudKitContainer, CloudKitAPIToken, CloudKitEnv. Never send secrets in chat or commit them |
| Official release identity | Choose the release signing identity and retain the private Ed25519 key offline or in secure CI; provide/inject the public key. Supply OS code-signing/notarization identities only for distribution that requires them | A trusted signed release/update channel after the remaining engineering gates pass. Test keys are not official release keys |
| Target-system validation access | Provide Windows/macOS runners or machines and representative clean-system/USB conditions for final installation and launch tests | Actual target-platform runtime validation. Cross-compilation and headless browser tests do not prove clean desktop installation |
| Source-download environment configuration | Review/publish the saved additive network-domain draft for cran.rstudio.com, cloud.r-project.org and tukaani.org in environment settings | Source reconstruction preparation currently sees HTTP CONNECT 403 for the CRAN endpoints. Draft persistence does not prove runtime access; retry only after the configuration is applied |
| Historical private Git data, if remediation is desired | Decide the permitted remediation scope for the pre-existing remote private-data history separately; current instruction continues to prohibit direct modification of main | Authorized history remediation. Development proceeds on the audited isolated branch; no force-push or main change is authorized here |
| Original Claude continuity material, if available | Supply the missing original checkpoint/transcripts privately | More complete historical recovery. Preserved Git commits remain available; absent transcripts are not fabricated and this does not block current development |

The scientific reference files should remain private development material.
Validation requires adequate documentation, reproducible extraction/calculation
and comparison with a trustworthy reference, with stated tolerances.

## Development still required; not an owner configuration problem

- Native XLS, scientific ODS import and distribution mapping with their own
  versioned contracts, safe reader behavior and appropriate reference evidence.
- Global Sign Export / Verify Signature, certificate/PFX/P12 handling and
  interoperable PDF/CMS verification. Obtaining a certificate alone does not
  implement this feature; private-key exclusion from sync/backups/recovery and
  export packages must be tested. Package /2 checksum coverage and the native
  integrity verifier are partial backend prerequisites, not completed signing
  or a global Verify interface; see PACKAGE_INTEGRITY.md.
- Desktop distribution/bootstrap packaging and verified offline USB behavior
  on supported clean operating systems; browser/runtime acquisition must be
  reproducible and integrity checked.
- Complete reproducible corresponding-source packaging and licensing checks
  for distributed statistical runtime components; pinned source inventories
  and license notices alone do not prove a complete redistributable release.
- Live cloud-provider verification once owner-controlled registrations are
  configured, and a signed Stable artifact/manifest only after these gates.

No paid OpenAI API is used. No native persistent goal/scheduler has been found
and tested in the available tools; automatic restart after quota renewal is not
promised. Every published development checkpoint must retain a verified private
handoff and an exact next action. Main and the preserved Claude history remain
unchanged.

The configuration initially reported an active base version without a pending
draft. A subsequent additive network-domain draft was saved for blocked source
acquisition; it remains pending publication. Existing install/start instructions
were preserved. The current toolchain/build/browser workflow is tested;
restoration on another independent instance remains a separate verification.
