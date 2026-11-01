# Changelog

All notable changes to Stremio Smart Search are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Preview builds use the `0.1.0-preview.N` pre-release form.

`0.1.0-preview.1` and `0.1.0-preview.2` were draft-only builds and were never published. `0.1.0-preview.3` is the first public preview. Each entry links to the source it was built from.

## [Unreleased]

### Added

- Automatic catalogue updates, on by default for new installations. While the helper runs, it checks IMDb every 2 days (or every day, 3 days or week) with conditional `HEAD` requests and rebuilds only when the files changed. Start-up and search never wait for it, the current catalogue stays until the new one is validated, failures back off from 1 to 24 hours across restarts, Windows metered connections are skipped, and **Refresh now** joins a running update. With alternative titles on, routine updates keep the existing ones and download IMDb's 520 MB alternative-title file only weekly. Existing installations stay manual until switched on. Details in [docs/CATALOGUE.md](docs/CATALOGUE.md#automatic-updates).
- Dark mode for the settings page. It follows your system's light or dark setting, and a header toggle switches between Auto, Light and Dark and remembers your choice. The logo, buttons, inputs, status text and storage bar all switch with it.
- Sound-alike matching as a last resort: a word spelled the way it sounds, such as `incepshun`, `pulp fikshun` or `nite at the museum`, can now find the title. It uses a precomputed Double Metaphone index for ASCII words, ranks below every spelling match and never displaces an exact title. On the Expanded catalogue it finds 84.6% of curated phonetic searches first (was 28.2%), 73.3% of a held-out set (was 43.3%) and 76.8% of a blind validation set (was 41.1%), with no regressions on any of eleven real catalogues, for 1.6 MiB of memory. Codes one sound away (`brekin bad`, `back to the fucher`) also match, and a loose sound-alike needs a popular title, which cuts results for random pronounceable strings from 37% to 13%. Details in [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md#sound-alike-fallback-2026-11-01).
- A `phonetic` category of 39 search-quality cases, and a separate held-out phonetic file that was not used for tuning.

### Changed

- Reorganized the README around installation, example searches and catalogue choices, preserving the before/after screenshots. Detailed setup, measurements and development instructions live in the linked guides.
- Corrected documentation of accuracy denominators, temporary recovery files, the Storage memory estimate, platform testing and benchmark provenance.
- Trimmed the docs: PERFORMANCE.md leads with current results and one before/after table, DESIGN.md keeps only the rules that apply to the setup page, and maintainer notes moved from INSTALLATION.md to CONTRIBUTING.md.

## [0.1.0-preview.3] - 2026-11-01

### Added

- A **Configure** button for Smart Search in Stremio's addon list opens the local settings page. Settings stay reachable from the host computer only.
- Optional alternative-title search. Choose a language in settings to also index regional titles from IMDb's `title.akas` data, such as "La Reine des neiges" for *Frozen*. Off by default; results still return the canonical title. ([#1](https://github.com/EskimoDev/stremio-smart-search/pull/1))
- Catalogue sizes: Compact, Standard (new default) and Expanded, with measured resource comparisons in [docs/PERFORMANCE.md](docs/PERFORMANCE.md). Changing size rebuilds in the background and keeps the current catalogue until the replacement is ready.
- macOS app and DMG smoke test in CI, and recorded platform test findings in [docs/PLATFORM-TESTING.md](docs/PLATFORM-TESTING.md). ([#2](https://github.com/EskimoDev/stremio-smart-search/pull/2))
- Guidance on why local matching feels fast and on putting Smart Search first in the addon order.
- Search-quality evaluation: 248 curated queries with verified IMDb answers, scored by `tools/quality` for top-1, recall@5, false positives and per-case regressions. Results in [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md). ([#8](https://github.com/EskimoDev/stremio-smart-search/pull/8))
- Storage card in settings: a segmented bar for the catalogue, previous catalogue and settings, free space on the drive, the helper's memory, and **Open folder** (**Show in Explorer** / **Show in Finder**). ([#6](https://github.com/EskimoDev/stremio-smart-search/pull/6))
- **Change location…** moves the catalogue to another folder or drive, with a native folder picker on Windows, macOS and Linux (zenity or kdialog) and a typed path as fallback. Search keeps working during the move, and **Use default** moves it back. ([#6](https://github.com/EskimoDev/stremio-smart-search/pull/6))
- "Enjoying Smart Search?" block in settings with links to star the project, report a problem and check for updates. ([#6](https://github.com/EskimoDev/stremio-smart-search/pull/6))
- `tools/synthetic` writes IMDb-shaped test data, and the benchmark gained an `accuracy` phase with fixed probe queries. ([#7](https://github.com/EskimoDev/stremio-smart-search/pull/7))

### Changed

- When several titles match a search equally well, the one the search covers best now ranks first: "the terminater" lists *The Terminator* before *Terminator 2*. Generated typo and prefix recall rose 1.3 to 5.2 points of top-1. See [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md#follow-up-prefer-titles-the-query-covers).
- Search accuracy on real catalogues: Expanded top-1 rose from 91.1% to 96.9% and recall@5 from 93.8% to 99.5% on the curated cases, with no case worse. A year typed after a title ("dune 1984") now picks that version instead of returning nothing. Swapped letters in long words ("game of thornes") and long translated titles are found. "hotd" no longer lists unrelated one-letter variants such as *Hot Fuzz*, and an exact regional title ranks by popularity alongside exact canonical names. See [docs/SEARCH-QUALITY.md](docs/SEARCH-QUALITY.md). ([#8](https://github.com/EskimoDev/stremio-smart-search/pull/8))
- Rebuilt the search index around compact sorted arrays and faster catalogue parsing. Re-measured on real IMDb data on Windows, Expanded's memory after startup fell from about 276 to 97 MiB, its 95th-percentile search time from about 9.9 to 0.8 ms, and its live refresh from 33 to 6 s, with a peak of 275 MiB instead of 539 MiB. Search results are unchanged. See [docs/PERFORMANCE.md](docs/PERFORMANCE.md#index-and-preparation-rewrite). ([#7](https://github.com/EskimoDev/stremio-smart-search/pull/7))
- Redesigned the setup page as a compact desktop utility following LongCipher Design, with bundled Geist fonts and official client icons for Stremio, dx.bi and Stremio Enhanced.
- Polished the settings catalogue card and the addon order guide. ([#3](https://github.com/EskimoDev/stremio-smart-search/pull/3))
- Simplified setup branding and removed the header logo.
- Balanced the settings page columns, with storage in its own card. ([#6](https://github.com/EskimoDev/stremio-smart-search/pull/6), [#7](https://github.com/EskimoDev/stremio-smart-search/pull/7))
- Rewrote the README as a short landing page with a before/after search comparison. Detail moved to [docs/CATALOGUE.md](docs/CATALOGUE.md), [docs/OTHER-DEVICES.md](docs/OTHER-DEVICES.md) and [docs/INSTALLATION.md](docs/INSTALLATION.md). ([#5](https://github.com/EskimoDev/stremio-smart-search/pull/5))
- Alternative titles no longer suggest switching from Expanded to Standard. The index rewrite removed the memory pressure behind that advice.
- README: measured accuracy, system requirements and how to report a wrong result.
- Brought the docs up to date with the storage card, the index rewrite and the search-quality work, and documented addon ordering for Stremio and Stremio Enhanced.
- Added a changelog, folder READMEs and release steps, and moved DESIGN.md into docs/. ([#4](https://github.com/EskimoDev/stremio-smart-search/pull/4))
- Recorded user-confirmed Windows x64 compatibility with Stremio, dx.bi and Stremio Enhanced, and Linux x64 install testing on Ubuntu 24.04.

### Fixed

- **Change location…** no longer deletes the catalogue when the chosen folder is the one already in use under another spelling, such as different letter case on Windows or a linked folder. A move that fails part-way now removes its partial copies, and the catalogue stays where it was.
- Linux `.deb` previews are versioned `0.1.0~preview.N`, so apt treats the final `0.1.0` as an upgrade.
- `SHA256SUMS.txt` lists plain file names, and the installation guide explains how to check a download.

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

[Unreleased]: https://github.com/EskimoDev/stremio-smart-search/compare/v0.1.0-preview.3...main
[0.1.0-preview.3]: https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.3
[0.1.0-preview.2]: https://github.com/EskimoDev/stremio-smart-search/commit/30f644c625763da24e44de98db46d9deca06d286
[0.1.0-preview.1]: https://github.com/EskimoDev/stremio-smart-search/commit/d3a6d98aed1aee833fb834ae55dc071186f83a4f
