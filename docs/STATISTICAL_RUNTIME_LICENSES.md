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
repeatability tests. Mixed now has independent ML random-intercept
coefficient/variance/likelihood/GLS/Wald F/df comparisons against
Statsmodels/SciPy and explicit boundary/design rejections. Its scientific
validation is restricted to the documented supported model; merely bundling
`nlme` supplies no validation for other designs.

Every package's included author/license/component files remain in the image.
[STATISTICAL_PACKAGES_LICENSES.md](STATISTICAL_PACKAGES_LICENSES.md) records
package versions and includes the pinned R runtime's full GPL-2 text. The
build copies it alongside the upstream webR/GPL-3 license. These packages
use GPL-family licensing with package/component exceptions; the manifest is
the version-specific record. Matching package source archives do not by
themselves reconstruct the exact Emscripten toolchain, patches or full webR
runtime. The Corresponding Source and complete distribution licensing gates
above continue to apply; no public binary release is announced.

## MBESS source subset for optional population eta intervals

The product evaluates the two unmodified MBESS 5.0.1 functions `ci.pvaf` and
`conf.limits.ncf`; it does not install the complete MBESS package. The exact
source/doc bytes and immutable revision, hashes, authorship and upstream
GPL-2 | GPL-3 license are retained in `web/src/lib/vendor/mbess` and copied to
the embedded runtime with their notice/manifest. CRLF is normalized only at
evaluation. The wrapper bounds upstream CDF work and marks unavailable
intervals explicitly. Existing full GPL-2 text accompanies these sources;
general applicable Corresponding Source/distribution gates remain above.

## Source acquisition checkpoint

The acquired upstream webR 0.6.0 checkout is pinned to
`f116a5e60e220182d09ac015837c88c454c663c5`, matching the official npm
package's `gitHead`; its npm integrity matches this project's lockfile.
Its recorded flang build-script submodule is
`25541509519240557c6c7bc695f3c9615366c219`. The complete tracked source,
patches and build scripts for those two commits were archived with per-member
hash verification. This preparation does not change runtime assets.

The recipes identify R 4.6.0 and Emscripten 5.0.7. The top-level Docker base
uses `flang-wasm:main`, and flang's LLVM source clone uses a mutable `wasm`
branch. Those references do not identify the exact release-time compiler/image.
The R release-source/checksum endpoints returned HTTP CONNECT 403 in the current
environment; required CRAN domain additions were saved for environment review.
Remaining gates include complete release/component source archives and verified
checksums, exact compiler/image inputs, patches, build/install machinery,
reconstruction or a fully documented compliant source distribution, and the
project's distribution licensing decision. A pinned subset of source or a
successful runtime test does not satisfy the complete Corresponding Source gate.
