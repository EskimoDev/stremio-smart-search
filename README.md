<div align="center">

# Stremio Smart Search

**Find what you meant to type.** A local search addon for Stremio that understands typos, joined words and abbreviations.

[Download](https://github.com/EskimoDev/stremio-smart-search/releases) · [Install guide](docs/INSTALLATION.md) · [Changelog](CHANGELOG.md)

[![Version](https://img.shields.io/badge/version-0.1.0--preview.3-orange)](CHANGELOG.md)
[![Check addon](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml/badge.svg)](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml)
![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux-informational)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

<img src="docs/images/search-before-stremio.jpg" alt="dx.bi search for hotd without Smart Search: hot dog films and unrelated series" width="100%">
<p><sub><b>Before:</b> searching <code>hotd</code> in dx.bi without Smart Search</sub></p>

<img src="docs/images/search-after-expanded.jpg" alt="dx.bi search for hotd with Smart Search on the Expanded catalogue: House of the Dragon leads the series row and House of the Devil and House of the Dead lead the movies" width="100%">
<p><sub><b>After:</b> the same search with Smart Search (Expanded catalogue)</sub></p>

</div>

## Why Smart Search

Without it, an abbreviation or a small misspelling often brings back unrelated titles, as in the screenshot above. Smart Search adds a Movies row and a Series row to your normal search results that work out what you meant.

| You type | You get |
|---|---|
| `hotd` | *House of the Dragon* |
| `strangerthings` | *Stranger Things* |
| `game of thornes` | *Game of Thrones* |
| `dune 1984` | *Dune* (1984), not the 2021 film |
| `les miserables` | *Les Misérables*, with or without accents |
| `la rene des neige` (with French titles on) | *Frozen* |

Each of these returns the expected title first in the [search-quality cases](docs/SEARCH-QUALITY.md).

### At a glance

- 🎯 **Accurate**: Right title first in **97.9%** of 248 checked searches, and in the top five in **99.5%** (Expanded catalogue).
- ⚡ **Fast**: **0.8 ms** for 95% of searches on the largest catalogue, measured over real HTTP on a desktop PC.
- 🪶 **Light**: About **97 MiB** of memory for 390,000 titles, no measurable CPU while idle, and a single 3 to 8 MB download with no runtime to install.
- 🔒 **Private**: No account, no Stremio login, no AI or search service. Matching runs on your computer.
- 🤝 **Plays nicely**: Adds search rows only. Your other addons keep supplying streams, metadata and their own results.

## Installation

> [!NOTE]
> Smart Search is in preview. Download it from the [latest release](https://github.com/EskimoDev/stremio-smart-search/releases). Builds are not code-signed yet, and only Windows x64 and Linux x64 have been tested on real machines.

| Your computer | Download | Tested |
|---|---|---|
| Windows, most Intel/AMD PCs | x64 installer `.exe` or portable ZIP | ✅ Stremio, dx.bi and Stremio Enhanced |
| Windows on ARM | ARM64 installer `.exe` or portable ZIP | Build only |
| Mac with an M-series chip | Apple Silicon `.dmg` or app ZIP | CI smoke test, not yet on a real Mac |
| Intel Mac | Intel `.dmg` or app ZIP | CI smoke test, not yet on a real Mac |
| Debian/Ubuntu, Intel/AMD | x64 `.deb` or portable `.tar.gz` | ✅ Ubuntu 24.04 |
| Debian/Ubuntu on ARM | ARM64 `.deb` or portable `.tar.gz` | Build only |

Needs Windows 10 or newer, macOS 11 or newer, or 64-bit Linux. Full results are in [platform testing](docs/PLATFORM-TESTING.md).

1. **Open Stremio Smart Search.** Its settings page opens in your browser.
2. **Prepare the catalogue.** Pick a size (Standard is the default) and choose **Prepare catalogue**. It downloads IMDb title data straight to your computer, a few hundred MB, and takes a few minutes the first time.
3. **Add it to your app.** Choose **Stremio**, **dx.bi** or **Stremio Enhanced**. The link is copied and the app opens. Go to **Addons → Add addon**, paste and choose **Install**.
4. **Search as usual.** Try `HOTD`, `strangerthings` or `LOTR` and look for the Smart Search rows.

Next time, open **Stremio + Smart Search** to start both together. You can close the settings page; the helper keeps running until you choose **Quit helper**.

<details>
<summary>Windows or macOS warns about the app</summary>

Preview builds are not code-signed yet. On Windows, choose **More info → Run anyway** if SmartScreen appears. On a Mac, approve the app under **System Settings → Privacy & Security**. See the [install guide](docs/INSTALLATION.md) for each platform.
</details>

<details>
<summary>Portable builds and Stremio Web</summary>

Run the executable directly. Add `--launch` to also open Stremio, or `--no-browser` to run quietly in the background. For Stremio Web, expand **Using Stremio Web or a portable app?** on the settings page and use the same account as your desktop app. Keep the full `http://127.0.0.1:11479/manifest.json` address when adding it by hand. More in [launcher and portable use](docs/INSTALLATION.md#launcher-and-portable-use).
</details>

## Put Smart Search first (recommended)

> [!TIP]
> Move Smart Search to the top of your addon order so its results show first. In dx.bi: **Addons → Order & backup → move Smart Search to the top → Save order**.

Moving only Smart Search keeps your other addons in the same order, and your stream providers are not affected. Other apps have different controls; see the [ordering guide](docs/INSTALLATION.md#put-smart-search-first).

## Catalogue sizes

Pick how many titles to search. You can change it later, and search keeps working while the new catalogue is built.

| Size | Titles kept | Minimum IMDb votes | Memory after startup | 95% of searches under | Right title first |
|---|---:|---:|---:|---:|---:|
| Compact | 50,000 | 250 | 23 MiB | 0.44 ms | 99.5% |
| **Standard** (default) | 184,606 | 25 | 53 MiB | 0.60 ms | 99.0% |
| Expanded | 390,470 | 5 | 97 MiB | 0.77 ms | 97.9% |

Titles kept are from the 2026-11-01 IMDb data; each size allows up to 25k, 100k or 250k each of movies and series. Larger catalogues find more obscure titles, so a few common typos have more look-alikes to compete with. How titles are chosen and updated is in [the catalogue guide](docs/CATALOGUE.md).

## Benchmarks

Measured on real IMDb data on one Windows PC (Ryzen 5 5600X, 32 GiB RAM), with 750 real HTTP requests per run mixing exact titles, acronyms, typos, accents, non-Latin text and no-result searches. Medians of three fresh runs.

**The index rewrite in this release:**

| Expanded catalogue | Before | After |
|---|---:|---:|
| Memory after startup | 276 MiB | **97 MiB** |
| 95th-percentile search | 9.86 ms | **0.77 ms** |
| 99th-percentile search | 17.87 ms | **1.15 ms** |
| Load and build the index | 4.45 s | **1.85 s** |
| Live catalogue refresh | 32.8 s | **6.2 s** |
| Peak memory during refresh | 539 MiB | **275 MiB** |

Search results did not change: about 96,000 generated queries returned identical result lists before and after.

**Accuracy** is scored with 248 hand-checked searches in 15 categories, from exact titles and acronyms to typos, years, accents, Japanese titles and regional titles in five languages. Every expected answer is verified against IMDb's own data before scoring. Exact titles, acronyms, joined words, punctuation, accents and Japanese titles all score 100%.

Full tables, method and caveats: [performance](docs/PERFORMANCE.md) and [search quality](docs/SEARCH-QUALITY.md). Both can be reproduced with the tools in this repository.

## Privacy and data

- No Stremio login, watch history, stream URLs or debrid credentials are needed or collected.
- Your searches are not sent to any search or AI service. Stremio still sends them to your other search addons as usual.
- IMDb is only contacted when you prepare or refresh the catalogue. Posters come from MetaHub.
- Settings are only reachable from your own computer. No telemetry, no background crawling.

IMDb data is downloaded by each user for personal, non-commercial use under [IMDb's terms](https://developer.imdb.com/non-commercial-datasets/), and is not included in this repository or the downloads. See [DATA.md](DATA.md).

## Is it right for you?

Smart Search is a good fit if you search Stremio on a Windows, Mac or Linux computer and often misspell, abbreviate or half-remember titles. It may not be, yet, if:

- **You watch on a TV, phone or Fire TV only.** The helper has to run on a computer. Home-server options are in [other devices](docs/OTHER-DEVICES.md).
- **You need signed installers.** Preview builds are not code-signed, so Windows and macOS show a warning the first time.
- **You type sounds, not letters.** `incepshun` is too far from *Inception* to match. When a real but obscure film has the exact name you typed, such as *Interstelar* (2016), it ranks first and the famous title second. Known gaps are listed in [search quality](docs/SEARCH-QUALITY.md#remaining-weaknesses).

## FAQ

<details>
<summary>Does it work on my TV or phone?</summary>

Not natively yet. The helper runs on Windows, macOS and Linux. Installing the addon on your PC does not make it run on other devices through account sync. Home-server and LAN options are described in [other devices](docs/OTHER-DEVICES.md).
</details>

<details>
<summary>How do regional titles work?</summary>

In settings, choose a language under **Alternative titles**, optionally a region, then **Apply titles & rebuild**. It adds about 520 MB to the download and a little memory. Results still use the normal title. Details are in [the catalogue guide](docs/CATALOGUE.md#alternative-titles-optional).
</details>

<details>
<summary>Will it slow down my computer?</summary>

Unlikely. The helper does no background work between searches: a 10-second quiet sample measured no CPU use for any size. It uses 23 to 97 MiB of memory depending on the catalogue size, and a refresh briefly peaks at up to 275 MiB. The **Storage** card in settings shows its current memory and disk use. Measured numbers are in [performance](docs/PERFORMANCE.md).
</details>

<details>
<summary>Where are its files, and can I move them?</summary>

The **Storage** card in settings shows how much space the catalogue, the previous catalogue kept for recovery and the settings take, and how much is free on that drive. **Open folder** shows the data folder, and **Change location…** moves the catalogue to another folder or drive while search keeps working. Settings stay in place. Details are in [the catalogue guide](docs/CATALOGUE.md#where-the-catalogue-is-stored).
</details>

<details>
<summary>How do I update the catalogue or uninstall?</summary>

Catalogue updates are manual: choose **Refresh catalogue** in settings. The current catalogue keeps working while the new one is prepared. To uninstall, use your system's normal app removal. Your Stremio settings and other addons are left untouched. The downloaded catalogue stays behind; use **Open folder** in settings first if you want to delete it too.
</details>

## Found a wrong result?

[Open an issue](https://github.com/EskimoDev/stremio-smart-search/issues/new) with what you typed, the title you expected, and whether it is a movie or series. Each report can become a [test case](docs/SEARCH-QUALITY.md), so the fix stays fixed. Please don't attach account tokens or debrid details.

## Development

Requires Go 1.24 or newer and has no third-party runtime dependencies. Node.js is only used for the setup-page tests.

```sh
go test ./...
go vet ./...
node scripts/ui-test.mjs
go run ./cmd/smart-search --help
```

Start with [architecture](docs/ARCHITECTURE.md) and the [contribution guide](CONTRIBUTING.md). Matching changes are checked with the [search-quality cases](docs/SEARCH-QUALITY.md) and index changes with the [performance benchmark](docs/PERFORMANCE.md#reproduce). The [docs folder](docs/README.md) lists everything else.

## Licence and credits

MIT licensed. This is an independent community project, not an official Stremio product, and it does not host or distribute video.

The setup page follows [LongCipher Design by Akagi201](https://designmd.ai/Akagi201/longcipher-design) (CC0), with Geist fonts bundled under the SIL Open Font License. See [third-party notices](THIRD-PARTY-NOTICES.txt).

Thanks to [dx.bi](https://github.com/notdxbi/dxbi-stremio), whose README inspired the layout of this one.
