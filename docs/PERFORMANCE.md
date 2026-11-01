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

## Reproduce

The [raw measurements](benchmarks/windows-2026-11-01.json) include every startup
sample, test case, source byte count and checksum. No production catalogue,
account data, user searches or local user paths are included.

1. Use Go and download title.ratings.tsv.gz and title.basics.tsv.gz from
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
