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
- Dunnett: two-sided single-step multivariate-t comparisons and simultaneous
  confidence intervals for independent classical One-Way ANOVA. The user
  selects a control with included values. Up to eight treatments are supported
  as a computational bound. Every treatment/control identity is explicit;
  alpha must be below 0.5, the mature two-sided quantile library's scope.
  punctuation in labels never determines a contrast. No Welch, repeated,
  mixed, one-sided or factorial Dunnett procedure is implied.
- Group means, sample SD, SEM and Student-t confidence intervals retain
  individual values. These intervals are group mean intervals, not effect
  size confidence intervals or simultaneous post-hoc intervals.

Mixed effects support a restricted Gaussian **ML random-intercept** model:
`nlme::lme`, categorical within-unit B and optional between-unit A, sum
contrasts, `y ~ A*B` (or `y ~ B` for one group), random `~1|unit`, homogeneous
conditional residual errors. Every included unit has at least two distinct
B observations; at least three explicit physical units, every factorial cell,
positive inner/outer residual df and at least two units per between group
are required. Incomplete and unequal repeated observations are allowed only
when the user explicitly chooses Mixed. Missing quantities are omitted, never
imputed. Classical repeated-measures ANOVA retains its complete/balanced limits.

Marginal Wald F tests use `anova.lme(type="marginal", adjustSigma=TRUE)`:
conditional fixed-effect covariance is multiplied by `n/(n-p)` for tests;
reported coefficient SEs are unscaled conditional GLS SEs. Numerator and
inner/outer denominator df, ML log likelihood, full-precision random/residual
variances, coefficient names and ordered factor levels persist and export.
Conditional response residuals feed the QQ/histogram and auxiliary Shapiro test.
Raw group summaries and their Student-t intervals describe original values,
not fitted mixed-model means or model confidence intervals. No classical
SS/effect sizes, Mauchly/GG/HF, independent-group variance test or post-hoc
procedure is supplied for Mixed.

Convergence warnings/errors, non-positive variance information, failed
variance intervals, rank deficiency and scalar random SD/residual SD below
`1e-4` stop calculation. The scalar boundary tolerance follows the documented
[lme4 1.1-37 isSingular default](https://github.com/cran/lme4/blob/79c411060a27b934ee4d541bdb983ea4db7269b2/R/utilities.R);
lme4 is a methodological reference, not a bundled fitting dependency.
This deliberately restricted numerical guard is not a scientific unit rule.
[nlme lme](https://stat.ethz.ch/R-manual/R-devel/library/nlme/html/lme.html)
and [anova.lme](https://stat.ethz.ch/R-manual/R-devel/library/nlme/html/anova.lme.html)
document the model and test conventions; the bundled source version is 3.1-169.
Random slopes, heteroscedastic/serial residual structures, REML selection,
mixed/Welch/RM post-hoc procedures and effect-size confidence intervals remain
pending. No simpler model or unadjusted pairwise t-test substitutes for them.

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
automatic method selectors. Classical repeated observations use sphericity diagnostics. Mixed instead
reports its covariance assumptions; neither applies an independent-groups
variance test to repeated values.

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

The build-only `scripts/statistics_packages.go` verifies every compressed
package's exact length and SHA-256 against `statistics-packages.json`, rejects
unsafe archive paths/entries, and generates a deterministic compressed
WORKERFS image. Its eleven packages are embedded locally; Dunnett and Mixed workers
mount a fresh read-only image. Cache/download/Go tooling belong to the build,
never the product. Current base-R analyses do not load the additional image.

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
effects and corrections for classical analyses. Mixed instead includes seven
tables: marginal tests with denominator df, raw groups, observations,
diagnostics, residuals, model metadata and sum-contrast coefficients. Delimited formats produce a separate file per table
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

`scripts/statistics_advanced_oracle.py` independently checks Dunnett with
SciPy's studentized multivariate-t distribution. The pinned development
`DunnettResult` model and `multivariate_t.cdf` plus Brent's root solver refine
simultaneous confidence limits without adopting its noisy default CI optimizer.
Balanced, unbalanced and single-treatment synthetic fixtures use a control
that is not the first sorted label, with punctuation in labels. Probabilities
are compared with absolute tolerance `3e-5`; CI endpoints use `1e-4` times
`max(1, abs(reference))`; differences retain `1e-8`. The independent reference
uses one million integration points and seed 1701. These are documented
numeric tolerances, not a claim of exact distribution integration.

The actual R engine uses multcomp 1.4-30 / mvtnorm 1.2-4, seed 1701,
Genz–Bretz integration, `maxpts=100000`, `abseps=1e-5`, `releps=0`.
`qmvt` also uses the upstream `ptol=1e-3`, `maxiter=100` and explicit seed 1701.
An excessive reported p integration error, failed quantile completion or CI
warning stops calculation. The pinned mvtnorm does not return `estim.prec`;
this absence is never treated as zero error. A direct mature `pmvt` evaluation
checks achieved simultaneous-CI coverage against `1-alpha` within `3e-5`,
with reported integration error at most `1e-5`. Its achieved coverage difference
and reported integration error are separate persisted diagnostics. Failed
checks stop calculation; no partially converged result is saved.
The reported integration error, settings, simultaneous-family correction,
control and library versions persist in diagnostics/source/results/exports.
Tests also reproduce every output after unrelated RNG activity. Validation
applies to this documented method and tolerances; broader stress fixtures
and other Dunnett designs do not inherit it.

References:
- https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.dunnett.html
- https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.multivariate_t.html
- https://cran.r-project.org/web/packages/multcomp/multcomp.pdf

Runtime licensing and corresponding-source requirements are documented in
[STATISTICAL_RUNTIME_LICENSES.md](STATISTICAL_RUNTIME_LICENSES.md).

### Mixed independent reference

`scripts/statistics_mixed_oracle.py` generates five wholly invented designs:
two/three between groups, three/four categorical times, complete/incomplete
observations and a one-group repeated design. Statsmodels 0.14.5 independently
fits ML variance parameters and fixed coefficients; SciPy 1.17.0 evaluates
GLS covariance and F probabilities. Joint observed-likelihood `bse_fe` is not
the same covariance convention as nlme's conditional GLS SE and is not used
as its oracle. F tests independently use the documented `n/(n-p)` adjustment
and inner/outer df for the restricted supported design. Parameters/F/residuals
use relative tolerance `1e-5` with a unit absolute floor, probabilities `1e-6`,
log likelihood absolute `1e-6`, and df exact equality. Actual product R code
runs against the same read-only local WORKERFS image as browser workers.
Unknown units, duplicate unit/time, unit group changes, absent factorial
cells, constant outcomes, near-boundary variance and post-hoc requests are
rejected. These tests establish only this documented scope; they do not
validate arbitrary mixed models or ZETA/NTA scientific quantities.
