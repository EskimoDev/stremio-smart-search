# Developer tools

Run these from the repository root. None is included in the installed helper.

| Tool | Purpose | Guide |
|---|---|---|
| `package/` | Turn a compiled binary into Windows ZIPs, macOS app bundles/ZIPs, and Linux .deb/tar.gz packages | [Build scripts](../scripts/README.md) |
| `benchmark/` | Measure catalogue preparation, startup, memory, HTTP search latency and generated-query recall | [Performance](../docs/PERFORMANCE.md#reproduce) |
| `quality/` | Score curated IMDb-answer cases and compare ranks and false positives with an earlier report | [Search quality](../docs/SEARCH-QUALITY.md#reproduce) |
| `synthetic/` | Generate invented IMDb-shaped input files when real source data is unavailable | [Performance](../docs/PERFORMANCE.md#reproduce) |

Keep raw IMDb files, generated catalogues and evaluation reports in a directory outside this repository. Label synthetic results as synthetic; they do not establish accuracy on real titles.

`scripts/build.ps1` calls the packager. Windows installers and macOS DMGs are produced by additional native steps in the release workflow, not by the packager itself.
