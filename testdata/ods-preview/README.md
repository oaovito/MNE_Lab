# Invented ODS review fixture

`invented.ods` contains invented cells, not instrument measurements or private
experimental data. Regenerate it with `python3 scripts/generate_ods_preview_fixture.py`.
The Python standard-library ZIP writer is independent of the product's Go reader.
The generated content and manifest were checked against the official OASIS ODF
1.3 Relax NG schemas using lxml during development. Neither tool is a product
runtime dependency.

The fixture exercises repeated physical rows/columns, display text distinct
from declared numeric values, percent/date/boolean/duration text, missing versus
empty versus zero, Unicode truncation and explicit ODF whitespace. Its familiar
header does not authorize scientific interpretation. Native tests also mutate
copies to check unsafe/malformed sources and resource limits; browser tests
review it without confirming or saving measurements.

Source specification: OASIS OpenDocument 1.3, Part 2 §3.3 and Part 3 §§6.1.2–6.1.5,
19.389, 19.679.3 and 19.681. The official committee's copy used for review is
[pinned here](https://github.com/oasis-tcs/odf-tc/tree/16a59d945875bd74834e82e166bebded316478da/docs/odf1.3/os).
