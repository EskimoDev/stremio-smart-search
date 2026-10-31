# Installation and packaging

No package should be described as device-tested until it has actually been run on that platform. The release workflow compiles and tests on Windows, macOS and Linux runners; ARM64 binaries also require device testing.

## Recommended search row order

After installing the addon, move **Stremio Smart Search** to the top of your addon order. In dx.bi, use **Addons → Order & backup**, move SSS to the top, then **Save order**. Controls vary by client; see [the ordering guide](../README.md#put-smart-search-first-recommended).

Move only SSS and keep other addons in their existing relative order. Its manifest provides search-only catalogues and no stream, metadata or subtitle resource, so this does not reprioritize your playback providers or add Home/Discover rows. The order is stored with your Stremio account and may sync to other clients. The helper does not change it automatically.

SSS matches against a local, prebuilt index, avoiding an internet round trip to a search server. This can reduce search waiting; posters, metadata, streams and the client UI still have their own loading time.

## Windows

The installer is per-user and requests no administrator privileges. It creates:

- **Stremio + Smart Search**: starts the helper and opens the registered Stremio app, or first-run setup if no catalogue is ready.
- **Smart Search Settings**: opens the local settings page.
- An uninstaller in Windows' installed-app list.

The optional **Start helper when I sign in** component keeps the helper available when Stremio is opened through other shortcuts. This setting is not silently enabled by the app. The portable ZIP makes no startup or shortcut changes.

Unsigned development installers can show Windows publisher/reputation prompts. Signing is a separate release task, not something GitHub Releases automatically provides. Do not claim a verified publisher without a signing certificate.

Uninstall removes the app and shortcuts, leaving the local catalogue in the user's configuration folder. No Stremio settings or other addons are removed.

## macOS

Open the disk image and drag both **Stremio Smart Search.app** and **Stremio + Smart Search.app** to Applications. Open the first for setup. Use the second as your combined Stremio launcher afterwards.

The build workflow can produce disk images for Intel and Apple Silicon. Development apps are ad-hoc signed, not Apple notarized. Gatekeeper may require approval in Privacy & Security. A frictionless public macOS release will need Developer ID signing and notarization; this has not been completed.

## Linux

Debian/Ubuntu users can open the `.deb` through their software installer. It provides application-menu entries for settings and the combined Stremio launcher.

Other distributions can extract the portable archive and run `stremio-smart-search` for setup. `stremio-smart-search --launch` opens Stremio after starting the helper. The binary includes its runtime and does not require Node.js or Python. A desktop session with `xdg-open` and a registered Stremio protocol handler is needed for launch buttons; copying the addon URL remains a fallback.

## A TV, phone or another computer

The companion currently runs on desktop operating systems. For another device to use a home-hosted copy, the host needs to remain online and the addon address must be reachable by that client. Account synchronization does not install or start the companion on that device.

For HTTPS-capable hosting, run the helper behind an HTTPS reverse proxy. Only addon routes should be exposed: `/manifest.json` and `/catalog/*`. The settings interface intentionally accepts loopback clients only. Avoid assuming plain HTTP LAN URLs are accepted by every Stremio client.

Native Android/TV packages, iOS/tvOS companions, a hosted public service and automatic cloud deployment are outside this first preview.

## Release readiness

Before publishing: run the platform-native tests and smoke tests, verify installer/uninstaller behavior, verify the actual Stremio addon installation prompt and results, record unsigned-platform prompts accurately, and review the package contents for data, credentials and machine-specific files. The workflow creates a **draft prerelease** for review rather than publishing a public release automatically.
