# Changelog

All notable changes to Stremio Smart Search are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Preview builds use the `0.1.0-preview.N` pre-release form.

`0.1.0-preview.1` and `0.1.0-preview.2` were draft-only builds and were never published. `0.1.0-preview.3` is the first public preview. Each entry links to the source it was built from.

## [Unreleased]

### Added

- **Only load search while Stremio is open**, an optional setting under **You can close this page**, off by default. The helper keeps its port open but drops its search index while no Stremio, dx.bi or Stremio Enhanced window is open, and loads it again when one opens. A search that arrives in standby, for example from web.stremio.com, loads the index straight away and waits for it. Most of the helper's memory is the index: on the Windows test PC with the Expanded catalogue, standby used 19.5 MiB instead of 101 MiB, and the first search after standby took 1.4 s. It checks for a running Stremio app every 5 seconds (every 30 seconds while one is open); in standby that used 188 ms of CPU a minute on the same PC, and goes into standby 2 minutes after the last Stremio window closes (15 minutes after a browser search). Automatic updates wait until the index is loaded. With the setting off, nothing changes and no process checks run.

### Changed

- Store display names together in the search index, reducing steady memory use without changing catalogue files, result metadata or ranking. On the Windows test PC with Expanded, idle working set fell from 96.0 to 82.5 MiB; peak memory did not improve. See [measurements and limits](docs/PERFORMANCE.md#resource-follow-up-main-only).
- Lower the memory peak of loading the search index, at startup and when standby wakes. The parsed catalogue is released while the index is built, the index keeps its words and names packed together, and loading runs under a 200 MiB memory limit instead of 256 MiB. In a cloud A/B with a real Expanded catalogue with French titles, peak memory fell from 251 to 197 MiB and idle memory from 84.1 to 75.8 MiB, with startup time unchanged and identical search results.

### Fixed

- A scheduled update no longer downloads files or wakes the index when its metadata check finishes after standby begins. The deferred update stays eligible after waking, and **Refresh now** still works from standby. Normal update sizes and the chosen schedule are unchanged.

## [0.1.0-preview.6] - 2026-11-03

### Security

- The settings page no longer contains the private control key. Any program or other user account on the computer could fetch the page and use the key to change settings, quit the helper or move the catalogue. Smart Search now opens settings with a one-time code that the browser swaps for a session cookie. Bookmarks and Stremio's **Configure** button keep working in a browser Smart Search has opened; another browser shows a page explaining how to open settings.
- Moving the catalogue no longer writes through a file or link already planted in the destination folder.

### Changed

- Queries with several misspelled words now find what they mean. A title used to have to match every word, so one word that matched nothing emptied the results: `gma e of thr` and `the wakling ded` found nothing. When the query as typed matches poorly, Smart Search now also tries leaving out one short word, joining a stray letter to its neighbour (`gma e` as *game*), moving a space (`shaw shank redemption`, `gam eof thrones`, `mad maxfury road`) and simple typos in rare three-letter words (`family gyu`, `bak to teh futre`). On real Standard and Expanded catalogues 116 of 117 such test queries now rank the right title first, against 93 before, and no quality case regressed on 11 snapshots. The quality suite gains `multi-typo` and `dropped-vowel` categories (58 cases). Searches that read fine as typed do no extra work. See [Search quality](docs/SEARCH-QUALITY.md#several-misspelled-words-2026-11-02).
- Matching is about twice as fast. Typo lookups no longer allocate memory for each query, so on the Windows test PC with the Expanded catalogue and French alternative titles, 95% of requests finish within 0.57 ms instead of 0.98 ms, two clients are served 75% more requests per second and each search allocates 16 KiB instead of 214 KiB. Results are unchanged. See [Performance](docs/PERFORMANCE.md#preview6-on-windows).
- Start-up is about 40% faster. The helper uses a faster catalogue reader and parallel index preparation: on the same PC it is ready in 1.4 s instead of 2.3 s and uses about 5% less memory once started. The built index is unchanged.
- The documentation separates released behavior from changes on main, puts current results first, and records the search comparison with Stremio's default search.
- Release maintenance: the release workflow no longer fails when pushing the tag of a release that has already been published.

### Fixed

- Opening the sections in the left column of settings no longer stretches the Storage card and spreads its contents apart.

## [0.1.0-preview.5] - 2026-11-02

### Added

- **Start Smart Search when I sign in** in settings, off by default. It writes only the current user's own sign-in entry (the Windows Run value the installer's optional component also uses, a macOS LaunchAgent, or a Linux XDG autostart file) and starts the helper without settings or Stremio. See [the launcher section](docs/INSTALLATION.md#launcher-and-portable-use).
- A **Quit Smart Search** shortcut in the Windows Start Menu and the Linux app menu.
- A Smart Search icon in the Windows notification area while the helper runs. Double-click it (or select it and press Enter) to open the dashboard; right-click it (or press Shift+F10) for **Open dashboard** and **Quit Smart Search**. It uses Windows' own API with no extra dependency, and returns after Explorer restarts or a slow sign-in.
- Until a catalogue exists, Smart Search's search rows show one "Smart Search needs setup" item explaining what to do, instead of staying silently empty.
- A message box (Windows), alert (macOS) or dialog (Linux, with zenity or kdialog) when the helper cannot start, for example because another program holds port 11479. Terminal and scripted runs still print to standard error only.
- The addon manifest supplies the official SSS logo as a 256×256 PNG served by the helper. Its absolute URL follows the manifest request's origin, including custom ports. Clients that retain an older installed manifest may need to [refresh it](docs/INSTALLATION.md#addon-logo-after-an-update).
- `sss-bench`, a developer tool for measuring a build on Windows, macOS and Linux and comparing two versions, with a manually triggered CI workflow. It runs offline against a generated fixture or a supplied snapshot, never touches an installed helper, and declines to compare results from different hardware, catalogues or workloads. It is not part of the installed app. See [Benchmarking](docs/BENCHMARKING.md).

### Changed

- A short or unfinished query word now stands for the words of the most popular titles it starts, not the first 20 in alphabetical order. `star war` used to find nothing because *wars* is the 89th word starting with *war*; it now lists the Star Wars films, and `game of thr` finds *Game of Thrones*. A completion that adds a letter or two counts slightly more than a longer one, so `spidr man no wy hom` ranks *No Way Home* above *Homecoming*. Sound-alike spellings no longer pay for letters that are written but not spoken (the k and gh of *knight*, the ph of *phone*), so `nite and day` finds *Knight and Day*, `nite rider` *Knight Rider* and `my nayber totoro` *My Neighbor Totoro*. No quality case regressed on 11 real catalogue snapshots.
- Query words typed without their vowels now find the words they stand for: `strngr thngs` finds *Stranger Things*, `gdfthr` *The Godfather*, `frnds` *Friends* and `gme thron` *Game of Thrones*. A word with at most one vowel that few titles use is compared with indexed words by its consonants in order, and every letter typed must appear in the word. On real Standard and Expanded catalogues 24 of 25 such test queries now rank the right title first, against 2 before, and no quality case regressed on 11 snapshots. The index grows by four bytes per word (about 0.4 MiB on Standard, 0.7 MiB on Expanded).
- First-run settings make **Download the title catalogue** step 01 in the main column, with the Stremio steps numbered after it. The page's heading, reading and keyboard order now start with the main steps.
- Accessibility: download progress is announced to screen readers when its stage changes and every 15 seconds; app buttons stay focusable while unavailable and say why when chosen; **Quit helper** stays armed until focus leaves it instead of a 4-second timer, and announces that state; small print rises from 11 to 12 px.
- The settings page's **Ready to try it?** examples now show one search per kind of match: a year (`dune 1984`), a typo, a sound-alike spelling, a short form, words run together, a few words of a long title and an original title. `interstelar` was dropped because on Standard and Expanded catalogues a little-known film actually titled *Interstelar* correctly ranks first. A test checks each example against locally built catalogues when `SMART_SEARCH_EXAMPLE_CATALOGS` is set.

### Fixed

- The Windows installer's component list shows full names (**Smart Search (required)** and **Start when I sign in**) with a description underneath, instead of cutting them off. The settings page wraps a long sign-in path instead of letting it run into the next column.

## [0.1.0-preview.4] - 2026-11-01

### Added

- Automatic catalogue updates, on by default for new installations. While the helper runs, it checks IMDb every 2 days (or every day, 3 days or week) with conditional `HEAD` requests and rebuilds only when the files changed. Start-up and search never wait for it, the current catalogue stays until the new one is validated, failures back off from 1 to 24 hours across restarts, Windows metered connections are skipped, and **Refresh now** joins a running update. With alternative titles on, routine updates keep the existing ones and download IMDb's 520 MB alternative-title file only weekly. Existing installations stay manual until switched on. Details in [docs/CATALOGUE.md](docs/CATALOGUE.md#automatic-updates).
- Dark mode for the settings page. It follows your system's light or dark setting, and a header toggle switches between Auto, Light and Dark and remembers your choice. The logo, buttons, inputs, status text and storage bar all switch with it.
- Sound-alike matching as a last resort for ASCII words, including `incepshun` and `pulp fikshun`. Same-code and near-code candidates have lower word-match weights than spelling matches; exact-title score bands are unchanged. Popularity and edit-distance checks limit loose matches. Measured gains, costs and remaining false positives are recorded in [search quality](docs/SEARCH-QUALITY.md#sound-alike-follow-up-near-codes-and-confidence-2026-11-01).
- A `phonetic` category of 39 search-quality cases, plus separate held-out and blind-validation files.
- SSS branding in the README, settings header, browser tab, Windows executable/installer and macOS app bundles. These icon changes are not in preview.3.

### Changed

- Local packages default to 0.1.0-dev; release preparation requires an explicit version. Portable archive documentation links to bundled offline setup and online guides instead of missing repository files.
- Added a dependency-free Markdown link/image check to CI and release preparation, and ignored local storage/update-state files.
- Reorganized the README around installation, example searches and catalogue choices, preserving the before/after screenshots. Detailed setup, measurements and development instructions live in the linked guides.
- Corrected documentation of accuracy denominators, temporary recovery files, the Storage memory estimate, platform testing and benchmark provenance.
- Distinguished public preview.3 behavior from newer main features, retained historical performance and quality records, and clarified automatic-update downloads and optional real-data tests.
- Trimmed the docs: PERFORMANCE.md leads with current results and one before/after table, DESIGN.md keeps only the rules that apply to the setup page, and maintainer notes moved from INSTALLATION.md to CONTRIBUTING.md.

### Fixed

- Automatic updates keep the working catalogue after interrupted, truncated or older-CDN downloads, and reject implausibly small replacements. Failure/retry state persists across restarts.
- Freshness checks overtaken by manual refreshes are discarded. Runs using `--catalog` keep automatic updates disabled for that run.
- A manual refresh can adopt an automatic update rather than starting a duplicate download; if alias carry-over is already committed, settings explain when to retry.

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

[Unreleased]: https://github.com/EskimoDev/stremio-smart-search/compare/v0.1.0-preview.6...main
[0.1.0-preview.6]: https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.6
[0.1.0-preview.5]: https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.5
[0.1.0-preview.4]: https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.4
[0.1.0-preview.3]: https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.3
[0.1.0-preview.2]: https://github.com/EskimoDev/stremio-smart-search/commit/30f644c625763da24e44de98db46d9deca06d286
[0.1.0-preview.1]: https://github.com/EskimoDev/stremio-smart-search/commit/d3a6d98aed1aee833fb834ae55dc071186f83a4f
