# Stremio Smart Search

A standalone, local search addon for Stremio. **Stremio Smart Search (SSS)** gives you fast local title matching, including misspellings, joined words and abbreviations such as **HOTD**, **LOTR** and **TDK**. Works through standard movie and series search catalogs; no changes to Stremio or dx.bi are required.

**Preview under development.** Windows x64 has been user-tested with standard Stremio, dx.bi and Stremio Enhanced. Linux x64 packages have been install- and smoke-tested on Ubuntu 24.04. Automated checks run on Windows, macOS and Linux; macOS, ARM64 and TV setups are not yet device-tested. See [platform testing](docs/PLATFORM-TESTING.md). No release is published yet.

## How it works

The companion runs on your computer and provides a normal Stremio addon address. Searches use a local title index, without AI calls or a hosted search backend. Your other addons continue to provide their own results, metadata and playback.

**Why it can feel faster:** once the catalogue is prepared, matching runs against an index in your computer's memory. SSS does not wait for a remote search server or an AI response to find matches. This avoids an internet round trip for its matching work and can make its results appear sooner than remote search providers. Artwork, metadata and playback still use online providers, so complete page-load times vary. See the local catalogue's coverage below.

Standard Stremio addons do not execute downloaded code inside Stremio. This project includes the local companion needed to run the matching engine. The “Stremio + Smart Search” launcher starts the helper and opens your registered Stremio application. The helper stays available in the background until you quit it; it does not continuously monitor your processes or watching activity.

## Installation

Preview package targets. Use **Windows x64** for most Intel/AMD PCs, or **macOS Apple Silicon** for M-series Macs. ARM64 Windows downloads are for ARM-based PCs, not ordinary Intel/AMD machines.

| Platform | Primary package | Alternative | Verification |
|---|---|---|---|
| Windows x64 | Installer `.exe` | Portable ZIP | Installer tested; Stremio, dx.bi and Enhanced user-tested |
| Windows ARM64 | Installer `.exe` | Portable ZIP | Build target; device testing needed |
| macOS Apple Silicon | `.dmg` | App ZIP | Untested on a Mac; CI builds and smoke-tests the app and DMG |
| macOS Intel | `.dmg` | App ZIP | Untested on a Mac; CI builds and smoke-tests the app and DMG |
| Linux x64 | `.deb` | Portable `.tar.gz` | Installed and smoke-tested on Ubuntu 24.04 |
| Linux ARM64 | `.deb` | Portable `.tar.gz` | Build target; device testing needed |

After installation:

1. Open **Stremio Smart Search**. Its settings page opens in your browser.
2. Choose a **Catalogue size**, then **Prepare catalogue**. Standard is the default for new setups. The first setup downloads IMDb title data directly to this device, builds the local index and discards the raw data. Every size downloads the same source files; expect a few hundred MB and allow several minutes.
3. Choose **Stremio**, **dx.bi** or **Stremio Enhanced** under **Choose your app**. The button copies the correct HTTP link and opens that app. Open **Addons → Add addon**, paste the link and choose **Install**. Alternatively, expand **Using Stremio Web or a portable app?** and choose **Install through Stremio Web**; use the same account as your desktop client to sync it.
4. Search normally. Look for the **Smart Search** movie and series rows.

Next time, use **Stremio + Smart Search** to open both. Windows setup optionally enables start at sign-in so the helper is available when you open Stremio through other shortcuts. Closing the browser settings page does not stop the helper. Use **Quit helper** in settings to stop it.

The launcher remembers the app chosen in setup. App-specific buttons launch installed executables directly; the helper does not change protocol associations. Generic `stremio://` addon links are deliberately avoided because Stremio rewrites them to HTTPS, which is unsuitable for this local HTTP service. Keep the full `http://127.0.0.1:11479/manifest.json` address when installing manually. Some browsers require local-network permission for the web install option.

Portable users can run the included executable directly for setup. Pass `--launch` to launch Stremio, or `--no-browser` to run in the background without opening settings. No Node.js, Python, Docker or API keys are needed for the desktop app.

See [installation details](docs/INSTALLATION.md) for platform-specific packaging and current limitations.

## Put Smart Search first (recommended)

Move **Stremio Smart Search** to the top of your addon order so its movie and series results are easier to find at the top of search. Row placement is controlled by the Stremio client and your addon order; SSS cannot give itself priority through its manifest.

In **dx.bi**:

1. Open **Addons → Order & backup**, beside **Add addon**.
2. Move **Stremio Smart Search** to the top using the drag handle or up arrows.
3. Choose **Save order**, then run your search again.

If that control is hidden, enable **Addon order & backup** in dx.bi settings. Saving with that tool requires signing into Stremio. Other clients can have different reorder controls.

Moving **only SSS** keeps the other addons in their existing relative order. SSS supplies search-only catalogues: it does not supply streams, metadata or subtitles, or add Home/Discover rows. Your existing playback providers continue working in their previous order. The order is a Stremio account setting and may sync to your other clients.

The companion leaves this choice to you; it does not sign into your account or reorder addons automatically.

## Scope and title coverage

Choose the catalogue size in setup:

| Size | Movie ceiling | Series ceiling | Minimum IMDb votes |
| --- | ---: | ---: | ---: |
| Compact | 25,000 | 25,000 | 250 |
| Standard (new-setup default) | 100,000 | 100,000 | 25 |
| Expanded | 250,000 | 250,000 | 5 |

These are ceilings, not promised counts. Titles are selected by vote count from qualifying movies, TV movies, series and miniseries. Adult-marked titles and individual episodes are excluded; titles without sufficient votes are excluded even with Expanded. Primary and original titles are always searchable. Optionally, **Alternative titles** adds regional and translated titles in one chosen language, for example *La Reine des neiges* for *Frozen* or *Zootropolis* for *Zootopia*. See below. Coverage depends on IMDb's data; this does not promise every regional spelling or alternate title. Normal results from other addons remain available.

### Alternative titles (optional)

Under **Catalogue → Alternative titles**, choose a language. Where titles commonly differ by country, you can also choose a **Regional preference**, such as Spain or Mexico for Spanish, or France or Canada for French. Then choose **Apply titles & rebuild**. Search then also matches those titles, including misspellings and missing accents. Results keep their normal IMDb identifiers and names, and English and original titles keep working. Off is the default, and existing catalogues keep their current behaviour until you change it.

This adds IMDb's alternative-title file to each preparation: about 520 MB more to download (2026-11-01), roughly tripling the download, and it is streamed and discarded like the other files. Preparation takes about 10 seconds longer. With French titles, measured RAM rose by about 8–10 MiB (Compact) and 20–30 MiB (Standard). Search speed was unchanged on those sizes; Expanded, already the most memory-hungry, became slightly slower, so the settings page recommends Standard when alternative titles are on. See [performance](docs/PERFORMANCE.md#alternative-titles). Only the chosen language is kept. It is matched against your own installed catalogue, not translated, and no other service is involved.

The measured snapshot contained **50,000 / 184,588 / 390,449 titles** respectively. See the [resource comparison and reproducible measurements](docs/PERFORMANCE.md) before choosing a larger option, particularly on a low-memory device.

Exact titles are ranked first. Acronyms can have several legitimate meanings; popularity helps rank the alternatives. Typo matching has bounded edit distances and work limits. It does not generate fictional titles or use an LLM.

Catalogue updates are manual from settings. They do not run during every startup or search. To change size, select the option and choose **Apply size & rebuild**. The current index keeps serving during preparation; the new size is saved with the replacement catalogue only after success. A failed refresh keeps the previous working catalogue. Existing catalogues retain their Compact selection until you change it.

## Privacy and data

- No Stremio login, watch history, stream URLs or debrid credentials are required or collected.
- The local matcher does not forward search terms to an external search or AI service. Stremio still sends searches to the user's other installed search addons as usual.
- IMDb is contacted when you prepare or refresh the catalogue. Result posters use MetaHub; normal Stremio metadata and stream providers remain online services.
- The service listens on `127.0.0.1` by default. Settings are restricted to the host computer and a private control key. Public addon endpoints have the CORS headers required by Stremio.
- No telemetry or automatic background crawling.

Software is MIT licensed. **IMDb data is separate:** each user downloads it directly for personal, non-commercial use under [IMDb's dataset terms](https://developer.imdb.com/non-commercial-datasets/). Production title snapshots are not included in this repository or release packages. See [DATA.md](DATA.md).

## Other devices

Android, Android TV, Fire TV, iOS and tvOS do not currently have native companion packages. A localhost addon installed on your PC does not become a running helper on those devices through account sync.

Advanced users can host the companion on a home server and expose its addon endpoints through HTTPS. This avoids maintaining a separate engine on each device, but the host must remain online and client support must be checked. Stremio clients can require HTTPS for addresses other than loopback. Plain HTTP to a LAN address is not a promised cross-platform installation route.

**LAN mode (untested on TVs).** Running the companion with `--listen 0.0.0.0:11479` on a PC lets other devices on your network reach `http://<pc-ip>:11479/manifest.json`. Only the manifest and search are exposed; settings and controls stay on the PC. Whether Stremio on Fire TV or Android TV accepts that plain-HTTP address has not been tested yet. See [platform testing](docs/PLATFORM-TESTING.md#fire-tv-and-android-tv-lan-mode) for the steps to try it.

## Development

Requires Go 1.24 or newer. There are no third-party runtime dependencies.

```sh
go test ./...
go vet ./...
node scripts/ui-test.mjs
go run ./cmd/smart-search
```

Useful options:

```text
--launch                  Start helper and open the registered Stremio app
--no-browser              Keep running without opening settings
--quit                    Stop the instance using this data folder and port
--listen 127.0.0.1:11479   Change the listen address
--data-dir PATH           Use an isolated data folder
--catalog PATH            Load your own catalogue JSON/JSON.gz instead
--version                 Print version
```

Node.js is only used for the optional setup-page development tests, not by the shipped app. See [architecture](docs/ARCHITECTURE.md), [contribution guide](CONTRIBUTING.md) and the build scripts under `scripts/`. Tests use a small handwritten fixture, never a user's catalogue or viewing history. The optional real-catalogue benchmark reads a local path supplied explicitly by the developer.

This is an independent community project, not an official Stremio product. It does not host or distribute video.

## Design and bundled fonts

The setup interface follows [LongCipher Design by Akagi201](https://designmd.ai/Akagi201/longcipher-design), published under CC0. Geist Sans and Geist Mono are bundled locally under the SIL Open Font License; see [third-party notices](THIRD-PARTY-NOTICES.txt). The setup page needs no remote font or UI-library downloads.
