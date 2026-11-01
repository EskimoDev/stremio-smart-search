# Performance

Memory, speed and disk use for each catalogue size, measured on real IMDb data.
Standard is the default for new setups. Existing installations keep their size
until you rebuild.

All figures come from one Windows x64 PC: Ryzen 5 5600X, 32 GiB RAM, two Go
execution threads and the helper's 256 MiB Go soft memory target. That target is
not a hard ceiling on process memory. macOS, Linux, ARM and TV hardware have not
been measured with a full catalogue.

## Results

Measured 2026-11-01 at commit 804ddcf with Go 1.27.0, on the IMDb snapshot of
2026-11-01. Each phase ran in a fresh process. Memory and latency are medians of
three runs; preparation and refresh are single runs with cached sources. Two other
local helpers were running at the time, so treat small differences as noise.

| Size | Titles | RAM after startup | Median request | 95th percentile | 99th percentile | CPU per request |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact | 50,000 | 23 MiB | 0.27 ms | 0.44 ms | 0.66 ms | 0.40 ms |
| Standard | 184,606 | 53 MiB | 0.36 ms | 0.60 ms | 0.82 ms | 0.46 ms |
| Expanded | 390,470 | 97 MiB | 0.42 ms | 0.77 ms | 1.15 ms | 0.50 ms |

| Size | Load + index build | Preparation (download excluded) | Live refresh (download excluded) | Peak RAM during refresh |
| --- | ---: | ---: | ---: | ---: |
| Compact | 0.21 s | 3.55 s | 3.82 s | 86 MiB |
| Standard | 0.82 s | 4.38 s | 4.74 s | 224 MiB |
| Expanded | 1.85 s | 4.60 s | 6.16 s | 275 MiB |

The quiet CPU sample, 10 seconds after the searches had finished, was zero for
every size. All fixed search cases passed in every run.

**What the numbers mean.** RAM is the process working set three seconds after
startup. Normal startup already calls `debug.FreeOSMemory()` after loading the
catalogue; the benchmark adds no further forced collection at sampling time.
It is not a ceiling: recent searches,
garbage collection and the OS all move it. Each run made 750 real loopback HTTP
requests, 15 fixed queries repeated 50 times, mixing exact titles, acronyms, typos,
common words, accents, non-Latin text and no-result queries. Timings include HTTP
and JSON handling, but not artwork, playback or other addons, so they do not predict
how long Stremio's whole results page takes. CPU per request also counts the
benchmark's own HTTP client. Load times use a warm file cache. Preparation and
refresh exclude the network download. During a refresh the previous index keeps
answering searches, which is why memory briefly peaks.

## Download and disk

Every size downloads the same two IMDb files, **227 MiB compressed** in the
2026-11-01 snapshot, so a larger size does not mean a larger download. The helper
streams them and does not keep them. The stored catalogue is small: about 1.3 MiB
for Compact, 4.7 MiB for Standard and 9.8 MiB for Expanded. A copy of the previous
catalogue is used temporarily during replacement, so allow about twice that
while saving. A successful save removes the recovery copy.

## Alternative titles

Alternative titles are off by default and cost nothing when off. Turning them on
adds IMDb's `title.akas` file, 520 MB in this snapshot, to each preparation. It is
streamed and never stored.

Measured with French titles in the same run as the tables above:

| Size | French titles kept | RAM after startup | 95th percentile | Preparation (download excluded) | Peak RAM during refresh |
| --- | ---: | ---: | ---: | ---: | ---: |
| Standard | 55,722 | 58 MiB | 0.52 ms | 11.4 s | 256 MiB |
| Expanded | 75,501 | 105 MiB | 0.86 ms | 12.8 s | 294 MiB |

Reading `title.akas` accounts for about 7 s of each preparation. Real counts per
language sit well below the per-size budgets, so no titles were dropped. On
Standard: English 54,950, French 55,722, German 55,288, Japanese 55,910, Spanish
84,843 (Spain only: 48,906).

## Index and preparation rewrite

The 0.1.0-preview.3 release rebuilt the search index around sorted 32-bit arrays
instead of Go maps, packed acronyms into integers and made each search reuse its
working memory. Preparation now reads each dataset on its own thread and parses
rows without allocating. On the same PC and IMDb data, before (2026-10-31) and after:

| Size | RAM after startup | 95th percentile | CPU per request | Load + index build | Live refresh | Peak RAM during refresh |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact | 65 → 23 MiB | 0.62 → 0.44 ms | 0.50 → 0.40 ms | 0.45 → 0.21 s | 8.5 → 3.8 s | 162 → 86 MiB |
| Standard | 196 → 53 MiB | 1.46 → 0.60 ms | 0.85 → 0.46 ms | 1.81 → 0.82 s | 14.0 → 4.7 s | 279 → 224 MiB |
| Expanded | 276 → 97 MiB | 9.86 → 0.77 ms | 4.02 → 0.50 ms | 4.45 → 1.85 s | 32.8 → 6.2 s | 539 → 275 MiB |

The old Expanded index sat above the 256 MiB soft target, so garbage collection
ran almost constantly, and that caused its long tail. **Search results did not
change**: across about 96,000 generated queries per run, every result list matched
the previous build exactly, and every snapshot was byte-for-byte identical apart
from its build time. That check ran on synthetic, IMDb-shaped data from
`tools/synthetic`; its [raw results](benchmarks/linux-synthetic-2026-11-01.json)
are kept for reference.

## Reproduce

1. Download `title.ratings.tsv.gz` and `title.basics.tsv.gz` (plus `title.akas.tsv.gz`
   for alternative titles) from [IMDb's dataset server](https://datasets.imdbws.com/)
   into a directory outside this repository, under
   [IMDb's dataset terms](https://developer.imdb.com/non-commercial-datasets/).
2. Build the benchmark and run the phases below. Windows reports working set, peak
   working set and CPU. Linux reports RSS, peak RSS, anonymous resident memory and
   process CPU; those memory counters differ from Windows' working set. Other
   platforms currently report Go heap and wall time without process counters.
3. Run `measure` three times per size in fresh processes, with nothing else
   benchmarking at the same time. Add `-aliases fr` (or another language, optionally
   with a region such as `es:ES`) to every phase for alternative titles. Do not
   publish the raw sources or generated snapshots.

```powershell
go build -trimpath -o catalogue-benchmark.exe ./tools/benchmark
$data = "C:/Data/SmartSearchBenchmark"
foreach ($size in @("compact", "standard", "expanded")) {
  ./catalogue-benchmark.exe -phase prepare -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase measure -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase refresh -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase measure -steady-idle 10s -profile $size -data-dir $data
  ./catalogue-benchmark.exe -phase accuracy -profile $size -data-dir $data
}
```

Without the IMDb files, `go run ./tools/synthetic -out <dir>` writes synthetic ones
(about 580 MB compressed) that the same phases accept; label any results from them
as synthetic. `-cpuprofile` writes a Go CPU profile of any phase, and
`-phase accuracy -dump <file>` records every generated query and its results so two
builds can be compared for identical output.

## Measurement records

The current Windows tables summarize the measurements recorded in
[commit b438f3b](https://github.com/EskimoDev/stremio-smart-search/commit/b438f3ba0d0fcc1d82a37e065778322efa660f3e).
Their individual post-rewrite reports are not checked into this repository.
The retained JSON files support these earlier comparisons:

- [Windows before the rewrite](benchmarks/windows-2026-11-01.json): startup samples,
  request timings, source byte counts and checksums for the earlier index.
- [Synthetic Linux before and after](benchmarks/linux-synthetic-2026-11-01.json):
  invented IMDb-shaped data, used to compare the index implementations. These
  are not real-catalogue Windows results.

Use the reproduction steps above to collect a new set of reports. Keep the raw
datasets and production catalogues outside the repository. The benchmark is a
developer tool and is not shipped with the helper.
