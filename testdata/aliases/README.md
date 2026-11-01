# Alternative-title test fixtures

Handwritten IMDb-shaped TSV files for catalogue and alternative-title tests. They are not downloaded from IMDb or loaded by a normal app installation.

Identifiers and well-known regional titles are illustrative; vote counts are invented. `tt9999902` is fictional: it represents an unrelated title sharing the French name “La Reine des neiges”.

Rows deliberately cover malformed input and titles that should be excluded: working names, literal translations, format versions, originals, wrong languages and shared regions without an explicit language. See the [catalogue tests](../../internal/catalog/akas_test.go) for the expected results and [title data](../../DATA.md) for the distinction between fixtures and production data.
