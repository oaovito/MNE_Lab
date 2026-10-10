# Statistical runtime dependencies

The pinned npm package is `webr@0.6.0`, published by the webR project:
https://github.com/r-wasm/webr/tree/v0.6.0

Its distribution includes R 4.6.0, BLAS/LAPACK, the Emscripten worker and the
R lazy filesystem. Its `LICENSE.md` states that distribution binaries are
GNU GPL version 3; other webR application/build scripts are MIT unless
specified otherwise. Package metadata's MIT entry alone does not describe
the binary runtime license.

`web/build.mjs` copies the complete upstream `LICENSE.md` alongside runtime
assets. It retains GPL text and external component notices/source links for
R, PCRE2, liblzma, libgfortran and other components. Runtime binaries are
unmodified. The application statistical adapter is
`web/src/lib/statistics-engine.R`, supplied as readable source in this repo.

Authoritative upstream build instructions and component sources:

- https://github.com/r-wasm/webr/tree/v0.6.0
- https://github.com/r-wasm/webr/blob/v0.6.0/README.md
- https://github.com/r-wasm/webr/blob/v0.6.0/LICENSE.md
- https://www.r-project.org/

A public binary release must supply the applicable Corresponding Source,
including exact runtime/component sources, patches, pinned build machinery
and any required installation information, through a GPL-compliant
distribution method. Upstream links and a copied license are **not by
themselves** evidence that this release requirement has been fulfilled.
Source acquisition/reconstruction and the project's complete distribution
licensing decision remain release gates. This development commit does not
announce a binary release or change the license of the existing repository.

R's `stats` library supplies numerical methods. SciPy, statsmodels and
Pingouin are independent development references only and do not ship in the
application. The build now pins eleven additional packages in
`scripts/statistics-packages.json`, with native length/SHA-256 checks and source
versions/URLs/SHA-256. Sources of matching versions were acquired: ten from the
same official repository's source index (MD5 checked); mvtnorm 1.2-4 from its
CRAN mirror revision recorded in the manifest. Their native runtime loads
offline through the same read-only WORKERFS image in Node and browser tests.
Dunnett has independent refined multivariate-t p/CI comparisons and seed
repeatability tests. Mixed remains preparation; merely bundling `nlme` does
not validate or enable it.

Every package's included author/license/component files remain in the image.
[STATISTICAL_PACKAGES_LICENSES.md](STATISTICAL_PACKAGES_LICENSES.md) records
package versions and includes the pinned R runtime's full GPL-2 text. The
build copies it alongside the upstream webR/GPL-3 license. These packages
use GPL-family licensing with package/component exceptions; the manifest is
the version-specific record. Matching package source archives do not by
themselves reconstruct the exact Emscripten toolchain, patches or full webR
runtime. The Corresponding Source and complete distribution licensing gates
above continue to apply; no public binary release is announced.
