# Tools

Developer commands. They are not part of the shipped helper.

- `package/` turns a compiled binary into the release archives: Windows ZIP, macOS `.app` bundles and ZIP, Linux `.deb` and `.tar.gz`. `scripts/build.ps1` calls it.
- `benchmark/` compares local catalogue snapshots for memory, startup and search latency. It reads IMDb data from a path you pass in, which must stay outside this repository. Its `accuracy` phase measures typo and prefix recall. See [docs/PERFORMANCE.md](../docs/PERFORMANCE.md#reproduce).
- `synthetic/` writes invented, IMDb-shaped dataset files for benchmarking where the real ones cannot be downloaded. Results from them must be labelled synthetic.
