# Installation and packaging

No package should be described as device-tested until it has actually been run on that platform. The release workflow compiles and tests on Windows, macOS and Linux runners; ARM64 binaries also require device testing.

## Put Smart Search first

Move **Stremio Smart Search** to the top of your addon order so its movie and series results are easier to find at the top of search. Row placement is controlled by the Stremio client and your addon order; SSS cannot give itself priority through its manifest.

In **dx.bi**:

1. Open **Addons → Order & backup**, beside **Add addon**.
2. Move **Stremio Smart Search** to the top using the drag handle or up arrows.
3. Choose **Save order**, then run your search again.

If that control is hidden, enable **Addon order & backup** in dx.bi settings. Saving with that tool requires signing into Stremio.

**Stremio** and **Stremio Enhanced** have no built-in reorder control. Two options, also shown on the settings page:

- **Addon manager:** open the community [Stremio Addon Manager](https://github.com/pancake3000/stremio-addon-manager), sign in, choose **Load addons**, drag Stremio Smart Search to the top, then **Sync to Stremio**. It is a third-party tool, not affiliated with this project or Stremio, and it asks for your Stremio login.
- **No sign-in:** newly installed addons go to the bottom. Note your other addons' install links, remove them, then reinstall them after Stremio Smart Search.

Moving **only SSS** keeps the other addons in their existing relative order. SSS supplies search-only catalogues: it does not supply streams, metadata or subtitles, or add Home/Discover rows. Your existing playback providers continue working in their previous order. The order is a Stremio account setting and may sync to your other clients.

The companion leaves this choice to you; it does not sign into your account or reorder addons automatically.

Smart Search matches against a local, prebuilt index, avoiding an internet round trip to a search server. This can reduce search waiting; posters, metadata, streams and the client UI still have their own loading time.

## Windows

The Windows x64 preview has user-confirmed compatibility with standard Stremio, dx.bi and Stremio Enhanced. Installation and removal were also tested on a Windows PC. This confirmation covers the tested desktop clients; Windows ARM64 and additional OS/client releases still need their own verification.

The installer is per-user and requests no administrator privileges. It creates:

- **Stremio + Smart Search**: starts the helper and opens the registered Stremio app, or first-run setup if no catalogue is ready.
- **Smart Search Settings**: opens the local settings page.
- An uninstaller in Windows' installed-app list.

The optional **Start helper when I sign in** component keeps the helper available when Stremio is opened through other shortcuts. This setting is not silently enabled by the app. The portable ZIP makes no startup or shortcut changes.

Unsigned development installers can show Windows publisher/reputation prompts. Signing is a separate release task, not something GitHub Releases automatically provides. Do not claim a verified publisher without a signing certificate.

Uninstall removes the app and shortcuts, leaving the local catalogue in `%AppData%\StremioSmartSearch`, or in the folder chosen with **Change location…**. No Stremio settings or other addons are removed. See [where the catalogue is stored](CATALOGUE.md#where-the-catalogue-is-stored).

## macOS

Open the disk image and drag both **Stremio Smart Search.app** and **Stremio + Smart Search.app** to Applications. Open the first for setup. Use the second as your combined Stremio launcher afterwards.

The build workflow can produce disk images for Intel and Apple Silicon. Development apps are ad-hoc signed, not Apple notarized. Gatekeeper may require approval in Privacy & Security. A frictionless public macOS release will need Developer ID signing and notarization; this has not been completed.

## Linux

Debian/Ubuntu users can open the `.deb` through their software installer. It provides application-menu entries for settings and the combined Stremio launcher.

Other distributions can extract the portable archive and run `stremio-smart-search` for setup. `stremio-smart-search --launch` opens Stremio after starting the helper. The binary includes its runtime and does not require Node.js or Python. A desktop session with `xdg-open` and a registered Stremio protocol handler is needed for launch buttons; copying the addon URL remains a fallback.

## Launcher and portable use

The **Stremio + Smart Search** launcher starts the helper and opens your registered Stremio app. The helper stays available in the background until you choose **Quit helper** in settings. It does not monitor your processes or watching activity. Closing the browser settings page does not stop it.

The launcher remembers the app chosen in setup. App-specific buttons launch installed executables directly, and the helper does not change protocol associations. Generic `stremio://` addon links are deliberately avoided because Stremio rewrites them to HTTPS, which does not suit this local HTTP service. Keep the full `http://127.0.0.1:11479/manifest.json` address when installing manually. Some browsers ask for local-network permission for the **Install through Stremio Web** option.

Portable users can run the executable directly. Pass `--launch` to also open Stremio, or `--no-browser` to run in the background without opening settings. No Node.js, Python, Docker or API keys are needed.

## A TV, phone or another computer

See [OTHER-DEVICES.md](OTHER-DEVICES.md).

## Release readiness

Before publishing: run the platform-native tests and smoke tests, verify installer/uninstaller behavior, verify the actual Stremio addon installation prompt and results, record unsigned-platform prompts accurately, and review the package contents for data, credentials and machine-specific files. The workflow creates a **draft prerelease** for review rather than publishing a public release automatically.
