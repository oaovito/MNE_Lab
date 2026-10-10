# MBESS source subset

MBESS 5.0.1, Ken Kelley (ci.pvaf) and Ken Kelley / Keke Lai
(conf.limits.ncf). Copyright and licensing remain with the upstream authors.
Source revision: `60919e2feb9d743b6f137441c934dc79bc5f88f7` in the CRAN MBESS mirror.
The manifest records exact, unmodified source/document bytes and SHA-256.
Only these two R functions are used; MBESS and its other dependencies are
not installed or bundled. CRLF is normalized only when evaluating source.

Upstream license: GPL-2 | GPL-3. MNE Lab supplies the GPL-2 text in
`docs/STATISTICAL_PACKAGES_LICENSES.md`; the build copies that text and this
notice with the applicable sources/manifest. The general runtime distribution
and Corresponding Source gates remain in STATISTICAL_RUNTIME_LICENSES.md.

The wrapper restricts use to classical independent one-factor fixed-effect
ANOVA, two-sided equal-tail intervals for the population proportion of
variance accounted for. Source lower-limit NA is converted to zero exactly
as upstream documents. A missing upper limit is explicitly non-estimable.
No estimator for partial eta squared, omega squared or other designs is
claimed. A per-calculation 10,000-pf-call guard and rejection of upstream
warnings bound numerical work without replacing the estimator.
