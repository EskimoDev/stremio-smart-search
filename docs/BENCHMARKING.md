# Benchmarking

`sss-bench` measures a build of Smart Search and compares two builds. It runs
offline, needs no IMDb data, and leaves an installed helper alone. Use it to answer:

- How fast is this build, and how much memory does it use?
- Did this change make search faster, slower or less accurate?
- Are two results comparable at all?
- What could not be measured on this platform?

It is a developer tool. Nothing here is part of the installed app.

## Quick start

From the repository root, with Go installed:

```sh
go run ./tools/sss-bench run --profile quick
```

Progress goes to stderr. The result is saved beside the repository in
`../sss-bench-results/` as three files: `.json` for tools, `.md` for people and
`.bench` for [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

| Profile | Use it for | Catalogue | Repetitions | Adds | Expected cost |
| --- | --- | ---: | ---: | --- | --- |
| `quick` | Small changes | 20,000 titles | 4 | Search latency, basic memory, curated quality | About 40 seconds |
| `standard` | Pull requests | 50,000 titles | 6 | Throughput, sustained memory, held-out quality suites | About 3 minutes |
| `full` | Releases and big changes | 200,000 titles | 8 | Catalogue preparation, rebuild, live refresh, 8 clients, generated quality | About 20 minutes |

`go run ./tools/sss-bench metrics` lists every metric and when it applies; add
`--json` for tools. Costs are for one build on a recent desktop, and an `ab` run
is about twice that. Temporary disk use stays under about 100 MB.

## Compare two versions

Interleaved comparison is the method to trust. It alternates the two builds in one
session, as A B B A, so slow drift in the machine affects both sides equally.

```sh
git worktree add ../sss-base origin/main
go run ./tools/sss-bench ab --baseline ../sss-base --candidate . --profile standard
git worktree remove ../sss-base
```

One harness builds and measures both trees, so both get the same workload and the
same definitions. Neither tree is modified, and the baseline can predate the
harness: `v0.1.0-preview.3` works. A side can also be a prebuilt binary, although
its direct-engine metrics are then [unavailable](#what-a-prebuilt-binary-cannot-show).

Comparing two saved results also works, for instance across days:

```sh
go run ./tools/sss-bench compare --baseline before.json --candidate after.json
```

### Reading a comparison

Each metric gets one verdict:

| Verdict | Meaning |
| --- | --- |
| `regressed`, `improved` | Statistically significant and larger than the metric's practical floor |
| `no_significant_change` | The two sides overlap. This is absence of evidence, not proof of equality |
| `below_threshold` | Probably real, but too small to matter |
| `insufficient_samples` | Fewer than four repetitions on a side. The change is shown without a verdict |
| `not_comparable` | Different hardware, catalogue, workload or counter. No percentage is shown, because any number would mislead |
| `missing`, `failed`, `unavailable` | Absent on a side, broken, or impossible on this platform. Treat these as unknown, not as unchanged |

Relative metrics show percentage change. Shares such as top-1 accuracy show
percentage points.

Search quality has no significance test, because ranks are deterministic: any
changed case is a real change, and the report names it. The headline only counts
headline metrics; per-class latency appears in the full table, since testing many
small slices would raise false alarms.

## Exit codes and output for agents

Files and exit codes are the interface. Do not parse terminal text.

| Code | Meaning |
| ---: | --- |
| 0 | Finished. For `compare`: no regression found |
| 1 | The harness could not produce a result |
| 2 | Bad flags or unusable input. Nothing was measured |
| 3 | Finished, but part of the measurement failed |
| 4 | `compare` or `ab` found a regression |
| 5 | Results are not comparable |

`go run` turns every non-zero code into 1, so build first when the code matters:

```sh
go build -o sss-bench.exe ./tools/sss-bench
./sss-bench.exe run --profile quick --json --quiet --out ../results/mine
```

`--json` prints the whole result on stdout. Results carry `schema`,
`schema_version`, a top-level `status` (`complete`, `partial` or `failed`), and for
every metric its `status`, `unit`, `samples` and exact `definition`; a metric that
could not be measured says why in `reason`. Comparisons carry a top-level `verdict`
(`regressed`, `improved`, `no_significant_change` or `not_comparable`) and
`incomplete`. Files are refused when their schema is unknown, so nothing is
silently misread. Existing output is not replaced without `--force`.

## What is measured

Every timing is taken in a fresh process per repetition, because that is the true
unit of independence. Values are medians across repetitions.

| Group | Metrics |
| --- | --- |
| Startup | Process start to ready, and to the first search response, with a warm file cache |
| Search over HTTP | p50, p95, p99 and mean; cold (first pass in a process) and warm; by query class and length; requests served per second at each concurrency; share shed |
| Search in the engine | The same latencies with no HTTP, plus allocations per search and searches per second |
| CPU | Application CPU per request, excluding the benchmark client, and when idle |
| Memory | After load, idle, after searching, peak, startup overhead, and growth under sustained searching. Go heap is reported apart |
| Catalogue | File read, load and index build, separately for wall and CPU time; rebuild; preparation and live refresh without the download; stored size; titles indexed |
| Quality | Top-1, top-5, found in 20, coverage and false positives for each suite and category |

**HTTP and engine numbers are labelled separately on purpose.** HTTP is what Stremio
sees, and most of it is HTTP handling; a small engine change can vanish in it. The
engine path shows the change itself.

**Concurrency has a trap this harness guards against.** The server has two search
slots. A third simultaneous request gets HTTP 200 with an empty list, immediately.
Counting those as successes would report a load test as very fast, so shed requests
are recognised by `cacheMaxAge: 0`, counted, and kept out of latency and throughput.

**Memory counters are not interchangeable**, so each metric names its counter, and
`compare` refuses to mix them:

| Quantity | Windows | Linux | macOS |
| --- | --- | --- | --- |
| Resident | Working set | `VmRSS` | RSS from `ps` |
| Private | `PrivateUsage` (commit charge) | `RssAnon` | Not available |
| Peak | `PeakWorkingSetSize` | `VmHWM` | `ru_maxrss` at exit |
| Go heap | `runtime.MemStats.HeapAlloc` | Same | Same |

`PrivateUsage` counts committed memory, resident or not, so it is usually far
larger than `VmRSS`-style numbers; never compare it with `RssAnon`.

### Search quality

The harness reuses the curated files in `testdata/quality` and the scoring in
`internal/quality`, so its numbers follow the rules in
[Search quality](SEARCH-QUALITY.md). Expected answers are never changed to suit a
result. Three things stay apart in every table:

- **Retrieval**: the expected title appears in the first 20 results.
- **Ranking**: it appears first, or in the first five.
- **Absence**: it is not in this catalogue at all. That lowers coverage and is
  never counted as a miss.

Every case lands in exactly one count, so the denominators add up. Scores come from
the engine probe, which has no time budget, so a busy machine cannot change them.
Each run searches twice in fresh processes and reports an error if the results
differ. It also replays the curated queries over HTTP and warns about any
difference, which catches the server changing what a user sees.

Category groups map onto the case file as follows: exact titles (`exact`,
`original`); typing errors (`fuzzy`, `transposition`); `phonetic`; abbreviations
(`acronym`); joined words (`joined`, `punctuation`); other languages (`alias`,
`alias-fuzzy`, `accent`, `non-latin`); ambiguity (`short`, `similar`, `year`); and
`negative`.

## The catalogue under test

By default the harness generates a fixture on the spot. It contains every title the
quality cases expect, a decoy for each forbidden result, and invented titles up to
the profile's size, from the same generator as `tools/synthetic`. The same harness
always produces the same content, and nothing is downloaded or stored in the
repository.

**Fixture quality scores detect ranking changes between builds. They are not
accuracy on real titles.** The fixture lacks the real look-alikes that make search
hard, and its popularity figures are invented, so its scores run higher than the
real ones in [Search quality](SEARCH-QUALITY.md). The report says so too.

To measure a real catalogue, point at any existing snapshot:

```sh
go run ./tools/sss-bench run --profile standard --catalog path/to/expanded.json.gz
```

The file is copied into the temporary folder, measured there and confirmed unchanged
afterwards. You can use the catalogue from your own installation this way; it is
read, never written. `--imdb-dir` adds preparation and refresh on real files with
the `full` profile, and is only read from. The harness never downloads anything.
Reports on a real catalogue can contain title names, so keep them out of this
repository, as with [other IMDb-derived output](../DATA.md).

## Isolation guarantees

- Each run works in a new temporary folder and removes it afterwards. Removal
  requires a marker holding that run's own ID, so it cannot delete anything else.
- The application runs on a free port and with a private `--data-dir`. Port 11479 is
  never used. A request can only succeed against the process the harness started,
  because its key is read from that private folder.
- A running helper is left alone. The harness checks whether the default port is
  answering, says so in the report, and does nothing else to it.
- Child processes get `HOME`, `APPDATA` and the like pointing into the temporary
  folder, so even a path that ignores `--data-dir` cannot reach your installation.
  Their proxy settings point at a closed port, so an accidental download fails
  immediately. Go runtime overrides such as `GOGC` are removed and reported.
- `--catalog` makes automatic updates impossible for that process, and the refresh
  probe replaces the download functions outright.
- A work folder inside your installed data folder is refused. Results never replace
  an existing file without `--force`.
- Reports contain no host name, user name or local path, so they are safe to paste
  into a public pull request.

## Method

- **Timer.** `time.Now` on Windows advanced only every 300 µs when measured here,
  about the length of a search. Timings use `QueryPerformanceCounter`, and each
  result records the timer resolution it saw.
- **No self-interference.** The HTTP client is a separate process from the
  application, so client work never lands in the application's memory or CPU counters.
  Fixture generation, process startup and connection setup all finish before any timed
  request. The first pass is reported as cold and warmups are discarded.
- **Percentiles** are nearest-rank, so each is a value that was actually observed. A
  p99 from fewer than 100 observations, or a p95 from fewer than 20, is flagged.
- **Significance** is a two-sided exact Mann-Whitney U test at alpha 0.05 across
  fresh-process repetitions, the test benchstat uses, with ties handled exactly. It
  assumes no distribution, which suits latency. Four repetitions per side are the
  minimum at which p below 0.05 is possible.
- **Practical floors** come on top: a change must also exceed a per-metric floor, for
  example 5% and 10 µs for warm latency, or 3% and 1 MiB for memory. Floors are not
  stored in results, so they can be revised without measuring again.
- **Environment.** Children run with `GOMAXPROCS=2` and a 256 MiB soft limit, as the
  shipped helper sets for itself.

benchstat gives a second opinion on any pair of results:

```sh
go run golang.org/x/perf/cmd/benchstat@latest before.bench after.bench
```

## Platform coverage

The `Benchmark` workflow runs on request, and on pull requests that touch the harness.
It is separate from `Check addon`, so a benchmark problem cannot be mistaken for a
functional failure, and it applies no timing thresholds: hosted runners share
hardware, so it proves the harness works on each platform and keeps reports as
artifacts. Do not compare timings between runners. Its summary lists which platforms
really ran.

| Platform | Runtime status |
| --- | --- |
PLATFORM_TABLE

Hosted runs are real executions on shared machines. They verify behaviour, not
performance. See [Platform testing](PLATFORM-TESTING.md) for the app itself.

### What a prebuilt binary cannot show

`--binary` has no source to build a probe from, so direct-engine metrics, Go heap and
catalogue operations are reported as unavailable, and quality comes over HTTP.
`compare` will not treat an HTTP-scored suite as equivalent to an engine-scored one,
because the server's 100 ms search budget can change results on a busy machine.

## How well it detects change

VALIDATION_SECTION

## Limitations

- **Hosted runners** have noisy neighbours. Their timings verify nothing about
  performance.
- **Laptops** throttle. Plug in, close other work and prefer `ab`.
- **Windows process CPU** advances about every 15.6 ms. Short operations show 0 or
  one tick; their floors allow for it, and CPU per request is measured across
  thousands of requests. Defender can also slow the first launch of a new binary,
  which the harness's `--version` check absorbs.
- **File cache** is warm: startup and load times do not show a cold disk. Forcing a
  cold cache needs administrator rights and is not attempted.
- **Network** is not measured, because nothing here uses it. Preparation and refresh
  exclude the download.
- **The fixture is not IMDb.** Its speed and memory are in the right range for a
  catalogue of its size, but use `--catalog` before quoting absolute numbers.
- **Regional titles** on the fixture use French only, so the 32 cases for other
  languages show as not applicable. Use a real snapshot built with that language.
- **macOS and Linux peaks** have caveats: on macOS `ru_maxrss` may include memory from
  before the child started, and macOS has no private-memory counter.
- **Memory growth** is an estimate from a short window, and the Go heap saws up and
  down within it. It flags a leak; it does not size one.
- **Repeated tests** at alpha 0.05 give false alarms in about 1 test in 20. The floors
  remove most, and the comparison states how many tests ran. Confirm a surprising
  single-metric result by repeating it.
- **Refresh** rebuilds titles only, without alternative titles, which need the large
  `title.akas` file.

## Changing the harness

Bump `measurementVersion` in `tools/sss-bench/schema.go` when a metric's meaning
changes, `workloadVersion` when the queries or fixture change, and `schemaVersion`
when the JSON changes shape. Each makes older results refuse comparison, which is the
intended behaviour. Add a new metric to `metricDefs` in `metrics.go`, and only there.

Probes in `tools/sss-bench/probes` are compiled into older source trees, so keep them
to the standard library, `internal/catalog`, `internal/search` and, for `catalogops`,
`internal/app`. If a probe stops compiling against an old tree, only its metrics are
lost, and the result says why.

Tests that start real processes need `SSS_BENCH_E2E=1`:

```sh
SSS_BENCH_E2E=1 go test ./tools/sss-bench/...
```

Do not commit generated reports. `tools/benchmark` remains for preparing real
snapshots and for reproducing the historical tables in [Performance](PERFORMANCE.md).

## Future extensions

- Score the fixture once per alias language, so every regional case runs offline.
- A pinned hardware runner, which would make trend tracking between commits meaningful.
- Sampled CPU profiles attached to a run, to explain a regression as well as find it.
- Cold-cache startup on machines where purging the file cache is allowed.
- Holm correction across headline metrics, if false alarms prove a problem in practice.
