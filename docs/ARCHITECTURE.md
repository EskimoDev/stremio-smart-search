# Architecture

The application is a single Go executable with an embedded local setup page. There are no native media hooks, Stremio patches, browser extensions or runtime package downloads.

## Search

The read-only index contains exact normalized titles, joined-title forms, acronym variants, token postings and a trigram-to-vocabulary index. A fuzzy query first selects bounded candidate words and confirms them with bounded Damerau-Levenshtein distance, including adjacent transpositions. Query terms are intersected. Exact names outrank aliases and fuzzy hits. Vote counts rank otherwise similar matches.

Optional alternative titles extend the same index; they are not a separate engine. Their normalized forms live in one sorted key array with 32-bit title references. A second ordering of the same keys, which ignores spaces, serves joined queries such as "lareinedesneiges" without storing another copy. Their words have separate postings, but share the vocabulary and trigram index, and therefore all typo handling. Score bands are: canonical exact 2200, canonical joined 2100, alternative exact 2000, alternative joined 1900, acronym 1200, then word matches. Alternative-title word matches are weighted ×0.9, so canonical words win ties. Acronyms are never generated from alternative titles. Short Latin alternative titles (five letters or fewer once spaces are removed, e.g. "H.O.T.D.") rank in the acronym band, so popularity decides between titles that share them. Aliases are search keys only: results return the canonical ID and name, and a title appears once however many of its names match. Very short Latin aliases are not indexed. Normalization (`internal/textnorm`) folds accents and common Latin letters such as ß, æ, œ, ł, š and ř, and keeps non-Latin scripts. The catalogue builder and the index use it identically. For 4–6 letter ASCII terms, the matcher also probes every interior single-edit variant against the vocabulary. Trigram candidates miss central typos in short words ("rene" for "reine", "alein" for "alien").

Input is limited to 120 Unicode characters and eight terms; results to 20 per request. Prefix expansion, trigram candidate scans, fuzzy word checks, posting lists and simultaneous searches have explicit limits. The normal HTTP request loop never needs an external search call. A request context bounds fuzzy work and the service limits matching concurrency.

Three catalogue profiles bound selection: Compact (25k movies + 25k series, minimum 250 votes), Standard (100k + 100k, minimum 25) and Expanded (250k + 250k, minimum 5). Standard is the new-setup default. These are independent per-type ceilings; actual eligible counts can be lower. Legacy snapshots without a profile are treated as Compact.

The data builder streams the same compressed IMDb files for every profile, excludes individual episodes and adult-marked content, and stores a compressed local JSON snapshot. When a language is selected for alternative titles, a third stream (`title.akas`) is read after title selection. A row is rejected after parsing only its ID unless that ID is already selected. Remaining rows are filtered by language and region. IMDb mostly leaves the language as `\N` and gives an ISO-style region, so each language maps to regions where it is the usual language. Multilingual regions such as CA, BE and CH count only when the row names the language. Working, original, literal, promotional and "version" rows are dropped. The rest are ranked (IMDb display title first), deduplicated after normalization, capped at four per title, and capped per size: Compact 75k, Standard 200k, Expanded 200k, keeping the most-voted titles. Expanded's extra titles are low-vote and rarely have regional titles, and its index already sits near the memory target. Snapshots gain optional `aliases`, `aliasLanguage` and `aliasRegion` fields, and the snapshot version stays 1. Older snapshots load with alternative titles off. Older builds ignore the fields and keep working. A snapshot naming an unknown language, for example from a newer build, still loads. The language selection is applied like a size change. It persists only with a successful replacement snapshot, and a build request that omits it keeps the active choice. A refresh builds a replacement separately and swaps the serving index only after a valid snapshot is saved. Active and building profiles remain separate in status; the successful snapshot persists its profile. The previous snapshot supports recovery after interruption. The loader accepts at most 500,000 titles, 600,000 alternative titles (eight per title) and 128 MiB of decompressed JSON; ratings parsing is also bounded. The executable retains its 256 MiB Go soft memory target and two Go execution threads. The target is not a process memory ceiling: live refresh temporarily holds the old and new index and can exceed it. See [measured resource use](PERFORMANCE.md).

## Stremio protocol

- `/manifest.json`: advertises two search-only catalogs, `movie/smart-search` and `series/smart-search`.
- `/catalog/{type}/smart-search/search={query}.json`: returns standard metadata previews with stable IMDb IDs.
- No `stream` resource; existing addons handle playback.
- No account authentication, watch-activity events or library events.
- Extra values are decoded exactly once from the escaped URL path; punctuation in a query cannot become a route separator.

Protocol responses include CORS headers for Stremio clients. `/api/*` management routes do not expose CORS and require a private control key, a loopback peer, an expected Host and a matching browser Origin when present. The settings page rejects framing and uses a content security policy. No arbitrary local path or external URL can be executed through a search request.

## Lifecycle

The launcher starts the companion and opens the registered Stremio URL handler. An authenticated existing instance is reused instead of starting another engine. The service remains available without polling Stremio's processes. The browser page may be closed. Quit and OS shutdown cancel any active data preparation.

The helper is loopback-only by default. An advanced `--listen` option permits server hosting, but remote settings remain blocked. Cross-device use requires client-compatible transport, commonly HTTPS through a reverse proxy. These paths require separate verification from a local desktop install.

## Performance claims

Measure cold startup, catalogue preparation, idle CPU, total process memory and latency against real title data. A small-fixture microbenchmark is useful for regressions but is not a substitute for an actual catalogue or device test. Cross-compilation does not establish ARM or TV performance. No zero-overhead or perfect-matching guarantee is made.
