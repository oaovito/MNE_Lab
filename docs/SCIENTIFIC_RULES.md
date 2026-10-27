# LIGHTSCATTERING scientific rules

The current parser is `lightscattering-parser/1.7.0`, using
`ls-spec/0.3-provisional`. The owner-supplied scientific reference has been
consulted privately; interpretation of instrument algorithms and undocumented
fields remains provisional. Development validation uses private experimental corpora outside the source
tree, public fixtures and release packages.

An independent literal reader matched 216 summary quantities and 216
distribution candidates in 54 private reports. This establishes extraction
fidelity for those layouts; undocumented scientific meanings remain
unresolved. The complete scope and remaining evidence are recorded in
[DLS_VALIDATION.md](DLS_VALIDATION.md). The specification remains provisional.

## Current and average count rates

`Current Count Rate` and `Average Count Rate` are separate reported quantities.
The parser stores the current reading as `count_rate` and an explicitly
labeled average as `average_count_rate`. It also accepts the exact shortened
label `Avg. Count Rate`; it does not infer an average from a current reading
or reinterpret an average as current. A bare `Count Rate` says neither, so it stays an
unmapped field with its label and text.

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

## Field labels and units

`Polydispersity` is read as the reported polydispersity. `PDI` and
`Polydispersity Index` stay unmapped: other instruments define a PDI with
their own models, and a label alone does not establish the same quantity.
Declared units are kept exactly as written (only surrounding space is
removed): `C` is not rewritten as `°C`, `mcps` as `Mcps` or `um` as `µm`.
Units that differ only in spelling stay distinct, and statistics refuse to
combine them like any other unit mismatch.

Instrument reports print `Effective Diameter: 51.3` without a unit and state
it in the runs table header of the same report (`Eff. Diam. (nm)`). The parser
takes that unit only when the table's `Combined` row repeats the summary value
exactly; the quantity then records `unitOrigin: file_table_header` and the
header line. Otherwise the unit stays missing and is reported. Reports imported
with an earlier parser get the same lookup once when the profile opens: only
the unit is added, and the file records `unitsReviewed` with the parser that
did it. `Polydispersity` and `Baseline Index` are dimensionless.

## Instrument distribution alternatives

Lognormal and Multimodal distributions and their report/spreadsheet
representations remain separate candidates. Plotting requires an explicit
selection, and saved graph definitions pin that selection.

Instrument `d(nm)` is diameter in nanometers. `G(d)` is retained as relative
intensity with its original precision and no assumed percentage normalization.
`C(d)` remains an unmapped auxiliary field. No conversion to volume or number
weighting is inferred.

## Instrument readings (runs table)

Version 1.7.0 also keeps the rows of a NanoBrook runs table as read-only
instrument readings, and only when the file has an explicit runs table: a
header line naming its columns followed by numbered rows. Each reading keeps
the run number, the source line, the header text and the header unit exactly
as written, and every cell exactly as written next to the number read from it.
Rows labelled `Mean`, `Combined` or similar are not readings. A second runs
table in the same report is ignored with the warning
`ls.run_table_ignored`. Files without a runs table get no readings; nothing is
derived from the summary values. The summary quantities of the measurement are
read exactly as in 1.6.0; the readings never replace or change them.

Readings are technical readings of one measurement. They are never
experimental units, never observations of a design and never count toward N:
twelve runs of one sample are one measurement of one preparation. Files
imported with an earlier parser version get their readings from the stored
original when the profile opens, only when the re-read summary of every
measurement matches the stored one value for value; otherwise they get none.

## Experimental design

A design (LIGHTSCATTERING → Designs) records, without touching any
measurement, which experimental unit (an independent preparation) each
measurement belongs to, its condition and its time point, which measurements
are only technical repeats of their unit's observation, and which conditions
are paired. A condition may be declared as derived from another one of the
same preparation (for example C-SR obtained from F-SR by centrifugation); its
measurements are then paired with the base condition within each unit, and a
pair never joins two units. Time is a factor within the unit when the same
units are measured at every time point.

N is the number of experimental units. Technical repeats are listed with
their unit's observation and never add to N; two experimental replicates of
one unit in one cell are flagged as an error of the design, as are units that
exist only under one condition when the names suggest the same preparation
(pseudoreplication). The detected structure (paired, longitudinal, paired
longitudinal or independent groups) is a review aid; no analysis is chosen or
run from it automatically.

Every save is a new immutable revision that keeps the field-level changes
from the previous one (which measurement, which field, before and after) and
an optional reason. A save against a revision that is no longer current is
refused, so a change made meanwhile elsewhere is never overwritten.

Units, conditions and times are never inferred silently from file names,
sample names or labels. The suggestion reads only the levels and units the
person declared (plus one short remaining word as a possible new unit name),
shows the text it read each proposal from, stores nothing, and applies only
the proposals the person confirms; every part can be corrected afterwards.

## Time and replication

Measurement time comes from the original file, never the import time. Only
fields that name the measurement are read as its date and time: `Measurement
Date`, `Date of Measurement`, `Measured`, `Acquired`, `Measurement Time` and
`Time of Measurement`. A bare `Date`, `Time`, `Date Time` or `Timestamp` can
be a report or export time; it stays an unmapped field. The
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
