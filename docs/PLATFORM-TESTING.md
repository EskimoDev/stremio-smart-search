# Platform testing

Historical device/package testing recorded on 2026-11-01, with later automated benchmark coverage linked below. The latest public download is preview.6; these records do not establish a new interactive device test for its packages or every current-main change.

Windows includes a test of the published `0.1.0-preview.3` packages. The sections below distinguish source/package checks, release-download tests and remaining manual checks.

| Platform | Status |
|---|---|
| Windows x64 | Device-tested by the maintainer, including the published `0.1.0-preview.3` download (see below) |
| Windows ARM64 | Builds and packages; not run on a device |
| Linux x64 | Verified on Ubuntu 24.04 (see below) |
| Linux ARM64 | Builds and packages; not run on a device |
| macOS (Apple Silicon, Intel) | Untested on a real Mac. CI builds, signs ad hoc and smoke-tests the app bundles and DMG |
| Fire TV / Android TV | No native helper. Access from a computer or home server still needs client testing; see below |

The developer benchmark also runs the helper natively on hosted Windows, Linux and macOS runners, ARM64 included; see [its coverage](BENCHMARKING.md#platform-coverage). Those are automated runs on shared machines, not device tests, so they do not change the table above.

## Windows x64 (published 0.1.0-preview.3)

Tested 2026-11-01 on Windows x64 with the files downloaded from the public release, as an upgrade over an earlier build with an Expanded catalogue.

- The Setup.exe and Portable.zip SHA-256 hashes match `SHA256SUMS.txt`.
- A browser download shows SmartScreen's "Windows protected your PC" prompt; **More info → Run anyway** continues.
- The upgrade kept the existing catalogue (no rebuild needed). The manifest, status API and settings page report `0.1.0-preview.3`.
- Searching `HOTD` in Stremio returns *House of the Dragon*, **Configure** opens the settings page, and **Stremio + Smart Search** starts the helper and opens dx.bi.
- The portable build runs from an extracted folder without adding Start Menu shortcuts or sign-in entries.

### Addon logo (preview.5, tested on a source build)

On 2026-11-01, the maintainer supplied a screenshot confirming the official SSS logo in dx.bi's installed-addon list using a local `0.1.0-dev` build. This confirms the installed-list appearance in dx.bi; it does not establish automatic refresh of an older installed manifest. A separate browser render check covered 32, 48, 64, 96 and 256 pixels on light and dark backgrounds. Standard Stremio and Stremio Enhanced have not been visually checked for this change.

Automated Go tests check absolute logo URLs for loopback, localhost, IPv6, LAN and direct HTTPS origins; fetching the 256×256 PNG before catalogue setup; image content type; public GET/HEAD/OPTIONS handling; and continued protection of settings routes. These are protocol checks, not additional client-device tests. See [manifest refresh instructions](INSTALLATION.md#addon-logo-after-an-update).

## Linux x64

Tested on Ubuntu 24.04.5 x64 with the packages built by `tools/package`.

- `go vet`, `go test ./...` and `node scripts/ui-test.mjs` pass.
- linux/amd64 and linux/arm64 builds are static binaries (about 7 MB); the .deb and .tar.gz are about 3 MB each.
- `sudo apt install ./StremioSmartSearch-<version>-Linux-x64.deb` installs `/usr/bin/stremio-smart-search`, two menu entries and the icon at the time of this test (preview.5 also adds **Quit Smart Search**), and pulls in `xdg-utils`. Plain `dpkg -i` stops with an unmet dependency when `xdg-utils` is missing, so prefer `apt install ./file.deb`.
- With the test catalogue: the status API returns 200 with the control key and 403 without it. `/manifest.json` lists both catalogs. `HOTD`, `interstelar` and `LOTR` return the expected first result. The settings page and its assets load, and `/api/quit` stops the helper cleanly.
- Search latency was about 0.5 ms and resident memory about 9 MB with the test catalogue. These are not full-catalogue figures.
- Starting without a browser or display works; the failed browser launch is ignored.

Not covered: menu entries and Stremio launch in a real desktop session, ARM64 at runtime, and the full IMDb catalogue.

## macOS

No Mac was available. The `test` job already runs the Go tests, the UI test and `scripts/smoke.ps1` on `macos-latest`, but only against the bare Apple Silicon binary. The `macos-app` job in `ci.yml` adds Apple Silicon and Intel runners, ad-hoc signs and verifies both app bundles, runs the smoke test against the binary inside `Stremio Smart Search.app`, and runs both bundled binaries from a mounted DMG.

CI cannot show the Gatekeeper prompt after a browser download, or the launcher opening Stremio. Those still need a real Mac:

- [ ] Open the DMG for the right chip and drag the app to Applications.
- [ ] Note the first-open Gatekeeper prompt and how to get past it (right-click > Open, or Privacy & Security > Open Anyway).
- [ ] The settings page opens, the catalogue builds and the search test works.
- [ ] "Stremio + Smart Search" opens Stremio with the addon working.
- [ ] After quitting and reopening, the catalogue is still there.

## Fire TV and Android TV (LAN mode)

The companion does not run on the TV. It runs on a computer on the same network, and Stremio on the TV installs the addon from that computer's address.

By default the helper listens on `127.0.0.1:11479`, so other devices cannot reach it. Starting it with `--listen 0.0.0.0:11479` makes it reachable. Tested from a non-loopback address, `/manifest.json` and catalog searches return 200, while the settings page and every `/api/` route return 403 even with the control key. Only addon routes are exposed (manifest, catalogue search and, since preview.5, logo); settings stay on the host computer.

To try it:

- [ ] Quit Smart Search, then run `StremioSmartSearch.exe --listen 0.0.0.0:11479` (or the platform binary with the same flag).
- [ ] Allow the firewall prompt for private networks only.
- [ ] Find the computer's LAN IP address (for example `ipconfig` on Windows).
- [ ] From a phone on the same Wi-Fi, `http://<ip>:11479/manifest.json` shows JSON and `http://<ip>:11479/` shows "Settings are available on the host computer only".
- [ ] In Stremio on the TV, install `http://<ip>:11479/manifest.json`, or install it from the same Stremio account elsewhere so it syncs.
- [ ] Note whether Stremio on the TV accepts a plain-HTTP LAN address. If it requires HTTPS, LAN mode needs a different approach.
- [ ] Search on the TV for `dune 1984`, `HOTD` and `LOTR`.
- [ ] Turn the computer off and check how Stremio behaves when the addon is unreachable.

Gaps before LAN mode is a supported feature:

1. The listen address is a command-line flag only. The installed shortcuts and the installer do not set it, and it is not remembered.
2. The settings page and startup line always show the `127.0.0.1` addon link, not the LAN link.
3. The manifest description says the companion must be on "this computer".
4. On Windows, a private-network firewall rule could be added by the installer.

## Small packaging notes

- The Linux .tar.gz has no top-level folder, so it extracts into the current directory.
- The published preview.3 macOS app bundles have no icon (`Contents/Resources/*.icns`). Preview.4 and later package `AppIcon.icns`; the icon's desktop appearance still needs native verification.
