# Contributing

Report a concrete search query, expected title and whether it is a movie or series. Do not attach account tokens, debrid credentials, watch history or your complete application data folder.

Run `go test ./...`, `go vet ./...`, `node scripts/ui-test.mjs` and `node scripts/docs-test.mjs` before proposing a code change. Match behavior changes should include a regression example and a `tools/quality` run against real snapshots with no per-case regressions; see [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md). Network access is not needed for the default tests; use the small handwritten fixture. Keep catalogue files, downloads, build outputs and private control keys out of Git.

Platform changes need a native test or a clear note that only compilation was checked. Avoid adding a runtime, background crawler or cloud dependency to make a local installation work. Preserve normal Stremio addon behavior and document any change to first-run download size or memory limits.

Follow [docs/DESIGN.md](docs/DESIGN.md) for setup-page styling. Use locally bundled assets and preserve the install and clipboard behavior. For catalogue, index or search changes, compare your branch with main using [`sss-bench`](docs/BENCHMARKING.md#compare-two-versions), and include its comparison in the pull request; add a real snapshot with `--catalog` when the change depends on real titles. The benchmark tools are developer utilities and are not shipped as part of the helper.

## Development setup

Requires Go 1.24 or newer; CI currently uses Go 1.27.x. Node.js is needed for
the setup-page and documentation checks, and PowerShell is used for packaging and smoke checks.
The installed app needs none of these tools.

From the repository root:

```sh
go test ./...
go vet ./...
node scripts/ui-test.mjs
node scripts/docs-test.mjs
go run ./cmd/smart-search --help
```

To explore setup without touching your installed helper, use a separate port and
data folder with the handwritten fixture:

```sh
go run ./cmd/smart-search --listen 127.0.0.1:11499 --data-dir ../sss-dev-data --catalog testdata/catalog.json
```

That fixture has a few illustrative titles, not a production catalogue. Keep any
real IMDb downloads, snapshots and quality reports outside this repository.
[Scripts](scripts/README.md) and [Tools](tools/README.md) explain the build and
evaluation commands.

## Documentation changes

Run `node scripts/docs-test.mjs` and `git diff --check`. Read the rendered Markdown as well: the link check cannot catch an unreadable table or a misleading claim.

- Keep the primary README focused on installation, examples and the main trade-offs; link to detailed measurements.
- Label public-release behavior, unreleased main changes and historical experiments separately. Never advertise open-PR results as released.
- For a measurement, record the source revision, suite/workload, catalogue, platform and date. Keep denominators and coverage gaps visible, and distinguish engine time from HTTP time and full-app behavior.
- Preserve useful historical results, but put the latest relevant evidence first. Use short tables and expandable experimental detail rather than a chronological wall of results.
- After a case-file change, update suite counts and rerun quality checks before changing accuracy figures. Do not commit real datasets, snapshots or per-title generated reports.

## Optional real-data tests

Default tests use handwritten fixtures and local HTTP servers, without IMDb
downloads. The following integration tests skip unless explicitly enabled:

| Environment variable | Test | Network/data cost |
| --- | --- | --- |
| `SMART_SEARCH_REAL_IMDB=1` | `TestRealIMDbFreshnessCheck` | Metadata-only requests to IMDb; no dataset bodies |
| `SMART_SEARCH_REAL_IMDB_UPDATE=1` | `TestRealIMDbAutomaticUpdate` | About 238 MB from IMDb; no alternative-title file |
| `SMART_SEARCH_REAL_DATA=PATH` | `TestRealDataAutomaticUpdate` | Uses all three cached IMDb files over loopback; no internet downloads |
| `SMART_SEARCH_EXAMPLE_CATALOGS=PATHS` | `TestSettingsSearchExamplesOnRealData` | Loads locally built snapshots (separated like `PATH`) and checks the settings page's search examples; no network |
| `SSS_BENCH_E2E=1` | `TestRunEndToEnd`, `TestPrebuiltBinaryHasHonestGaps` | Builds and runs the real application in temporary folders; no network |

Select the intended test explicitly, give it `-timeout 60m` for full-data runs,
and unset its variable afterwards. All use temporary application folders.
The cached-file test covers alias carry-over and weekly refresh; the live test
covers real CDN validators and a download, so they serve different purposes.
See [recorded results and commands](docs/PERFORMANCE.md#automatic-updates).
A live-server check is not a substitute for deterministic regression tests.

## Releasing

Plain Go builds and local packaging now default to **0.1.0-dev**. Release jobs
must supply the intended version explicitly; this prevents a new main build
from identifying itself as an already published preview.
`scripts/build.ps1 -Version <version>` stamps the binary and passes the same
version to the archive packager. The release workflow also passes it to NSIS.

1. Choose an unused version and reviewed source commit. Check the public
   [releases](https://github.com/EskimoDev/stremio-smart-search/releases) and
   [Unreleased changes](CHANGELOG.md#unreleased); do not reuse a published version.
2. Move the intended changelog entries under that version and date. Check feature
   labels and benchmark scopes; a merge into main does not make a feature released.
   Keep local-build defaults at `0.1.0-dev`.
3. With approval to prepare release artifacts, push `v<version>` or run the release
   workflow on the chosen commit with an explicit version. It creates a **draft
   prerelease**, packages and `SHA256SUMS.txt`; it refuses to replace a published
   release. Replace its placeholder description with version-specific release
   notes, using tag-pinned documentation links so later main edits do not change
   instructions for that download.
4. Before publishing, check package contents, checksums, native install/uninstall,
   and a real Stremio install. Distinguish compiled targets and automated smoke
   checks from device tests. Do not include datasets, generated production
   catalogues, credentials or machine-specific files.
5. Publish only with approval. After publication, update the README's latest-release
   badge and release-status paragraph and add the version to platform records only
   for the devices actually tested. Retain historical benchmark context.
