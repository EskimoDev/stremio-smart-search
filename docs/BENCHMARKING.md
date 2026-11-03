# Benchmarking

`sss-bench` measures a build of Smart Search and compares two builds. It runs
offline, needs no IMDb data, and leaves an installed helper alone. Use it to answer:

- How fast is this build, and how much memory does it use?
- Did this change make search faster, slower or less accurate?
- Are two results comparable at all?
- What could not be measured on this platform?

It is a developer tool. Nothing here is part of the installed app.

[Quick start](#quick-start) · [Compare versions](#compare-two-versions) · [Metrics](#what-is-measured) · [Platform coverage](#platform-coverage) · [Limitations](#limitations)

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
| `quick` | Small changes | 20,000 titles | 6 | Search latency, basic memory, curated quality | About 30 seconds |
| `standard` | Pull requests | 50,000 titles | 8 | Throughput, sustained memory, held-out quality suites | About 4 minutes |
| `full` | Releases and big changes | 200,000 titles | 10 | Catalogue preparation, rebuild, live refresh, 8 clients, generated quality | About 16 minutes |

`go run ./tools/sss-bench metrics` lists every metric and when it applies; add
`--json` for tools. Costs are for one build on a six-core desktop, and an `ab` run
is about twice that. Temporary disk use was 39 MB for a `full` run.

## Compare two versions

Interleaved comparison is the method to trust. It alternates the two builds in one
session, as A B B A, so slow drift in the machine affects both sides equally.

```sh
git worktree add --detach ../sss-base origin/main
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
| `insufficient_samples` | Too few repetitions to reach significance, so the change is shown without a verdict |
| `not_comparable` | Different hardware, catalogue, workload or counter. No percentage is shown, because any number would mislead |
| `missing`, `failed`, `unavailable` | Absent on a side, broken, or impossible on this platform. Treat these as unknown, not as unchanged |

Relative metrics show percentage change. Shares such as top-1 accuracy show
percentage points.

Only a small, fixed set of **primary metrics** decides the overall result: warm
p50 and p95 on both paths, startup, idle and peak memory, Go heap after indexing, load
and index time and, where the profile measures them, throughput at two clients,
sustained memory, preparation, refresh and refresh peak. Every other metric is a
**secondary hint**: shown, uncorrected and never decisive. Expect one or two hints even
on identical builds.

Search quality has no significance test, because ranks are deterministic: any
changed case is a real change, and the report names it.

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
| Environment | Machine-wide CPU use while the application is idle, which shows other work on the machine |

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
`original`); typing errors (`fuzzy`, `transposition`, `dropped-vowel`, `multi-typo`); `phonetic`; abbreviations
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
- **Priming.** The first launches of a new executable are not representative. On
  Windows 11 here they showed 48 MiB of working set against 11 MiB, four times the page
  faults and the same private memory. Running the file clears it; waiting does not.
  Every binary and probe is run several times, and one launch and pass discarded,
  before anything is timed.
- **Significance** is a two-sided exact Mann-Whitney U test at alpha 0.05 across
  fresh-process repetitions, the test benchstat uses, with ties handled exactly. It
  assumes no distribution, which suits latency. Primary metrics are Holm-corrected as a
  family, which holds the chance of even one false finding near alpha. Six repetitions
  per side are the fewest that can reach significance across `quick`'s ten primary
  metrics; below that a comparison reports too few samples.
- **Practical floors** come on top: a change must also exceed a per-metric floor, for
  example 5% and 10 µs for warm latency, or 3% and 1 MiB for memory. Floors are not
  stored in results, so they can be revised without measuring again.
- **Interference.** Machine-wide CPU is sampled while the application sits idle, and
  a result warns above 20%. An ordinary desktop read 8 to 12% here, one busy thread 17 to
  20%, and four about 40%. A lock detects a second `sss-bench`.
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

| Platform | Native runs of the harness | Not available there |
| --- | --- | --- |
| Windows x64 | Every profile on a Ryzen 5 5600X desktop; `quick` on a hosted runner | Nothing |
| Windows ARM64 | `quick` on a hosted runner (Cobalt 100) | Nothing |
| Linux x64 | `quick` on a hosted runner (EPYC 9V45) | Nothing |
| Linux ARM64 | `quick` on a hosted runner (Neoverse N2) | Nothing |
| macOS Apple Silicon | `quick` on a hosted runner (virtual M1) | Private memory, startup overhead, machine-wide CPU |
| macOS Intel | `quick` on a hosted runner (Core i7-8700B) | The same |

All six produced the same fixture fingerprint, workload ID and quality scores, so the
inputs really are reproducible across systems. Hosted runs are real executions on
shared machines: they verify behaviour, not performance. `standard` has also run on a Linux cloud VM against real Standard and Expanded catalogues; see [the preview.4 measurements](PERFORMANCE.md#preview4-on-a-linux-cloud-vm). `full` is recorded only on Windows. See [Platform testing](PLATFORM-TESTING.md) for the app itself.

### What a prebuilt binary cannot show

`--binary` has no source to build a probe from, so direct-engine metrics, Go heap and
catalogue operations are reported as unavailable, and quality comes over HTTP.
`compare` will not treat an HTTP-scored suite as equivalent to an engine-scored one,
because the server's 100 ms search budget can change results on a busy machine.

## How well it detects change

A harness earns trust by staying quiet when nothing changed and speaking up when
something did, so both were tested on Windows on the 5600X, one batch at a time.
Faults were injected into throwaway worktrees and never committed.

| Test | Result |
| --- | --- |
| Identical builds, `quick`, 8 runs | No false finding |
| Identical builds, `standard`, 3 runs | No false finding |
| Slowdown of about 40 µs per search, `quick` | Regression: engine p50 +100%, p95 +35% |
| The same slowdown, `standard` | Regression: engine p50 +69%, HTTP p50 +23%, throughput -18% |
| 30 MiB kept alive by the index, `quick` | Regression: idle memory +174%, peak +193%, Go heap +832% |
| Ranking bug: acronyms lose their weight, `quick` | Regression: two named cases worse, no primary timing change |
| `v0.1.0-preview.3` as the baseline, `standard` | Both probes built. 52 quality cases better; engine p50 +24%, HTTP p50 +12%, throughput -9%, the cost of what was added since |

Six of the eleven identical-build runs carried the busy-machine warning, because the
desktop was in use. They still raised no false finding, but the warning is the reason
to repeat a surprising result rather than trust it.

Three outside references agree:

- **benchstat** made the same significant-or-not call on all 78 metrics of two
  comparisons, with p-values differing only by its three-decimal rounding.
- **The preview.4 quality results, using its 288-case suite,** were reproduced exactly on a real Expanded catalogue
  of 390,470 titles: 222/231 first and 228/231 in five on `cases`, 22/30 and 24/30 on
  the held-out set, and 43/56 with 3 false positives on the validation set.
- **The recorded load time** of 1.85 s matched 1.87 s to ready on the same PC. Latency
  and memory ran higher than the preview.3 tables, as they should: the build and the
  query mix both differ, so those tables are not a like-for-like reference.

## Limitations

- **Hosted runners** have noisy neighbours. Their timings verify nothing about
  performance.
- **Laptops** throttle. Plug in, close other work and prefer `ab`.
- **Windows process CPU** advances about every 15.6 ms. Short operations show 0 or
  one tick; their floors allow for it, and CPU per request is measured across
  thousands of requests.
- **Process-to-process groups.** On the Windows desktop, HTTP p50 sat near 0.165 ms in
  some processes and 0.21 ms in others, whatever order they ran in. The cause was not
  established. It is why results rest on repeated fresh processes, and a reason to trust
  the engine path for small changes.
- **Sensitivity is not fully known.** The faults above were large. The smallest change
  each profile can detect was not measured, and `quick` flagged only the engine path for
  a slowdown that `standard` also caught over HTTP.
- **Interference checks** only sample the settle periods, so a burst at another moment
  goes unseen, and macOS has no machine-wide reading, so its warning cannot fire.
- **File cache** is warm: startup and load times do not show a cold disk. Forcing a
  cold cache needs administrator rights and is not attempted.
- **Network** is not measured, because nothing here uses it. Preparation and refresh
  exclude the download.
- **The fixture is not IMDb.** Its speed and memory are in the right range for a
  catalogue of its size, but use `--catalog` before quoting absolute numbers.
- **Regional titles** on the fixture use French only, so the 32 cases for other
  languages show as not applicable. Use a real snapshot built with that language.
- **macOS peak** comes from `ru_maxrss`, which may include memory from before the
  child started.
- **Linux** has native hosted-runner and real-data cloud-VM measurements, as well as earlier WSL2 checks. Those do not establish first-launch desktop behavior on physical Linux hardware.
- **Memory growth** is an estimate from a short window, and the Go heap saws up and
  down within it. It flags a leak; it does not size one.
- **Secondary hints** are uncorrected, so some will be false. Confirm a surprising one
  by repeating it.
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
- A sensitivity study with graded faults, to state the smallest change each profile finds.
- `standard` and `full` on every platform, through the workflow's manual trigger.
