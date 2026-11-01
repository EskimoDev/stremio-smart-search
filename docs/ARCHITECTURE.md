# Architecture

The application is a single Go executable with an embedded local setup page. There are no native media hooks, Stremio patches, browser extensions or runtime package downloads.

## Search

The read-only index contains exact normalized titles, joined-title forms, acronym variants, token postings and a trigram-to-vocabulary index. A fuzzy query first selects bounded candidate words and confirms them with bounded Damerau-Levenshtein distance, including adjacent transpositions. Query terms are intersected. Exact canonical names and longer regional names outrank acronyms and fuzzy hits; short Latin regional names use the acronym band. Among close word matches, a title whose significant words the query covers gets up to 10 of 100 extra points, so "the terminater" prefers *The Terminator* over *Terminator 2*. Vote counts rank otherwise similar matches. A four-digit year (1880–2099) typed with other words is matched by a title's start year or by a title word, so "dune 1984" picks that film; if no candidate matches, the year is ignored. A single word of up to five letters that is some title's acronym gets exact and prefix matches only, no typo variants, so "hotd" does not also list "Hot Fuzz" or "Robin Hood". As a last resort, a query word spelled the way it sounds ("incepshun", "nite") is matched to indexed words with the same Double Metaphone code, for ASCII words only, at most eight per query word and at less than half the weight of a one-letter typo, so it never displaces an exact title. Accuracy is measured with [curated cases](SEARCH-QUALITY.md).

Optional alternative titles extend the same index; they are not a separate engine. Their normalized forms live in one sorted key array with 32-bit title references. A second ordering of the same keys, which ignores spaces, serves joined queries such as "lareinedesneiges" without storing another copy. Their words have separate postings, but share the vocabulary and trigram index, and therefore all typo handling. Score bands are: exact 2200 and joined 2100, canonical and alternative alike, so popularity decides between them; acronym 1200; then word matches. Alternative-title word matches are weighted ×0.9, so canonical words win ties. Acronyms are never generated from alternative titles. Short Latin alternative titles (five letters or fewer once spaces are removed, e.g. "H.O.T.D.") rank in the acronym band, so popularity decides between titles that share them. Aliases are search keys only: results return the canonical ID and name, and a title appears once however many of its names match. Very short Latin aliases are not indexed. Normalization (`internal/textnorm`) folds accents and common Latin letters such as ß, æ, œ, ł, š and ř, and keeps non-Latin scripts. The catalogue builder and the index use it identically. For 4–6 letter ASCII terms, the matcher also probes every interior single-edit variant against the vocabulary. Trigram candidates miss central typos in short words ("rene" for "reine", "alein" for "alien"). For 7–20 letter ASCII terms it probes every deletion and adjacent transposition, which can break up to four trigrams ("thornes" for "thrones").

Input is limited to 120 Unicode characters, 16 words and eight looked-up terms (words of three or more characters other than articles); results to 20 per request. Prefix expansion, trigram candidate scans, fuzzy word checks, posting lists and simultaneous searches have explicit limits. The normal HTTP request loop never needs an external search call. A request context bounds fuzzy work and the service limits matching concurrency.

## Catalogue preparation

Three catalogue profiles bound selection: Compact (25k movies + 25k series, minimum 250 votes), Standard (100k + 100k, minimum 25) and Expanded (250k + 250k, minimum 5). Standard is the new-setup default. These are independent per-type ceilings; actual eligible counts can be lower. Legacy snapshots without a profile are treated as Compact.

The data builder streams the same compressed IMDb files for every profile, excludes individual episodes and adult-marked content, and stores a compressed local JSON snapshot. When a language is selected for alternative titles, a third stream (`title.akas`) is read after title selection. A row is rejected after parsing only its ID unless that ID is already selected. Remaining rows are filtered by language and region. IMDb mostly leaves the language as `\N` and gives an ISO-style region, so each language maps to regions where it is the usual language. Multilingual regions such as CA, BE and CH count only when the row names the language. Working, original, literal, promotional and "version" rows are dropped. The rest are ranked (IMDb display title first), deduplicated after normalization, capped at four per title, and capped per size: Compact 75k, Standard 200k, Expanded 200k, keeping the most-voted titles. Expanded's extra titles are low-vote and rarely have regional titles.

Snapshots gain optional `aliases`, `aliasLanguage` and `aliasRegion` fields, and the snapshot version stays 1. Older snapshots load with alternative titles off. Older builds ignore the fields and keep working. A snapshot naming an unknown language, for example from a newer build, still loads. The language selection is applied like a size change. It persists only with a successful replacement snapshot, and a build request that omits it keeps the active choice. A refresh builds a replacement separately and swaps the serving index only after a valid snapshot is saved. Active and building profiles remain separate in status; the successful snapshot persists its profile. The previous snapshot supports recovery after interruption.

The loader accepts at most 500,000 titles, 600,000 alternative titles (eight per title) and 128 MiB of decompressed JSON; ratings parsing is also bounded. The executable retains its 256 MiB Go soft memory target and two Go execution threads. The target is not a process memory ceiling: live refresh temporarily holds the old and new index and can exceed it. See [measured resource use](PERFORMANCE.md).

## Storage

Settings, the private control key and, by default, the catalogue live in the OS configuration folder (`os.UserConfigDir()/StremioSmartSearch`, or `--data-dir`). The catalogue is `catalog.json.gz`. Saving temporarily renames the old file to `catalog.json.gz.previous`; the loader can recover from it after an interrupted replacement. A successful save removes it, so it is not a permanent backup.

A custom catalogue folder is recorded in `storage.json` in the settings folder. Settings never move, so launches using that same data folder and port can authenticate and reuse the helper. A move copies the catalogue through temporary names, records the new folder, swaps it in and only then removes the old copies. It is refused during a build and checks free space first.

Storage figures are read on request (two file sizes and a walk of the small settings folder), with no background bookkeeping. The Memory value is `runtime.MemStats.Sys - HeapReleased`, an estimate of memory retained by the Go runtime rather than the OS's total process working set. The `/api/storage` routes have the same loopback, Host, Origin and control-key checks as the rest of `/api/*`.

## Stremio protocol

- `/manifest.json`: advertises two search-only catalogs, `movie/smart-search` and `series/smart-search`.
- `/catalog/{type}/smart-search/search={query}.json`: returns standard metadata previews with stable IMDb IDs.
- No `stream` resource; existing addons handle playback.
- No account authentication, watch-activity events or library events.
- Extra values are decoded exactly once from the escaped URL path; punctuation in a query cannot become a route separator.

Protocol responses include CORS headers for Stremio clients. `/api/*` management routes do not expose CORS and require a private control key, a loopback peer, an expected Host and a matching browser Origin when present. The settings page rejects framing and uses a content security policy. No arbitrary local path or external URL can be executed through a search request.

## Lifecycle

The launcher starts the companion and opens the client chosen in setup. With no saved preference, it opens the registered Stremio URL handler. An authenticated existing instance is reused instead of starting another engine. The service remains available without polling Stremio's processes. The browser page may be closed. Quit and OS shutdown cancel any active data preparation.

The helper is loopback-only by default. An advanced `--listen` option permits server hosting, but remote settings remain blocked. Cross-device use requires client-compatible transport, commonly HTTPS through a reverse proxy. These paths require separate verification from a local desktop install.

## Command-line options

Run the platform's helper executable with `--help` for its options.

| Option | Effect |
| --- | --- |
| `--launch` | Start or reuse the helper and open the chosen Stremio client; open setup if no catalogue is ready |
| `--no-browser` | Run without opening settings or launching a client |
| `--quit` | Stop the instance authenticated by the selected data folder on the selected port |
| `--listen 127.0.0.1:11479` | Set the listening address; this is the default |
| `--data-dir PATH` | Choose the settings and default catalogue folder |
| `--catalog PATH` | Load a JSON or JSON.gz catalogue for this run, instead of restoring the managed one |
| `--version` | Print the version and exit |

Portable and installed builds use the same default OS settings folder. Use both a
separate `--data-dir` and a different port to run an isolated development instance.
The listen address is a launch option, not a saved setting.

## Performance claims

Measure cold startup, catalogue preparation, idle CPU, total process memory and latency against real title data. A small-fixture microbenchmark is useful for regressions but is not a substitute for an actual catalogue or device test. Cross-compilation does not establish ARM or TV performance. No zero-overhead or perfect-matching guarantee is made.
