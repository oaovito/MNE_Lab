# LIGHTSCATTERING scientific rules

The current parser is `lightscattering-parser/1.4.0`, using
`ls-spec/0.3-provisional`. The owner-supplied scientific reference has been
consulted privately; interpretation of instrument algorithms and undocumented
fields remains provisional. Development validation uses private experimental corpora outside the source
tree, public fixtures and release packages.

## Current and average count rates

`Current Count Rate` and `Average Count Rate` are separate reported quantities.
The parser stores the current reading as `count_rate` and an explicitly
labeled average as `average_count_rate`. It also accepts the exact shortened
label `Avg. Count Rate`; it does not infer an average from a current reading
or reinterpret an average as current.

Each quantity retains its original numeric text, explicit unit, exact source
label and source line. A missing unit remains missing and produces a warning.
The application does not infer `kcps` or `Mcps` from the magnitude of a value.

Graphs, cycle statistics and normalized exports can select the average
independently. Current-only and average-only measurements leave the other
quantity absent. The current implementation refuses statistics across mixed
units; normalized measurement exports retain per-value units when units differ.
No automatic count-rate scaling or unit conversion is applied.

The official scientific reference names Current Count Rate. Privately reviewed
instrument exports explicitly name Average Count Rate. Their different labels
do not establish identical acquisition semantics. This separation preserves
what the instrument actually reports while manufacturer-level interpretation
remains under review.

## Instrument distribution alternatives

Lognormal and Multimodal distributions and their report/spreadsheet
representations remain separate candidates. Plotting requires an explicit
selection, and saved graph definitions pin that selection.

Instrument `d(nm)` is diameter in nanometers. `G(d)` is retained as relative
intensity with its original precision and no assumed percentage normalization.
`C(d)` remains an unmapped auxiliary field. No conversion to volume or number
weighting is inferred.

## Time and replication

Measurement time comes from the original file, never the import time. The
parser retains the `TZKnown` flag and ambiguous numeric date order requires
confirmation. Cycle autoassociation refuses to compare a known-zone instant
with an unknown-zone wall clock: the person must confirm its cycle point.
Confirming date order alone does not establish a time zone. Known-zone dates
with different offsets compare actual instants; unknown-zone dates retain
the existing wall-clock matching behavior. Graph and export provenance retain the original date text, zone-knowledge,
ambiguity and confirmation flags in a timestamp snapshot. Unknown-zone ISO
text carries no inferred offset; fractional seconds are retained. Existing
UI wall-clock and cycle-start handling is covered by browser regressions in
UTC and America/Sao_Paulo in all three languages. Source run counts
remain metadata; they do not create extra independent measurements or imply
experimental triplicates.

Cycle statistics retain individual values and use sample standard deviation
only with at least two observations. Missing observations remain gaps.
Repeating a measurement identifier in a cycle request does not create another
observation: preview, planning and saved cycles use the same unique records.
The planned replicate count does not manufacture additional observed values.
