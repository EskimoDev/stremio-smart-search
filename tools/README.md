# Tools

Developer commands. They are not part of the shipped helper.

- `package/` turns a compiled binary into the release archives: Windows ZIP, macOS `.app` bundles and ZIP, Linux `.deb` and `.tar.gz`. `scripts/build.ps1` calls it.
- `benchmark/` compares local catalogue snapshots for memory, startup and search latency. It reads IMDb data from a path you pass in, which must stay outside this repository. See [docs/PERFORMANCE.md](../docs/PERFORMANCE.md#reproduce).
