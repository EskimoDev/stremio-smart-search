<div align="center">

# Stremio Smart Search

**Find what you meant to type.** A local search addon for Stremio that understands typos, joined words and abbreviations.

[Download](https://github.com/EskimoDev/stremio-smart-search/releases) · [Install guide](docs/INSTALLATION.md) · [Changelog](CHANGELOG.md)

[![Version](https://img.shields.io/badge/version-0.1.0--preview.3-orange)](CHANGELOG.md)
[![Check addon](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml/badge.svg)](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml)
![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux-informational)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

</div>

## What it does

- **Forgiving search.** `interstelar` finds *Interstellar*, `HOTD` finds *House of the Dragon*, `strangerthings` finds *Stranger Things*.
- **Regional titles, if you want them.** Turn on one language and `La Reine des neiges` finds *Frozen*.
- **Runs on your computer.** Matching uses a local index in memory, with no AI service, hosted backend, account or API key.
- **Plays nicely.** It adds search rows only. Your other addons keep supplying streams, metadata and their own results.

## Installation

> [!NOTE]
> Smart Search is in preview. No public release is published yet; preview builds are prepared as draft releases.

| Your computer | Download | Tested |
|---|---|---|
| Windows, most Intel/AMD PCs | x64 installer `.exe` or portable ZIP | ✅ Stremio, dx.bi and Stremio Enhanced |
| Windows on ARM | ARM64 installer `.exe` or portable ZIP | Build only |
| Mac with an M-series chip | Apple Silicon `.dmg` or app ZIP | CI smoke test, not yet on a real Mac |
| Intel Mac | Intel `.dmg` or app ZIP | CI smoke test, not yet on a real Mac |
| Debian/Ubuntu, Intel/AMD | x64 `.deb` or portable `.tar.gz` | ✅ Ubuntu 24.04 |
| Debian/Ubuntu on ARM | ARM64 `.deb` or portable `.tar.gz` | Build only |

Full results are in [platform testing](docs/PLATFORM-TESTING.md).

1. **Open Stremio Smart Search.** Its settings page opens in your browser.
2. **Prepare the catalogue.** Pick a size (Standard is the default) and choose **Prepare catalogue**. It downloads IMDb title data straight to your computer, a few hundred MB, and takes a few minutes the first time.
3. **Add it to your app.** Choose **Stremio**, **dx.bi** or **Stremio Enhanced**. The link is copied and the app opens. Go to **Addons → Add addon**, paste and choose **Install**.
4. **Search as usual.** Try `HOTD`, `interstelar` or `LOTR` and look for the Smart Search rows.

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

| Size | Movies | Series | Minimum IMDb votes |
|---|---:|---:|---:|
| Compact | up to 25,000 | up to 25,000 | 250 |
| **Standard** (default) | up to 100,000 | up to 100,000 | 25 |
| Expanded | up to 250,000 | up to 250,000 | 5 |

Larger sizes use more memory. Compare them in [performance](docs/PERFORMANCE.md), and read how titles are chosen and updated in [the catalogue guide](docs/CATALOGUE.md).

## Privacy and data

- No Stremio login, watch history, stream URLs or debrid credentials are needed or collected.
- Your searches are not sent to any search or AI service. Stremio still sends them to your other search addons as usual.
- IMDb is only contacted when you prepare or refresh the catalogue. Posters come from MetaHub.
- Settings are only reachable from your own computer. No telemetry, no background crawling.

IMDb data is downloaded by each user for personal, non-commercial use under [IMDb's terms](https://developer.imdb.com/non-commercial-datasets/), and is not included in this repository or the downloads. See [DATA.md](DATA.md).

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

The helper does no background work between searches, and its memory use is bounded by the catalogue size you pick. Measured numbers are in [performance](docs/PERFORMANCE.md).
</details>

<details>
<summary>How do I update the catalogue or uninstall?</summary>

Catalogue updates are manual: choose **Refresh catalogue** in settings. The current catalogue keeps working while the new one is prepared. To uninstall, use your system's normal app removal. Your Stremio settings and other addons are left untouched.
</details>

## Development

Requires Go 1.24 or newer and has no third-party runtime dependencies. Node.js is only used for the setup-page tests.

```sh
go test ./...
go vet ./...
node scripts/ui-test.mjs
go run ./cmd/smart-search --help
```

Start with [architecture](docs/ARCHITECTURE.md) and the [contribution guide](CONTRIBUTING.md). The [docs folder](docs/README.md) lists everything else.

## Licence and credits

MIT licensed. This is an independent community project, not an official Stremio product, and it does not host or distribute video.

The setup page follows [LongCipher Design by Akagi201](https://designmd.ai/Akagi201/longcipher-design) (CC0), with Geist fonts bundled under the SIL Open Font License. See [third-party notices](THIRD-PARTY-NOTICES.txt).
