<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo-dark.png">
  <img src="docs/images/logo-light.png" alt="Stremio Smart Search logo" width="140">
</picture>

# Stremio Smart Search

**Find what you meant to type.** A local search addon for Stremio that understands typos, joined words and abbreviations.

[Download](https://github.com/EskimoDev/stremio-smart-search/releases) · [Install guide](docs/INSTALLATION.md) · [Changelog](CHANGELOG.md)

[![Version](https://img.shields.io/badge/version-0.1.0--preview.4-orange)](CHANGELOG.md)
[![Check addon](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml/badge.svg)](https://github.com/EskimoDev/stremio-smart-search/actions/workflows/ci.yml)
![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux-informational)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

<img src="docs/images/search-before-stremio.jpg" alt="dx.bi search for hotd without Smart Search: hot dog films and unrelated series" width="100%">
<p><sub><b>Before:</b> searching <code>hotd</code> in dx.bi without Smart Search</sub></p>

<img src="docs/images/search-after-expanded.jpg" alt="dx.bi search for hotd with Smart Search on the Expanded catalogue: House of the Dragon leads the series row and House of the Devil and House of the Dead lead the movies" width="100%">
<p><sub><b>After:</b> the same search with Smart Search (Expanded catalogue)</sub></p>

</div>

## Installation

**Version covered: 0.1.0-preview.4.** This version adds sound-alike search, automatic catalogue updates, dark mode and the new app icons. Check [Releases](https://github.com/EskimoDev/stremio-smart-search/releases) for available downloads; draft prereleases are not public downloads. Still using preview.3? Follow its [version-specific guide](https://github.com/EskimoDev/stremio-smart-search/blob/v0.1.0-preview.3/README.md#installation).

Choose the package for your computer:

| Your computer | Package | Testing |
|---|---|---|
| Windows, Intel/AMD | **Windows-x64-Setup.exe** or portable ZIP | Stremio, dx.bi and Enhanced tested |
| Windows on ARM | Windows-ARM64-Setup.exe or portable ZIP | Builds; device testing needed |
| Mac, M-series chip | macOS-AppleSilicon.dmg or app ZIP | Automated checks; desktop testing needed |
| Mac, Intel | macOS-Intel.dmg or app ZIP | Automated checks; desktop testing needed |
| Linux, Intel/AMD | Linux-x64.deb or portable .tar.gz | Ubuntu 24.04 tested |
| Linux on ARM | Linux-ARM64.deb or portable .tar.gz | Builds; device testing needed |

Package names also include the version number. Preview builds are not developer-signed or notarized; see the [installation guide](docs/INSTALLATION.md) for download checks and first-open prompts. [Full testing status →](docs/PLATFORM-TESTING.md)

SSS runs as a small helper on your computer. It needs to be running for its search results to appear. There is no native TV or phone helper yet; see [other devices](docs/OTHER-DEVICES.md).

1. **Install and open Smart Search.** Its settings page opens in your browser.
2. **Prepare the catalogue.** Leave **Catalogue size** on Standard, or choose another size, then select **Prepare catalogue**. The first download is a few hundred MB; allow several minutes.
3. **Add it to Stremio.** Choose your app: **Stremio**, **dx.bi** or **Stremio Enhanced**. This copies the addon link and opens the app. In that app, go to **Addons → Add addon**, paste the link and choose **Install**. If your app isn't detected, copy the link manually.
4. **Search normally.** Try `HOTD` and look for the Smart Search movie and series rows.

> [!TIP]
> Move Smart Search to the top of your addon order to find its results first. In dx.bi: **Addons → Order & backup → move Smart Search to the top → Save order**. For other clients, see [the ordering guide](docs/INSTALLATION.md#put-smart-search-first).

Next time, use **Stremio + Smart Search** to start both together. You can close the settings page; the helper keeps running. Use **Smart Search Settings** or the addon's **Configure** button to reopen it.

[Windows, macOS, Linux and portable instructions →](docs/INSTALLATION.md)

## Dashboard

Choose your app, catalogue size and update schedule in one place, with light and dark themes.

<p>
  <a href="docs/images/dashboard-light.jpg"><img src="docs/images/dashboard-light.jpg" alt="Light dashboard with app connection options, catalogue settings, automatic updates and storage controls" width="49%"></a>
  <a href="docs/images/dashboard-dark.jpg"><img src="docs/images/dashboard-dark.jpg" alt="Dark dashboard with the same app connection options, catalogue settings, automatic updates and storage controls" width="49%"></a>
</p>
<p><sub>Preview.4 in light and dark mode; click either image to enlarge. Shown with a prepared Standard catalogue and automatic updates off. Totals and storage use vary.</sub></p>

## Try a search

| You type | Title to look for |
|---|---|
| `hotd` | *House of the Dragon* |
| `strangerthings` | *Stranger Things* |
| `game of thornes` | *Game of Thrones* |
| `pulp fikshun` | *Pulp Fiction* — preview.4 or newer |
| `dune 1984` | *Dune* (1984) |
| `les miserables` | *Les Misérables* |
| `la rene des neige` | *Frozen*, with French alternative titles enabled |

For translated or regional names, choose a language under **Alternative titles** in settings, then rebuild. This adds another data download. [Alternative-title setup →](docs/CATALOGUE.md#alternative-titles-optional)

Matching has limits: an exact title can rank above the similar title you meant. The sound-alike matcher finds `incepshun` and `back to the fucher`, but still misses `rattatooey`. See [tested search quality and known gaps](docs/SEARCH-QUALITY.md).

## Choose a catalogue

Standard is the default. Choose Compact to use less memory, or Expanded to include more titles with fewer IMDb votes. You can change size in settings; the current catalogue keeps working during a rebuild.

| Size | Titles in the measured snapshot | Memory after startup |
|---|---:|---:|
| Compact | 50,000 | 23 MiB |
| **Standard** | 184,606 | 53 MiB |
| Expanded | 390,470 | 97 MiB |

These are the **preview.3 Windows baseline**, recorded on one PC with IMDb data from 2026-11-01 and alternative titles off. They have not been remeasured on preview.4. Counts and memory vary. Each size keeps up to 25,000, 100,000 or 250,000 **each** of movies and series, subject to its vote threshold.

Matching runs locally. In that baseline, 95% of Expanded search requests finished within **0.77 ms**, including the local HTTP request and response. This measures SSS, not the time for Stremio to load artwork or other addons' results. [Measurements and method →](docs/PERFORMANCE.md)

[Title selection, updates and storage →](docs/CATALOGUE.md)

## Privacy and online access

- SSS needs no account, Stremio password, API key or AI subscription.
- Matching happens on your computer. SSS does not send your searches to a search service; Stremio still queries your other installed search addons as usual.
- IMDb is contacted when you prepare or refresh the catalogue, and by automatic updates. Posters come from MetaHub; your other addons provide metadata and playback.
- **Automatic-update bandwidth:** new installations default to every 2 days while the helper runs. Each changed-data update downloads about **240 MB**, plus about **520 MB weekly** with alternative titles: roughly **3.6 GB/month**, or **5.8 GB/month** with alternatives. Existing catalogues remain manual until enabled. Turn **Update automatically** off for manual-only downloads. See [controls, metered connections and estimates](docs/CATALOGUE.md#automatic-updates).
- No telemetry. Settings are accessible from the host computer only.

The software is MIT licensed. IMDb data has [separate personal, non-commercial terms](DATA.md) and is downloaded by each user; it is not bundled in the app.

## Common questions

<details>
<summary>Do I need to keep the browser page open?</summary>

No. Close it after setup and keep using Stremio. The helper continues running until you choose **Quit helper**, stop its process or shut down your computer. The combined launcher starts it again next time. Windows setup also offers an optional start-at-sign-in component.
</details>

<details>
<summary>Can I use a TV, phone or Stremio Web?</summary>

Stremio Web on the same computer has an install option in settings. A localhost addon link does not run the helper on your TV or phone when your addons sync. Advanced home-server options still need client testing. See [other devices](docs/OTHER-DEVICES.md).
</details>

<details>
<summary>How do I move the catalogue or remove Smart Search?</summary>

Use **Change location…** in the settings page's **Storage** card to move the catalogue. To remove the app, follow [uninstall instructions](docs/INSTALLATION.md#uninstalling). Uninstalling keeps your downloaded data and does not remove the addon entry from Stremio.
</details>

## Help and contributions

[Report a problem](https://github.com/EskimoDev/stremio-smart-search/issues/new). For a wrong result, include your search, the expected movie or series, catalogue size and any alternative-title language or region. Please leave out account tokens and debrid credentials.

For code changes, start with [Contributing](CONTRIBUTING.md) and [Architecture](docs/ARCHITECTURE.md). The [documentation index](docs/README.md) lists setup guides, benchmarks and search-quality results.

## Licence and credits

[MIT](LICENSE). Independent community project; not an official Stremio product. SSS does not host or distribute video.

The setup page follows [LongCipher Design by Akagi201](https://designmd.ai/Akagi201/longcipher-design) (CC0), with Geist fonts under the SIL Open Font License. See [third-party notices](THIRD-PARTY-NOTICES.txt).

Thanks to [dx.bi](https://github.com/notdxbi/dxbi-stremio), whose README inspired this layout.
