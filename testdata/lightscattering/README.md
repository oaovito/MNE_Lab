# LIGHTSCATTERING test files

These files are **synthetic**. They were written to exercise the parser's
handling of the fields named in the scientific specification (Effective
Diameter, Polydispersity, Current Count Rate, BaseLine Index, Diameter,
Intensity, Volume, Number), delimiters, decimal separators, encodings,
missing fields and malformed input.

They are not exports from a NanoBrook 90Plus and must not be used as
evidence that the parser reads real instrument files. Real exports are
added under `real/` when available, and the parser is validated against
them (see `docs/TECHNICAL_CONTEXT.md`, "Scientific validation").
