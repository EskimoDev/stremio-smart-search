# Contributing

Report a concrete search query, expected title and whether it is a movie or series. Do not attach account tokens, debrid credentials, watch history or your complete application data folder.

Run `go test ./...` and `go vet ./...` before proposing a code change. Match behavior changes should include a regression example. Network access is not needed for the default tests; use the small handwritten fixture. Keep catalogue files, downloads, build outputs and private control keys out of Git.

Platform changes need a native test or a clear note that only compilation was checked. Avoid adding a runtime, background crawler or cloud dependency to make a local installation work. Preserve normal Stremio addon behavior and document any change to first-run download size or memory limits.

Follow [DESIGN.md](DESIGN.md) for setup-page styling. Use locally bundled assets and preserve the install and clipboard behavior. For catalogue or index changes, use the [resource comparison workflow](docs/PERFORMANCE.md#reproduce) with real source data kept outside this repository. The benchmark tool is a developer utility and is not shipped as part of the helper.
