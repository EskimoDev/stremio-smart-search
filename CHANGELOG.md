# Changelog

All notable changes to Stremio Smart Search are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Preview builds use the `0.1.0-preview.N` pre-release form.

Previews so far are draft GitHub releases. They are listed here so each build can be traced to its commit.

## [Unreleased]

Planned as `0.1.0-preview.3`.

### Added

- Optional alternative-title search. Choose a language in settings to also index regional titles from IMDb's `title.akas` data, such as "La Reine des neiges" for *Frozen*. Off by default; results still return the canonical title. ([#1](https://github.com/EskimoDev/stremio-smart-search/pull/1))
- Catalogue sizes: Compact, Standard (new default) and Expanded, with measured resource comparisons in [docs/PERFORMANCE.md](docs/PERFORMANCE.md). Changing size rebuilds in the background and keeps the current catalogue until the replacement is ready.
- macOS app and DMG smoke test in CI, and recorded platform test findings in [docs/PLATFORM-TESTING.md](docs/PLATFORM-TESTING.md). ([#2](https://github.com/EskimoDev/stremio-smart-search/pull/2))
- Guidance on why local matching feels fast and on putting Smart Search first in the addon order.
- Search-quality evaluation: 248 curated queries with verified IMDb answers, scored by `tools/quality` for top-1, recall@5, false positives and per-case regressions. Results in [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md).

### Changed

- When several titles match a search equally well, the one the search covers best now ranks first: "the terminater" lists *The Terminator* before *Terminator 2*. Generated typo and prefix recall rose 1.3 to 5.2 points of top-1. See [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md#follow-up-prefer-titles-the-query-covers).
- Search accuracy on real catalogues: Expanded top-1 rose from 91.1% to 96.9% and recall@5 from 93.8% to 99.5% on the curated cases, with no case worse. A year typed after a title ("dune 1984") now picks that version instead of returning nothing. Swapped letters in long words ("game of thornes") and long translated titles are found. "hotd" no longer lists unrelated one-letter variants such as *Hot Fuzz*, and an exact regional title ranks by popularity alongside exact canonical names. See [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md).
- Rebuilt the search index around compact sorted arrays and faster catalogue parsing. On synthetic data, Expanded's memory after startup fell from about 276 to 92 MiB, its 95th-percentile search time from about 20 to 2 ms, and its live refresh from 50 to 14 s, with a peak of 270 MiB instead of 554 MiB. Search results are unchanged. See [docs/PERFORMANCE.md](docs/PERFORMANCE.md#index-and-preparation-rewrite).
- Redesigned the setup page as a compact desktop utility following LongCipher Design, with bundled Geist fonts and official client icons for Stremio, dx.bi and Stremio Enhanced.
- Polished the settings catalogue card and the addon order guide. ([#3](https://github.com/EskimoDev/stremio-smart-search/pull/3))
- Simplified setup branding and removed the header logo.
- Rewrote the README as a short landing page with a before/after search comparison. Detail moved to [docs/CATALOGUE.md](docs/CATALOGUE.md), [docs/OTHER-DEVICES.md](docs/OTHER-DEVICES.md) and [docs/INSTALLATION.md](docs/INSTALLATION.md).
- Added a changelog, folder READMEs and release steps, and moved DESIGN.md into docs/.
- Recorded user-confirmed Windows x64 compatibility with Stremio, dx.bi and Stremio Enhanced, and Linux x64 install testing on Ubuntu 24.04.

## [0.1.0-preview.2] - 2026-10-31

### Added

- Separate **Add to Stremio**, **Add to dx.bi** and **Add to Stremio Enhanced** buttons that copy the local manifest address and open the chosen app.
- **Install through Stremio Web** option that keeps the local HTTP address and port.
- The launcher remembers the chosen app.

### Fixed

- Local addon installation. The generic `stremio://` link was removed because Stremio rewrites it to HTTPS, which does not match the helper's local HTTP endpoint.

## [0.1.0-preview.1] - 2026-10-31

### Added

- First preview: a standalone local Stremio search addon with typo, joined-word and acronym matching (for example HOTD, interstelar, LOTR).
- Single Go executable with an embedded setup page that downloads IMDb title data on the device and builds a local index.
- Windows installers and portable ZIPs, macOS DMGs and app ZIPs, and Linux `.deb` and portable archives for x64 and ARM64, with `SHA256SUMS.txt`.
- "Stremio + Smart Search" launcher that starts the helper and opens Stremio.

### Fixed

- Executable permissions are preserved in cross-built archives.
- Platform download names are easier to identify.

[Unreleased]: https://github.com/EskimoDev/stremio-smart-search/compare/30f644c...main
[0.1.0-preview.2]: https://github.com/EskimoDev/stremio-smart-search/commit/30f644c625763da24e44de98db46d9deca06d286
[0.1.0-preview.1]: https://github.com/EskimoDev/stremio-smart-search/commit/d3a6d98aed1aee833fb834ae55dc071186f83a4f
