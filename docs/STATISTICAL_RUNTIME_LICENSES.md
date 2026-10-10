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
application. Additional packages such as `nlme`/`multcomp`/`mvtnorm` must have
their source, licenses, runtime compatibility and independent numerical
validation recorded before Mixed Effects or Dunnett can be enabled.
