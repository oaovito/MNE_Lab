# Statistical analysis

Status: **IMPLEMENTED / TESTED / VALIDATED within the documented scope**.

Statistical analyses use reviewed, recognized summary quantities from
LIGHTSCATTERING. Distribution bins are not inferential replicates. ZETA and
NTA quantities require their own validated scientific definitions before
being accepted by this shared engine.

## Implemented methods and limits

- One-Way ANOVA: R `lm` / `anova`, classical sums of squares, eta squared,
  partial eta squared and omega squared (negative omega estimates are
  bounded at zero and identified by this convention).
- Welch ANOVA: R `oneway.test`, fractional denominator degrees of freedom.
  Classical SS-based effect sizes are deliberately absent.
- Two-Way ANOVA: full interaction model, sum contrasts and Type III tests
  through `drop1`. Every factorial cell must exist, with estimable residual
  error. Unbalanced complete factorials are supported.
- Repeated measures: explicit physical unit IDs, complete unit × time
  observations, balanced between-subject groups. R `aov` with subject
  error strata. Mauchly and GG/HF are calculated from R's multivariate
  covariance implementation. Requested correction is shown alongside
  the original terms. Singular/non-estimable corrections stop calculation.
  The pinned R internal `sphericity` function is version-sensitive; upgrades
  must pass independent epsilon/df/p comparisons.
- Tukey HSD: independent classical one-factor models. Two-factor models
  expose conditional simple effects within each level of each factor;
  Bonferroni adjustment covers both sets of Tukey comparison families.
- Group means, sample SD, SEM and Student-t confidence intervals retain
  individual values. These intervals are group mean intervals, not effect
  size confidence intervals or simultaneous post-hoc intervals.

Mixed effects and Dunnett are **not implemented**. Their mature additional
WebAssembly libraries are not yet bundled or independently validated for these methods.
Incomplete/unbalanced repeated designs are blocked, not silently fitted
with a simpler model. Welch/RM post-hoc procedures and effect-size confidence
intervals remain pending. No unadjusted pairwise t-test substitutes for them.

## Review and persistence

Cycle → New analysis or LIGHTSCATTERING → Statistical analyses opens the
editor. Cycle associations only supply time levels for already confirmed or
automatic valid assignments. The user reviews factors and explicitly decides
whether observations are independent or repeated; unit identity is never
inferred from names, replicate numbers or dates.

The core builds a profile-scoped source snapshot. Missing quantities remain
missing; exclusion reasons persist; units must match exactly. No automatic
imputation, unit conversion or outlier removal occurs. Residual Shapiro–Wilk
and median-centred Brown–Forsythe tests are auxiliary diagnostics, not
automatic method selectors. Repeated observations use sphericity instead of
an inappropriate independent-groups variance test.

A scoped, process-local HMAC receipt binds preparation to the reviewed
definition, file SHA-256, measurement IDs, selected quantity, source
worksheet/range, parser/spec versions, cycle and organization metadata.
Saving rechecks sources inside the write transaction. Saved groups must
contain exactly the reviewed values in source order. Result origin is
`local_bundled_browser_worker`: this is **not** a server attestation of
numerical correctness. Numerical correctness is established by tests.

`science.analysis` and `science.experimental_unit` are normal encrypted
profile collections and use existing backup, recovery and sync machinery.
Account and Profile headers are required by every statistical API. Changing
source metadata marks historical analyses Source Changed. Recalculation
creates a linked new revision; it never rewrites the old results.

## Offline runtime and lifecycle

webR 0.6.0 / R 4.6.0 is pinned in the npm lockfile. The build embeds its worker,
WASM runtime, BLAS/LAPACK and lazy filesystem into the existing application.
Users need no installed R, Python, Node, Excel or statistical packages.
Package downloads are disabled in the product. Local calculation uses a
fresh PostMessage worker and an ephemeral R filesystem; no IndexedDB store.
The unused upstream public TLS certificate bundle is omitted from the build.
Completion, cancellation, timeout or leaving the profile closes the worker.

R's Emscripten linker needs generated JS function wrappers. Only the
`/statistics-engine/webr-worker.js` response allows `unsafe-eval`; worker
network access is restricted to `self`. The main UI retains its restrictive
CSP. Missing runtime/package paths return 404 rather than the SPA document.
An independent browser test calculates with all external requests blocked.
This is an offline product test on the development machine, not proof of
the complete native desktop workflow on a fresh Windows machine.

## Graphs and exports

The shared Graph Engine renders immutable saved analyses as categorical
individual points plus group means with explicitly selected SD, SEM or CI.
Display-only horizontal jitter separates coincident points without changing
scientific values. Graphs retain the analysis ID and original provenance even
after a source changes. Comparisons retain explicit group identities and stable IDs; labels are never
parsed to infer pairs. Users select up to eight significance brackets and
choose exact adjusted p-values or stars. Brackets, multiplicity corrections
and display thresholds persist in graph definitions and provenance; the
shared screen/SVG/PDF/raster renderer uses the same display list. No bracket
is added automatically. A stored zero p-value is displayed as below engine
precision, without inventing a numerical bound.

Smart Export accepts statistical analyses. XLSX includes named sheets for
ANOVA, group summaries, observations, post-hoc, diagnostics, residuals,
effects and corrections. Delimited formats produce a separate file per table
to avoid dropping secondary results. JSON preserves the full analysis and
source snapshot; exports omit local Account/Profile IDs and receipts. PDF
reports paginate every table row using the existing bundled font/library.
Full analysis packages use eight statistics folders and existing manifest
checksums. Saved statistical graph definitions and annotation selections
are included. A cycle research package includes every saved analysis linked
to that cycle in its own eight-folder subtree. Source dependency warnings
include analyses, whose original snapshots survive source deletion.

Sign Export / Verify Signature remain unimplemented global features. These
new exports are unsigned; there is no misleading signature placeholder.
No Stable or complete Statistical Analysis claim is justified by this stage.

## Independent validation

`scripts/statistics_oracle.py` creates invented golden data using SciPy
1.17.0, statsmodels 0.14.5 and Pingouin 0.5.5. Python is a development-only
reference and is never imported by the application or release build.
`web/tests/statistics.test.mjs` runs the actual bundled R functions and compares
SS, df, MS, F, p, effect sizes, group SD/SEM/CI, Tukey intervals/adjusted p,
Shapiro/Brown–Forsythe and repeated-measures Mauchly/GG/HF results.

Fixtures cover balanced, unbalanced, heteroscedastic, interaction, decimal
and complete repeated designs. Constant values, insufficient groups and
non-finite input are rejected. App tests cover source/receipt invalidation,
revisions, profile isolation, missing values, experimental units, source
module separation, graph immutability and export contracts.

Numeric comparisons use relative/absolute tolerance `1e-8`, except Tukey
confidence endpoints (`1e-4`): R documents `qtukey` as accurate to the fourth
decimal place, which matters for extreme quantiles with small residual df.
Difference estimates and adjusted p-values retain the stricter tolerance.
For a Tukey family with exactly two levels, the exact studentized-range
identity `sqrt(2) * abs(t)` uses R's mature `qt`/`pt` implementation; it avoids
low-df tail approximation error while preserving simultaneous-family
Bonferroni adjustment. Families with more than two levels use `TukeyHSD`.
A non-estimable conditional family is reported explicitly, with no fabricated
comparison; the full main model and other estimable families remain visible.
See https://stat.ethz.ch/R-manual/R-devel/library/stats/html/Tukey.html .

Runtime licensing and corresponding-source requirements are documented in
[STATISTICAL_RUNTIME_LICENSES.md](STATISTICAL_RUNTIME_LICENSES.md).
