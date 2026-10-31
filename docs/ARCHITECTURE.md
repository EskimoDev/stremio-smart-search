# Architecture

The application is a single Go executable with an embedded local setup page. There are no native media hooks, Stremio patches, browser extensions or runtime package downloads.

## Search

The read-only index contains exact normalized titles, joined-title forms, acronym variants, token postings and a trigram-to-vocabulary index. A fuzzy query first selects bounded candidate words and confirms them with bounded Damerau-Levenshtein distance, including adjacent transpositions. Query terms are intersected. Exact names outrank aliases and fuzzy hits. Vote counts rank otherwise similar matches.

Input is limited to 120 Unicode characters and eight terms; results to 20 per request. Prefix expansion, trigram candidate scans, fuzzy word checks, posting lists and simultaneous searches have explicit limits. The normal HTTP request loop never needs an external search call. A request context bounds fuzzy work and the service limits matching concurrency.

The default catalogue holds no more than 50,000 titles, split between movies and series. The data builder streams compressed IMDb files, excludes individual episodes and adult-marked content, and stores a compressed local JSON snapshot. A refresh builds a replacement separately and swaps the serving index only after a valid snapshot is saved. The previous snapshot supports recovery after interruption.

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
