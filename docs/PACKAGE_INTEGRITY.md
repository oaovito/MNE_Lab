# Research package integrity

New exports use `mnelab-package/2`. The sorted manifest and SHA-256 list cover
every payload file, including `README.txt` when present. The original scientific
file bytes and scientific cells are unchanged. Manifest/checksum metadata are
the indices themselves; ZIP headers and comments are not authenticated.

The native backend `export.VerifyPackage` reads without extracting or invoking
other programs. It accepts the exact canonical JSON emitted by the package
writer, checks ZIP streams/CRC, paths, duplicate names (including case aliases),
declared sizes, SHA-256 values and checksum-list correspondence. Unexpected,
missing, linked, encrypted and unsafe entries fail without partial success.
Input and total expanded data are each bounded to 512 MiB, with 4,096 ZIP entries
and 8 MiB per manifest/checksum index. Entry paths are limited to 2,048 UTF-8
bytes to bound the index's in-memory names. Larger archives are outside this initial
verifier's supported scope. Writer paths must be safe and cannot collide with
the reserved indices or generated README.

Historical `mnelab-package/1` packages remain readable. Their optional README
was not in the manifest: the verifier reports `Uncovered: ["README.txt"]` and
`WholePayloadCovered: false`, and does not fabricate a checksum or change the
historical package. Other unexpected legacy files are rejected.

This is **PARTIAL backend infrastructure**. There is no global verification UI
or certificate/PFX/CMS/PDF signature implementation yet. Matching a package's
own hashes is integrity correspondence, **not authentication**: an attacker can
rewrite both contents and hashes. `Authenticated` is always false. It does not
validate a signing identity, trust, timestamps, revocation or scientific results.

Independent invented fixtures in `testdata/package-integrity` use Python's
standard ZIP/JSON/SHA-256 implementations, separately from the product writer.
Tests include legacy coverage, changed README/original/checksum data, missing and
extra files, ambiguous JSON, duplicate/unsafe paths, links, resource limits and
CRC errors. This development fixture generator is not an end-user dependency.
