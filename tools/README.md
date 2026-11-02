# Developer tools

Run these from the repository root. None is included in the installed helper.

| Tool | Purpose | Guide |
|---|---|---|
| `package/` | Turn a compiled binary into Windows ZIPs, macOS app bundles/ZIPs, and Linux .deb/tar.gz packages | [Build scripts](../scripts/README.md) |
| `sss-bench/` | Measure a build across platforms and compare two builds, with reproducible workloads and honest comparability checks. Start here | [Benchmarking](../docs/BENCHMARKING.md) |
| `benchmark/` | Prepare catalogue snapshots from local IMDb files; the fixed workload behind the historical tables | [Performance](../docs/PERFORMANCE.md#reproduce) |
| `quality/` | Score curated IMDb-answer cases and compare ranks and false positives with an earlier report | [Search quality](../docs/SEARCH-QUALITY.md#reproduce) |
| `icons/` | Rebuild the Windows icon resources (`cmd/smart-search/rsrc_windows_*.syso`) from `assets/icon.ico` after the logo changes | `go run ./tools/icons` |
| `synthetic/` | Generate invented IMDb-shaped input files when real source data is unavailable | [Performance](../docs/PERFORMANCE.md#reproduce) |

Keep raw IMDb files, generated catalogues and evaluation reports in a directory outside this repository. Label synthetic results as synthetic; they do not establish accuracy on real titles.

`scripts/build.ps1` calls the packager. Local versions default to `0.1.0-dev`; pass an explicit version for release artifacts. Portable archives include small README/installation entry points linking to bundled offline setup and the online guides, rather than copying repository documents whose images and relative links are absent.

Windows installers and macOS DMGs are produced by additional native steps in the release workflow, not by the packager itself. The checked-in `.syso` files are intentional build inputs: regenerate them from `assets/icon.ico` using `tools/icons`. macOS uses the separately supplied `assets/icon.icns`. No production catalogue is a build input.
