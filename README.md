# Stremio Smart Search

A standalone, local search addon for Stremio. Find titles with misspellings, joined words and abbreviations such as **HOTD**, **LOTR** and **TDK**. Works through standard movie and series search catalogs; no changes to Stremio or dx.bi are required.

**Preview under development.** Windows is being tested locally. macOS and Linux build targets and CI are included, but device support is not established merely by cross-compiling. No release is published yet.

## How it works

The companion runs on your computer and provides a normal Stremio addon address. Searches use a local title index, without AI calls or a hosted search backend. Your other addons continue to provide their own results, metadata and playback.

Standard Stremio addons do not execute downloaded code inside Stremio. This project includes the local companion needed to run the matching engine. The “Stremio + Smart Search” launcher starts the helper and opens your registered Stremio application. The helper stays available in the background until you quit it; it does not continuously monitor your processes or watching activity.

## Installation

Preview package targets. Use **Windows x64** for most Intel/AMD PCs, or **macOS Apple Silicon** for M-series Macs. ARM64 Windows downloads are for ARM-based PCs, not ordinary Intel/AMD machines.

| Platform | Primary package | Alternative | Verification |
|---|---|---|---|
| Windows x64 | Installer `.exe` | Portable ZIP | Tested on Windows in development |
| Windows ARM64 | Installer `.exe` | Portable ZIP | Build target; device testing needed |
| macOS Apple Silicon | `.dmg` | App ZIP | Build target; device testing needed |
| macOS Intel | `.dmg` | App ZIP | Build target; device testing needed |
| Linux x64 | `.deb` | Portable `.tar.gz` | Build target; device testing needed |
| Linux ARM64 | `.deb` | Portable `.tar.gz` | Build target; device testing needed |

After installation:

1. Open **Stremio Smart Search**. Its settings page opens in your browser.
2. Choose **Prepare catalogue**. The first setup downloads IMDb title data directly to this device, streams it into a compact index and discards the raw data. Expect a few hundred MB of network transfer; allow several minutes.
3. Choose **Add to Stremio**, **Add to dx.bi** or **Add to Stremio Enhanced**. The button copies the correct HTTP link and opens that app. Open **Addons → Add addon**, paste the link and choose **Install**. Alternatively, **Install through Stremio Web** opens the web install prompt; use the same account as your desktop client to sync it.
4. Search normally. Look for the **Smart Search** movie and series rows.

Next time, use **Stremio + Smart Search** to open both. Windows setup optionally enables start at sign-in so the helper is available when you open Stremio through other shortcuts. Closing the browser settings page does not stop the helper. Use **Quit helper** in settings to stop it.

The launcher remembers the app chosen in setup. App-specific buttons launch installed executables directly; the helper does not change protocol associations. Generic `stremio://` addon links are deliberately avoided because Stremio rewrites them to HTTPS, which is unsuitable for this local HTTP service. Keep the full `http://127.0.0.1:11479/manifest.json` address when installing manually. Some browsers require local-network permission for the web install option.

Portable users can run the included executable directly for setup. Pass `--launch` to launch Stremio, or `--no-browser` to run in the background without opening settings. No Node.js, Python, Docker or API keys are needed for the desktop app.

See [installation details](docs/INSTALLATION.md) for platform-specific packaging and current limitations.

## Scope and title coverage

The default catalogue keeps up to 25,000 movies and 25,000 series, selected by IMDb vote counts. Titles need at least 250 votes; adult-marked titles and individual TV episodes are excluded. Primary and original titles are searchable. This is deliberately bounded and does not promise every title, regional spelling or subtitle alias in the world. Normal results from other addons remain available.

Exact titles are ranked first. Acronyms can have several legitimate meanings; popularity helps rank the alternatives. Typo matching has bounded edit distances and work limits. It does not generate fictional titles or use an LLM.

Catalogue updates are manual from settings. They do not run during every startup or search. A failed refresh keeps the previous working catalogue.

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
