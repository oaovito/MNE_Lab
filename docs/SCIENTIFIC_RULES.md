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
No automatic count-rate scaling or unit conversion is applied to stored data;
the only conversion is the display-only one described under "Units of
measure" below.

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

## Units of measure (registry and display conversion)

Settings → Manage Units lists units of measure (not experimental units).
System scientific units are exactly the spellings the LIGHTSCATTERING parser
recognizes: nm, µm (U+00B5), μm (U+03BC), um, mm, cps, kcps, Kcps, Mcps, mcps,
s, ms, °C, C, K, mV, cP, %, a.u., au, deg and °. Each spelling is its own
entry; none is an alias of another. deg and ° stay unclassified because they
can name an angle or a temperature. System units are never deleted; they can
be hidden and shown again. A test checks this list against the parser.

Custom units are created by the person with a symbol (kept exactly as
written, up to 64 characters), an optional name and a dimension. A symbol
that repeats a registered spelling is refused. The column-mapping unit field
uses a searchable selector that offers only units of the field's dimension
(a diameter field never offers count-rate units), with favourites and units
recently used on this computer. Archived or hidden units are not offered,
but data, recipes, graphs and analyses that use one keep it and show it.

A custom unit that nothing uses can be deleted. One that is used (by
measurements, saved import recipes, graphs or cycles through their
measurements, or saved analysis snapshots) is never deleted destructively:
the choices are Archive, Replace Usage Then Delete, or Cancel. Replacement
rewrites only units a person assigned (column-mapped measurement values and
saved recipes) to another registered unit of the same dimension, records a
provenance entry with the old definition and every change, then deletes the
custom unit. Values are not converted. Units stated by a source file and
saved analysis snapshots are never rewritten; while any use them, replacement
is refused and archiving remains available. The file's original import
declaration is kept as imported.

Conversion is display-only and exists only between exact spellings of the
same dimension with exact factors: nm, µm and m; cps, kcps and Mcps; s, min, h
and d. It always carries the original value, its source text and its unit
next to the display value and unit; the stored value is never changed.
Different dimensions and any pair without an exact factor (including spelling
variants such as um, μm, Kcps or mcps) are refused. Decimals are carried
through only for power-of-ten factors.

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

## Design graphs, Combine and annotations

A condition × time graph draws a saved design revision. For each condition
and time point the value of an experimental unit is the arithmetic mean of
its readings in that cell (one reading gives itself); technical readings are
averaged first and never counted, so twelve readings of one unit are n = 1.
The mean and error bar of a cell are computed over the units' values, and n
is the number of units with a value. Error bars are always named on the
figure: sample SD (n − 1) by default, SEM on request, and a confidence
interval only when the graph links a saved analysis of the same design and
variable, whose stored interval is drawn as saved. A cell with one unit has
no SD and is drawn without a bar. Each unit's value is visible as a point;
its trajectory over time is optional. A paired view joins the values of the
same unit under the conditions at one time point; the line means the same
experimental unit, never two units, and when the design declares no pair for
that unit the figure says so.

Combine builds a new graph from the data of saved graphs, never from their
images, and keeps every parent unchanged. It is refused when the graphs draw
different variables, different designs or design revisions, different X
domains or values stored in different units. When the units differ but
every one converts to a common unit with an exact factor (nm, µm, m; cps,
kcps, Mcps; s, min, h, d), that conversion is offered and applied to the
display only; there is no conversion otherwise. Two conditions stored in
different units are never compared by a bracket.

Statistical annotations (brackets with the adjusted p-value, stars from the
adjusted p-value, or the estimated difference with its interval and adjusted
p-value) come only from the comparisons of a saved analysis; nothing is
recalculated to draw them, and the correction named by the analysis is shown
with them. Star thresholds are display conventions and do not change the
analysis alpha. A comparison the graph does not show (another condition or
time point) is not drawn and is reported.

Groups and saved selections are lists of measurement or series identifiers.
They never copy values, and a group that holds several readings of one unit
does not make them independent: N still comes from the design.

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
