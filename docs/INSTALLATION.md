# Installation

[Download Smart Search](https://github.com/EskimoDev/stremio-smart-search/releases) for the computer where you use Stremio. SSS is a separate helper that must be running for its search rows to respond.

This guide covers **0.1.0-preview.5**. Check [Releases](https://github.com/EskimoDev/stremio-smart-search/releases) for available downloads; a draft is not public. Preview.5 adds **Start Smart Search when I sign in**, a Windows notification-area icon, a **Quit Smart Search** shortcut and the addon logo. Preview.4 added **Update automatically**, sound-alike matching and dark mode. For older versions, use the [preview.4](https://github.com/EskimoDev/stremio-smart-search/blob/v0.1.0-preview.4/docs/INSTALLATION.md) or [preview.3](https://github.com/EskimoDev/stremio-smart-search/blob/v0.1.0-preview.3/docs/INSTALLATION.md) guide.

**Bandwidth:** a new installation (preview.4 or newer) enables automatic checks every 2 days. A changed-data update downloads about 240 MB, plus about 520 MB weekly with alternative titles (roughly 3.6 or 5.8 GB/month). Existing catalogues stay manual until enabled. Disable **Update automatically** for manual-only updates; see [update behavior](CATALOGUE.md#automatic-updates).

[Choose a download](#choose-a-download) · [First setup](#first-setup) · [Addon order](#put-smart-search-first) · [Troubleshooting](#troubleshooting) · [Uninstall](#uninstalling)

## Choose a download

The release filenames include a version, an operating system and a processor type:

| Computer | Installable package | Portable alternative |
|---|---|---|
| Windows, Intel/AMD | `Windows-x64-Setup.exe` | `Windows-x64-Portable.zip` |
| Windows on ARM | `Windows-ARM64-Setup.exe` | `Windows-ARM64-Portable.zip` |
| Mac, M-series chip | `macOS-AppleSilicon.dmg` | `macOS-AppleSilicon.zip` |
| Mac, Intel | `macOS-Intel.dmg` | `macOS-Intel.zip` |
| Linux, Intel/AMD | `Linux-x64.deb` | `Linux-x64.tar.gz` |
| Linux on ARM | `Linux-ARM64.deb` | `Linux-ARM64.tar.gz` |

Windows x64 has been tested with Stremio, dx.bi and Stremio Enhanced. Linux x64 has installation and helper tests on Ubuntu 24.04. macOS has automated app-bundle checks, with interactive installation still untested. Windows and Linux ARM64 need device testing. See [platform testing](PLATFORM-TESTING.md) for the scope of each result.

No Node.js, Python, Docker or API key is needed to run the app. TV and phone setups are covered separately in [other devices](OTHER-DEVICES.md).

### Check your download

Download `SHA256SUMS.txt` from the same release. Calculate the SHA-256 hash of your package and compare it with the line for that exact filename. Replace `<download-file>` below with its name.

| System | Command |
|---|---|
| Windows PowerShell | `Get-FileHash ".\<download-file>" -Algorithm SHA256` |
| macOS Terminal | `shasum -a 256 "./<download-file>"` |
| Linux terminal | `sha256sum "./<download-file>"` |

### Windows

Run the installer. It installs for your user, without administrator privileges, and creates:

- **Stremio + Smart Search** on the desktop and Start Menu: starts the helper and opens your chosen Stremio app, or setup if no catalogue is ready.
- **Smart Search Settings** in the Start Menu: opens the settings page.
- **Quit Smart Search** in the Start Menu: stops the helper.
- An uninstaller in Windows' installed-app list.

While the helper runs, its icon sits in the notification area next to the clock (open the **^** overflow if Windows hides it there). Double-click it, or select it and press Enter, to open the dashboard. Right-click it, or press Shift+F10, for **Open dashboard** and **Quit Smart Search**.

**Start when I sign in** is an optional installer component, off by default. Select it if you want the helper available when opening Stremio through other shortcuts.

Preview installers are unsigned. If SmartScreen appears for the official download you checked above, **More info → Run anyway** continues installation.

For the portable ZIP, extract it to a folder and run `StremioSmartSearch.exe`. It creates no shortcuts or sign-in entries.

Running an older installer over a newer version installs the older app without a downgrade warning. Catalogue and settings are kept.

### macOS

Choose the package for your chip. Open the disk image and drag both **Stremio Smart Search.app** and **Stremio + Smart Search.app** to Applications. Open the first for setup; use the second as your combined launcher afterwards. The ZIP contains the same two apps.

The apps are ad-hoc signed, without a developer certificate or Apple notarization. If Gatekeeper blocks the first open, approve the app under **System Settings → Privacy & Security → Open Anyway**. This interactive flow still needs testing on a real Mac.

### Linux

For Debian/Ubuntu, install the downloaded `.deb` through your software installer, or run:

```sh
sudo apt install ./StremioSmartSearch-<version>-Linux-x64.deb
```

Replace `<version>` with the downloaded version, and use `ARM64` instead of `x64` on an ARM computer. Using `apt` also installs the `xdg-utils` dependency. The package adds **Smart Search Settings**, **Stremio + Smart Search** and **Quit Smart Search** to the app menu.

On other distributions, extract the portable archive into its own folder and run `./stremio-smart-search`. The archive has no enclosing folder. Add `--launch` to open Stremio too.

Opening browsers and folders needs a desktop session with `xdg-open`. Stremio is detected through its executable or, for standard Stremio, its Flatpak installation. The system's registered Stremio URL handler is used when no app preference has been saved. If detection fails, copy the addon link manually.

## First setup

1. **Open Smart Search.** On Windows or Linux, choose **Smart Search Settings**. On a Mac, open **Stremio Smart Search.app**. A local settings page opens in your browser.
2. **Prepare the catalogue.** On first run this is step 01 on the settings page. Choose a **Catalogue size** under **Title catalogue** if you want something other than Standard. Choose **Prepare catalogue** and wait for the status to show **Running locally**. All sizes download the same IMDb source files, a few hundred MB. Allow several minutes depending on your connection. Optional **Alternative titles** adds another source file; leave it off unless you want regional names.
3. **Choose your Stremio app.** Select **Stremio**, **dx.bi** or **Stremio Enhanced**. Its button copies the addon link and opens the app. If the app isn't detected, use **Copy link** instead.
4. **Install the addon.** In Stremio, open **Addons → Add addon**, paste the copied link and choose **Install**. Keep the full `http://127.0.0.1:11479/manifest.json` address, including `http://` and the port.
5. **Try a search.** Search for `dune 1984`, `game of thornes` or `hotd`. The settings page lists more examples, one for each kind of match. Look for the **Smart Search** movie and series rows. They may appear below your other search addons until you change the order.

For Stremio Web on the same computer, expand **Using Stremio Web or a portable app?** in settings and choose **Install through Stremio Web**. Use the same account as your desktop client to sync the addon. Your browser may ask for permission to connect to the local helper.

## Put Smart Search first

Reordering is optional. It makes the Smart Search rows easier to find; SSS cannot set its own priority through its addon manifest.

### dx.bi

1. Open **Addons → Order & backup**, beside **Add addon**.
2. Move **Stremio Smart Search** to the top using its drag handle or up arrows.
3. Choose **Save order**, then search again.

If the control is hidden, enable **Addon order & backup** in dx.bi settings. Saving requires a Stremio account.

### Standard Stremio and Stremio Enhanced

These clients do not provide dx.bi's reorder screen. You can either:

- **Use dx.bi with the same Stremio account.** Save the order there, then reopen your other client so it can load the account's updated addon order.
- **Use a community addon manager.** The [Stremio Addon Manager project](https://github.com/pancake3000/stremio-addon-manager) links to its web app. Sign in there, choose **Load Addons**, drag only Smart Search to the top, then choose **Sync To Stremio** and reopen your client. This is a separate, unofficial tool that requires access to your Stremio account.

Moving only Smart Search preserves the relative order of your other addons. SSS supplies search rows; it supplies no streams, metadata resource or subtitles and adds no Home/Discover rows. Account order may sync to your other devices. Smart Search never signs in or reorders addons for you.

## Launcher and portable use

Next time, use **Stremio + Smart Search** to start the helper and open the app you chose in setup. If no preference has been saved, it uses the registered Stremio URL handler. It does not change that association.

The helper stays running after you close the settings page or Stremio. It does not watch Stremio's processes or your playback. To stop it, choose **Quit Smart Search** in the Start Menu, Linux app menu or the Windows notification-area icon, or **Quit helper** in settings and then confirm with a second click.

Smart Search rows only appear while the helper is running. If you usually open Stremio with its own shortcut, turn on **Start Smart Search when I sign in** under **You can close this page** in settings. It is off by default. It starts this copy of the helper without opening settings or Stremio, keeps any custom `--data-dir` or `--listen` you started it with, and only changes your own user's sign-in items: the Windows Run entry also used by the installer option, `~/Library/LaunchAgents/community.eskimodev.smartsearch.plist` on macOS, or `~/.config/autostart/stremio-smart-search.desktop` on Linux. For a portable copy, turn it off before moving or deleting the folder.

To reopen settings:

- Choose **Configure** next to Smart Search in Stremio's addon list. If it was installed before that button was added, remove and reinstall **only Smart Search** to refresh its manifest.
- Open **Smart Search Settings** on Windows/Linux, or **Stremio Smart Search.app** on macOS.
- Visit `http://127.0.0.1:11479/` on the computer running the helper.

Portable builds use the same settings folder as installed builds unless you pass `--data-dir`. They do not keep data beside the executable automatically. To open both apps, run the executable with `--launch`; to keep the helper running without opening settings, use `--no-browser`. See [command-line options](ARCHITECTURE.md#command-line-options).

## Troubleshooting

**“Failed to fetch addon manifest.”** Start the helper, then open `http://127.0.0.1:11479/manifest.json` on the same computer. It should show JSON. Copy the link from settings again, keeping HTTP and the port. Generic `stremio://` addon links can be rewritten to HTTPS, which this local HTTP service does not serve.

**The Smart Search rows disappeared.** Start the helper through **Stremio + Smart Search** and search again. To avoid this, turn on **Start Smart Search when I sign in** in settings. Closing the browser is fine; quitting the helper or shutting down the host stops SSS searches until it runs again.

**The row shows “Smart Search needs setup.”** The helper is running but has no catalogue yet. Open settings and choose **Prepare catalogue**; results replace the message when it finishes.

**“Stremio Smart Search couldn't start.”** The message says why. If another program is using port 11479, close it, or quit any other copy of Smart Search started from a different folder, then open Smart Search again.

**An app is “Not detected.”** Open that app yourself, then copy and paste the addon link. Portable and unusual install locations may not be detected.

**A title is missing or the wrong one comes first.** Check your catalogue size and alternative-title language. See [coverage and ranking](CATALOGUE.md) and [known search gaps](SEARCH-QUALITY.md#remaining-weaknesses). Normal results from your other addons remain available.

### Addon logo after an update

The addon logo arrived in preview.5. These steps apply after updating from preview.4 or earlier.

The helper serves the logo at `/addon-logo.png` on the same address as its manifest; no external image host or settings-page visit is required. The addon ID and manifest address stay the same, so an existing installation can fetch the new manifest at its original URL. The helper must be running for the logo to load.

Stremio clients can retain the installed manifest, including its old or missing logo. A helper update or client restart does not guarantee that this saved copy is replaced. After updating the helper:

1. Open Smart Search's addon details. If your client offers **Update** or **Upgrade**, use that to refresh the existing addon.
2. If no update option is offered and the logo is still missing, remove **only Stremio Smart Search**, then add it again using the same full HTTP manifest link copied from the helper's settings page. Your downloaded catalogue and helper settings are retained; put SSS back in your preferred addon position if needed.

The manifest and image have a five-minute HTTP cache lifetime. This is separate from the client's saved installed-addon metadata. Client testing is recorded in [platform testing](PLATFORM-TESTING.md).

## Uninstalling

1. Before uninstalling, open the **Storage** card in settings and note the catalogue folder if you also want to delete downloaded data.
2. Choose **Quit helper** and confirm with a second click.
3. If you turned on **Start Smart Search when I sign in**, turn it off first; on macOS and Linux, uninstalling does not remove that sign-in entry.
4. Remove the app:
   - **Windows installer:** use Windows' installed-app list.
   - **macOS:** delete both Smart Search apps from Applications.
   - **Linux .deb:** use your software manager or `sudo apt remove stremio-smart-search`.
   - **Portable archive:** delete its extracted folder.
5. Remove **Stremio Smart Search** from Stremio's addon list if you no longer want its search rows.

App removal leaves catalogue and settings files behind. Their default paths are in [catalogue storage](CATALOGUE.md#where-the-catalogue-is-stored). If you chose a custom catalogue folder, settings remain in the default folder. Your Stremio settings and other addons are untouched.

