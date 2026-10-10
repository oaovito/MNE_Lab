# Invented workbook mapping fixture

`invented.xlsx` is synthetic data, generated directly as OOXML ZIP/XML by
`scripts/generate_workbook_mapping_fixture.py`, without Excel or Excelize.
SHA-256: `c5011bdc548a4006be9134322aa1ae3fc9dda9cfd860cd1b0009f9c055d78aee`.

The XML declares numeric `B3 = 1.2300`, numeric `C3 = 0.0000`, text
`D3 = 1,2500`, shared-string `A3 = Synthetic\nsample`, empty quantities in
row 4, numeric zero in row 5 and an omitted physical row 6. Rows 7–28 declare
`B = (row - 5).0000`. Header row 2 deliberately names an unknown size in
`(um)`; tests assign `nm` explicitly without changing values or headers. The
number format of B3 does not change its declared precision. A second sheet
contains unrelated literal text.

The fixture proves raw extraction, source coordinates, limits and explicit
user declarations. It is not an instrument result or a scientific reference.
Regeneration is byte-for-byte deterministic. Mutated test variants check
formulas/merges/links on excluded sheets, duplicate or conflicting coordinates,
unsafe numbers and unsupported cell types without shipping real source data.
