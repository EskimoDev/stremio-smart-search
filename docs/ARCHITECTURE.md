# Architecture

The application is a single Go executable with an embedded local setup page. This document describes **main**. The latest public release is [0.1.0-preview.5](https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.5); main also includes [unreleased search and security fixes](../CHANGELOG.md#unreleased). Preview.4 added phonetic search, automatic updates and theme controls; preview.5 adds the Windows notification-area icon, start at sign-in and the served addon logo. There are no native media hooks, Stremio patches, browser extensions or runtime package downloads.

## Package boundaries

- `cmd/smart-search`: flags, process reuse, HTTP-server lifetime and runtime limits.
- `internal/catalog`: snapshot validation/storage, streamed IMDb parsing, source freshness and alias carry-over.
- `internal/search`: the read-only in-memory index and ranking; no network calls.
- `internal/textnorm`: shared title normalization for catalogue building and search.
- `internal/app`: local management API/UI, client launching, storage moves and the update schedule.
- `internal/quality`: developer-only evaluation against curated cases.
- `tools/` and `scripts/`: builds, packaging, icons and measurement utilities; not runtime dependencies.

## Search

The engine makes no network calls. Its read-only index contains normalized exact titles, joined-title forms, acronym variants, token postings and a trigram-to-vocabulary index. Optional alternative titles extend the same index; they are not a separate engine.

### Candidate matching

- **Spelling.** A bounded trigram scan selects candidate words, then bounded Damerau-Levenshtein distance checks them, including adjacent transpositions. For 4–6-letter ASCII terms, direct single-edit probes also find central typos that share no trigrams (`rene` for `reine`, `alein` for `alien`). For 7–20-letter terms, direct deletion and swap probes recover cases such as `thornes` for `thrones`.
- **Prefixes and missing vowels, since preview.5.** Short prefixes choose completions by title popularity. Rare words with missing vowels can use the consonant-skeleton index (`internal/search/skeleton.go`). Common indexed words are taken as typed.
- **Sound-alikes, since preview.4.** As a last resort, a query word can gain up to eight same-code Double Metaphone matches at weight 0.4 and eight near-code matches at 0.3, below spelling terms (0.76 or 0.6). Words must be ASCII, at least four letters, pronounceable and used by fewer than 50 indexed titles. Length, edit-distance and popularity checks limit loose matches; a one-word query has a stricter limit.
- **Alternative readings, unreleased on main.** A weak query can also try leaving out a word, joining or splitting a fragment, moving a space, or a rare three-letter typo (`internal/search/readings.go`). Each title keeps its best reading score. Strong queries skip this work. This behavior is not in the public preview.5 binary.

Query terms are normally intersected. A single word of up to five letters that is a title's acronym suppresses typo and phonetic expansion, so `hotd` does not also list *Hot Fuzz* or *Robin Hood*. A four-digit year (1880–2099) typed with other words matches a title's start year or a title word, so `dune 1984` selects that film and *Blade Runner 2049* still works. If no candidate matches the year, it is ignored.

### Ranking and alternative titles

| Match | Score band |
| --- | ---: |
| Exact canonical or alternative title | 2200 |
| Joined canonical or alternative title | 2100 |
| Acronym or short Latin alternative title | 1200 |
| Word matches | Below the acronym band |

Vote counts order otherwise similar matches. Among close word matches, a title whose significant words the query covers gets up to 10 of 100 extra points, so `the terminater` prefers *The Terminator* over *Terminator 2*. Alternative-title word matches weigh ×0.9, so canonical words win ties. Exact-title bands are unaffected by the fuzzy fallbacks. Accuracy and known gaps are in [Search quality](SEARCH-QUALITY.md).

Alternative forms use one sorted key array with 32-bit title references. A second ordering, ignoring spaces, serves joined queries such as `lareinedesneiges` without copying the keys. Their words have separate postings but share the vocabulary, trigram index and typo handling. Acronyms are never generated from alternatives; short Latin alternatives (five letters or fewer without spaces, such as `H.O.T.D.`) use the acronym band instead. Very short Latin alternatives are not indexed.

Aliases are search keys only: results keep the canonical ID and name, and each IMDb ID appears once however many names match. Shared normalization in `internal/textnorm` folds accents and letters such as ß, æ, œ, ł, š and ř, and keeps non-Latin scripts. Catalogue building and search use it identically.

### Request limits

Input is limited to 120 Unicode characters, 16 words and eight looked-up terms (words of three or more characters other than articles); results to 20 per HTTP request. Prefix expansion, trigram scans, fuzzy word checks, posting lists and simultaneous searches have explicit limits. The request context bounds fuzzy work and the service limits matching concurrency.

## Catalogue preparation

Three catalogue profiles bound selection: Compact (25k movies + 25k series, minimum 250 votes), Standard (100k + 100k, minimum 25) and Expanded (250k + 250k, minimum 5). Standard is the new-setup default. These are independent per-type ceilings; actual eligible counts can be lower. Legacy snapshots without a profile are treated as Compact.

The data builder streams the same compressed IMDb files for every profile, excludes individual episodes and adult-marked content, and stores a compressed local JSON snapshot. When a language is selected for alternative titles, a third stream (`title.akas`) is read after title selection. A row is rejected after parsing only its ID unless that ID is already selected. Remaining rows are filtered by language and region. IMDb mostly leaves the language as `\N` and gives an ISO-style region, so each language maps to regions where it is the usual language. Multilingual regions such as CA, BE and CH count only when the row names the language. Working, original, literal, promotional and "version" rows are dropped. The rest are ranked (IMDb display title first), deduplicated after normalization, capped at four per title, and capped per size: Compact 75k, Standard 200k, Expanded 200k, keeping the most-voted titles. Expanded's extra titles are low-vote and rarely have regional titles.

Snapshots gain optional `aliases`, `aliasLanguage` and `aliasRegion` fields, and the snapshot version stays 1. Older snapshots load with alternative titles off. Older builds ignore the fields and keep working. A snapshot naming an unknown language, for example from a newer build, still loads. The language selection is applied like a size change. It persists only with a successful replacement snapshot, and a build request that omits it keeps the active choice. A refresh builds a replacement separately and swaps the serving index only after a valid snapshot is saved. Active and building profiles remain separate in status; the successful snapshot persists its profile. The previous snapshot supports recovery after interruption.

The loader accepts at most 500,000 titles, 600,000 alternative titles (eight per title) and 128 MiB of decompressed JSON; ratings parsing is also bounded. The executable retains its 256 MiB Go soft memory target and two Go execution threads. The target is not a process memory ceiling: live refresh temporarily holds the old and new index and can exceed it. See [measured resource use](PERFORMANCE.md).

## Storage

Settings, update state, the private control key and, by default, the catalogue live in the OS configuration folder (`os.UserConfigDir()/StremioSmartSearch`, or `--data-dir`). The catalogue is `catalog.json.gz`. Saving temporarily renames the old file to `catalog.json.gz.previous`; the loader can recover from it after an interrupted replacement. A successful save removes it, so it is not a permanent backup.

A custom catalogue folder is recorded in `storage.json` in the settings folder. Settings never move, so launches using that same data folder and port can authenticate and reuse the helper. A move copies the catalogue through temporary names, records the new folder, swaps it in and only then removes the old copies. It is refused during a build and checks free space first.

Storage figures are read on request (two file sizes and a walk of the small settings folder), with no background bookkeeping. The Memory value is `runtime.MemStats.Sys - HeapReleased`, an estimate of memory retained by the Go runtime rather than the OS's total process working set. The `/api/storage` routes have the same loopback, Host, Origin and control-key checks as the rest of `/api/*`.

## Stremio protocol

- `/manifest.json`: advertises two search-only catalogs, `movie/smart-search` and `series/smart-search`.
- `/addon-logo.png`: serves the embedded 256×256 addon logo independently of settings or catalogue setup. Its manifest URL uses the request host/port and direct TLS state; forwarded scheme headers are not interpreted.
- `/catalog/{type}/smart-search/search={query}.json`: returns standard metadata previews with stable IMDb IDs.
- No `stream` resource; existing addons handle playback.
- No account authentication, watch-activity events or library events.
- Extra values are decoded exactly once from the escaped URL path; punctuation in a query cannot become a route separator.

Protocol responses include CORS headers for Stremio clients. `/api/*` management routes do not expose CORS and require a loopback peer, an expected Host and a matching browser Origin when present. Processes/scripts can authenticate with the private key in `control.key`.

**Unreleased settings-session hardening on main:** browser requests can instead use a settings session. The settings page never contains the key: Smart Search opens it with a one-time code (single use, two minutes), which the page swaps for an HttpOnly, SameSite=Lax cookie derived from the key; API calls from the page must also send an `X-Smart-Search-Session` header, which cannot be sent cross-site without a CORS preflight. A browser without a session gets a short page with no data. The settings page rejects framing and uses a content security policy with a fresh nonce per page. No arbitrary local path or external URL can be executed through a search request.

## Lifecycle

The launcher starts the companion and opens the client chosen in setup. With no saved preference, it opens the registered Stremio URL handler. An authenticated existing instance is reused instead of starting another engine. The service remains available without polling Stremio's processes. The browser page may be closed. Quit and OS shutdown cancel any active data preparation.

Automatic catalogue updates run inside the helper (`internal/app/updates.go`); there is no separate service. One goroutine wakes three minutes after start-up and then every 30 minutes. A wake-up that is not due compares a few persisted timestamps and does no I/O. When the interval has passed since the last success (and no backoff or six-hour re-check spacing applies), it sends conditional `HEAD` requests for the dataset files and, if a file changed or the server gives nothing to compare, starts the same build as a manual refresh.

With alternative titles on, a routine automatic build skips `title.akas` while the last one is under seven days old and copies aliases by type and ID from the current snapshot (`catalog.CarryAliases`, which decodes only IDs and aliases); an unreadable snapshot falls back to downloading it.

User settings (`updates.json`) are kept apart from bookkeeping (`update-state.json`: last success, last check and result, failures, retry time, an in-progress marker and per-file `ETag`, `Last-Modified` and size, and when `title.akas` was last downloaded). Persisted times in the future are clamped to now.

On Windows the machine-wide connection cost comes from the Network List Manager (`INetworkCostManager`) through COM; other systems report it as unknown. See [CATALOGUE.md](CATALOGUE.md#automatic-updates) for the user-facing behaviour.

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
The listen address is a launch option, not a saved setting. A `--catalog` run
disables scheduled updates for that run so the imported catalogue is not
replaced automatically.

## Performance claims

Measure cold startup, catalogue preparation, idle CPU, total process memory and latency against real title data. A small-fixture microbenchmark is useful for regressions but is not a substitute for an actual catalogue or device test. Cross-compilation does not establish ARM or TV performance. No zero-overhead or perfect-matching guarantee is made.
