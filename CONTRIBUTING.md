# Contributing

Report a concrete search query, expected title and whether it is a movie or series. Do not attach account tokens, debrid credentials, watch history or your complete application data folder.

Run `go test ./...`, `go vet ./...` and `node scripts/ui-test.mjs` before proposing a code change. Match behavior changes should include a regression example and a `tools/quality` run against real snapshots with no per-case regressions; see [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md). Network access is not needed for the default tests; use the small handwritten fixture. Keep catalogue files, downloads, build outputs and private control keys out of Git.

Platform changes need a native test or a clear note that only compilation was checked. Avoid adding a runtime, background crawler or cloud dependency to make a local installation work. Preserve normal Stremio addon behavior and document any change to first-run download size or memory limits.

Follow [docs/DESIGN.md](docs/DESIGN.md) for setup-page styling. Use locally bundled assets and preserve the install and clipboard behavior. For catalogue or index changes, use the [resource comparison workflow](docs/PERFORMANCE.md#reproduce) with real source data kept outside this repository. The benchmark tool is a developer utility and is not shipped as part of the helper.

## Development setup

Requires Go 1.24 or newer; CI currently uses Go 1.27.x. Node.js is needed for
the setup-page tests only, and PowerShell is used for packaging and smoke checks.
The installed app needs none of these tools.

From the repository root:

```sh
go test ./...
go vet ./...
node scripts/ui-test.mjs
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

## Releasing

The version is stamped at build time: `scripts/build.ps1` passes it to the binary with `-ldflags "-X main.version=..."`, and to `tools/package` and the Windows installer. Untagged local builds report `0.1.0-dev`.

1. Move the `[Unreleased]` entries in [CHANGELOG.md](CHANGELOG.md) under the new version and date.
2. Update the default preview version in `scripts/build.ps1`, `scripts/windows-installer.nsi`, `tools/package/main.go` and `.github/workflows/release.yml` to the next planned version, and the version badge at the top of `README.md`.
3. Push a `v<version>` tag, or run the release workflow by hand. It prepares a draft pre-release with all packages and `SHA256SUMS.txt`. The workflow writes a fixed one-line description on the draft, so paste the release notes in after it finishes. The draft shows an `untagged-…` link until it is published; publishing creates the tag at the commit the workflow built.
4. Before publishing, run the native tests and smoke tests, check install and uninstall, install the addon in a real Stremio client, and check the packages contain no catalogue data, credentials or machine-specific files. Only call a package device-tested once it has run on that platform.
