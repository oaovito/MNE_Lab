# Synthetic design graph export (golden files)

Every value here is invented. The files are the data tables a condition × time
graph exports: the readings with their experimental unit, condition, time point
and replicate kind (`graph3-data.csv`), each unit's value per cell, the mean of
its technical readings (`graph3-units.csv`), and the condition × time summary
over experimental units (`graph3-statistics.csv`). Column names are the
untranslated interface keys.

They are produced by `internal/export/design_test.go` from an invented
3 units × 2 conditions × 2 time points design with one extra technical reading.
Regenerate them with:

    go test ./internal/export -run TestDesignExportGolden -update-design-golden
