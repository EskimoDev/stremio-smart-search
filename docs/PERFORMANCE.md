# Catalogue sizes and measured resource use

Standard is the default for new setups. On this snapshot it provided 3.7 times
the Compact coverage, using about 196 MiB of RAM after startup. Expanded provided
7.8 times the coverage and used about 276 MiB. Compact remains available for
smaller devices. Existing installations keep their size until you rebuild it.

These measurements come from one Windows x64 PC: Ryzen 5 5600X, 32 GiB RAM,
Go 1.27.3, two Go execution threads and the helper's unchanged 256 MiB Go soft
memory target. The target is not a hard process-memory ceiling. These are not
measurements of macOS, Linux, ARM or TV hardware.

## Search and normal memory

| Size | Actual movies | Actual series | RAM after startup | Median request | 95th percentile | 99th percentile |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact | 25,000 | 25,000 | 65 MiB | 0.33 ms | 0.62 ms | 1.18 ms |
| Standard | 100,000 | 84,588 | 196 MiB | 0.66 ms | 1.46 ms | 2.39 ms |
| Expanded | 250,000 | 140,449 | 276 MiB | 1.43 ms | 9.86 ms | 17.87 ms |

RAM is the median process working set after three seconds idle following ordinary
snapshot load, without a forced garbage collection or working-set trim. Across
three runs it ranged from 64–105 MiB for Compact, 195–198 MiB for Standard and
272–279 MiB for Expanded. Garbage collection, recent searches and the OS affect
memory use; these figures are not ceilings. Browser and Stremio memory are excluded.

Each run made 750 real loopback HTTP requests: 15 fixed queries repeated 50 times,
mixing exact titles, acronyms, typos, common words, accents, non-Latin text and
no-result queries. The table gives medians across three runs. Timings include
HTTP and JSON handling, but exclude artwork, online metadata, playback and other
search addons. They do not predict the entire Stremio results page's loading time.

## CPU

| Size | Mean CPU per request | Quiet CPU sample |
| --- | ---: | ---: |
| Compact | 0.50 ms | 0.000 CPU seconds / 10 seconds |
| Standard | 0.85 ms | 0.000 CPU seconds / 10 seconds |
| Expanded | 4.02 ms | 0.000 CPU seconds / 10 seconds |

Active CPU includes both the helper and the benchmark HTTP client in one process.
CPU milliseconds are not elapsed time: two threads can work concurrently. The
quiet sample runs after the search workload and a three-second settling period,
with no browser page polling the helper. Zero means no increase detected by the
Windows process counter during that sample, not literally zero work forever.
Expanded recorded about 0.11–0.13 CPU seconds in the initial three-second window
immediately after startup, before settling.

## Disk, preparation and refresh

| Size | Stored catalogue | Load + index build | First preparation, cached sources | Live refresh, cached sources | Peak RAM during refresh |
| --- | ---: | ---: | ---: | ---: | ---: |
| Compact | 1.28 MiB | 0.45 s | 7.95 s | 8.54 s | 162 MiB |
| Standard | 4.70 MiB | 1.81 s | 9.40 s | 13.96 s | 279 MiB |
| Expanded | 9.82 MiB | 4.45 s | 10.96 s | 32.84 s | 539 MiB |

Load times use a warm OS file cache; this is not a cold-disk or reboot benchmark.
Preparation and refresh are one run per profile and exclude the network download.
Refresh includes parsing, saving and replacing the index while the previous index
continues answering requests. The test checked successful HOTD results throughout
each refresh. Expanded's roughly 540 MiB refresh peak is temporary and matters
when choosing a size for a small device.

All sizes download the same source files: **227.2 MiB compressed** in this snapshot.
Expanded does not multiply that download. Future source sizes and download times
vary. The normal helper streams the files and does not retain them. A previous
compressed catalogue can add roughly one extra catalogue's disk space. Updates
run when you request preparation or refresh, not on every startup or search.

## Coverage and ranking

Compact retains up to 25,000 movies and 25,000 series with at least 250 IMDb votes.
Standard retains up to 100,000 of each with at least 25 votes. Expanded retains up
to 250,000 of each with at least 5 votes. These are ceilings; actual qualifying
counts can be lower. Adult-marked titles, individual episodes and unsupported
types are excluded. Unrated and below-threshold titles remain absent. All Compact
IDs were present in both larger test snapshots.

The expected HOTD, House of the Dragon typo, Interstellar typo, Breaking Bad typo
and Game of Thrones results remained in the first 20 matches for every profile.
More coverage can make abbreviations and typos more ambiguous. For example, the
larger snapshot includes a separate movie named “Interstelar”: exact-title ranking
puts it ahead of the fuzzy Interstellar match, which remains in the results.
The benchmark checks presence rather than requiring a fuzzy match to outrank a
genuine exact title.

## Alternative titles

Alternative titles are off by default and cost nothing when off. These measurements use French titles (`-aliases fr`)
on the same Windows x64 PC (Ryzen 5 5600X), with Go 1.27.0, two Go threads and the 256 MiB soft target, on IMDb data
downloaded 2026-11-01. Ranges cover three fresh `measure` runs; preparation and refresh are one run each and
exclude the download.

| Size | French titles kept | Stored catalogue | RAM after startup | Median request | 95th percentile | 99th percentile |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact, off | | 1.34 MB | 70 MiB | 0.36 ms | 0.65 ms | 1.1–1.4 ms |
| Compact, French | 25,394 | 1.68 MB | 78–80 MiB | 0.37 ms | 0.66–0.74 ms | 1.2 ms |
| Standard, off | | 4.93 MB | 197–200 MiB | 0.71–0.77 ms | 1.57–1.78 ms | 2.8–3.2 ms |
| Standard, French | 55,722 | 5.70 MB | 217–231 MiB | 0.71–0.78 ms | 1.45–1.69 ms | 2.7–3.5 ms |
| Expanded, off | | 10.29 MB | 299–301 MiB | 1.5–1.6 ms | 11–15 ms | 19–24 ms |
| Expanded, French | 75,501 | 11.38 MB | 306–309 MiB | 2.3–2.5 ms | 8–14 ms | 12–30 ms |

| Size | First preparation, off → French | Live refresh, off → French | Peak RAM during refresh, off → French |
| --- | ---: | ---: | ---: |
| Compact | 7.4 → 16.9 s | 7.6 → 19.0 s | 165 → 182 MiB |
| Standard | 8.2 → 18.8 s | 15.1 → 27.8 s | 291 → 310 MiB |
| Expanded | 8.7 → 19.9 s | 27.4 → 45.9 s | 565 → 595 MiB |

- **Download:** `title.akas.tsv.gz` was 520 MB (520,070,554 bytes), against 238 MB for ratings and basics together.
  Enabling alternative titles roughly triples each preparation's download. It is streamed and never stored.
- **Preparation:** streaming and filtering the alternative-title file adds about 10–11 s of processing per build.
- **Memory:** Compact and Standard add about 8–10 MiB and 20–30 MiB of RAM after startup. Expanded adds about 7 MiB,
  but it already runs above the soft target, which makes its RAM figures less sensitive.
- **Latency:** Compact and Standard search times are unchanged within run-to-run variation. Expanded's median rose
  from about 1.55 to 2.4 ms, and its tail varied too much between runs to call. Its heap (261–277 MiB) is already
  past the 256 MiB soft target, so any added memory means more garbage collection.
  The settings page therefore recommends Standard when alternative titles are chosen with Expanded.
- **Title counts:** real counts per language are far below the per-size budgets (Compact 75k, Standard 200k,
  Expanded 200k), so no titles were dropped. On Standard: English 54,950, French 55,722, German 55,288,
  Japanese 55,910, Spanish 84,843 (Spain only: 48,906).
- **Quality:** *la reine des neiges*, *la rene des neige* (→ Frozen) and *le trone de fer* (→ Game of Thrones)
  returned the expected title first. With English titles, IMDb lists "H.O.T.D." for Highschool of the Dead, which first
  outranked House of the Dragon for "HOTD". Short Latin alternative titles (five letters or fewer) now rank with
  acronyms, so popularity decides. That fix is covered by a regression test but has not been re-measured on this PC.

Synthetic tests during development, at the same title counts but with about 0.9 alternative titles per title, put
the index cost at about 100–160 bytes per alternative title. The real counts above are lower than that synthetic load.

To reproduce, add `title.akas.tsv.gz` to the data directory and pass `-aliases fr` to every phase (any listed
language works, optionally with a region such as `es:ES`). Snapshots are stored as `<size>-<language>.json.gz`. The
timed 15-query workload is unchanged, so runs with and without alternative titles are comparable. Language-specific
checks are reported once in `cases` and are not timed.

## Reproduce

The [raw measurements](benchmarks/windows-2026-11-01.json) include every startup
sample, test case, source byte count and checksum. No production catalogue,
account data, user searches or local user paths are included.

1. Use Go and download title.ratings.tsv.gz and title.basics.tsv.gz (plus title.akas.tsv.gz for alternative titles) from
   [IMDb's dataset server](https://datasets.imdbws.com/) into an explicitly chosen
   directory outside this repository. Observe [IMDb's dataset terms](https://developer.imdb.com/non-commercial-datasets/).
   The benchmark caches the compressed sources to reuse an identical snapshot;
   the installed helper does not cache them.
2. Build the developer utility and run the phases below. Windows gets native
   process working-set, peak-working-set and CPU counters. Other platforms
   currently provide heap/wall-time measurements only; their process-counter
   values are unavailable.
3. Repeat measure three times per profile in fresh processes, without other
   benchmark phases running simultaneously. Keep the same sources, Go build and
   memory target. Use steady-idle for the additional quiet sample. Do not publish
   the raw sources, generated snapshots or controls directories.

```powershell
go build -trimpath -o catalogue-benchmark.exe ./tools/benchmark
$data = "C:/Data/SmartSearchBenchmark"
foreach ($size in @("compact", "standard", "expanded")) {
  ./catalogue-benchmark.exe -phase prepare -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase measure -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase refresh -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase measure -steady-idle 10s -profile $size -data-dir $data
}
```

This benchmark executable is a developer utility, not an installed background
component. Release packages contain the helper, local setup assets and notices;
they do not contain raw source datasets or generated production catalogues.
