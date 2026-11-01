# Title catalogue

Smart Search matches against a catalogue built on your own computer from IMDb's public datasets. This page covers what goes into it, the optional alternative titles, where it is stored and how it stays up to date. For memory and speed per size, see [PERFORMANCE.md](PERFORMANCE.md).

## Catalogue sizes

Choose the catalogue size in setup:

| Size | Movie ceiling | Series ceiling | Minimum IMDb votes |
| --- | ---: | ---: | ---: |
| Compact | 25,000 | 25,000 | 250 |
| Standard (new-setup default) | 100,000 | 100,000 | 25 |
| Expanded | 250,000 | 250,000 | 5 |

These are ceilings, not promised counts. Titles are selected by vote count from qualifying movies, TV movies, series and miniseries. Adult-marked titles and individual episodes are excluded; titles without sufficient votes are excluded even with Expanded. Primary and original titles are always searchable. Optionally, **Alternative titles** adds regional and translated titles in one chosen language, for example *La Reine des neiges* for *Frozen* or *Zootropolis* for *Zootopia*. Coverage depends on IMDb's data; this does not promise every regional spelling or alternate title. Normal results from other addons remain available.

On the 2026-11-01 IMDb data, the three sizes held **50,000, 184,606 and 390,470 titles**. Memory and speed for each are in [performance](PERFORMANCE.md).

## Alternative titles (optional)

Under **Catalogue → Alternative titles**, choose a language. Where titles commonly differ by country, you can also choose a **Regional preference**, such as Spain or Mexico for Spanish, or France or Canada for French. Then choose **Apply titles & rebuild**. Search then also matches those titles, including misspellings and missing accents. Results keep their normal IMDb identifiers and names, and English and original titles keep working. Off is the default, and existing catalogues keep their current behaviour until you change it.

A manual preparation or refresh adds IMDb's alternative-title file: about 520 MB more to download (2026-11-01), streamed and discarded like the other files. The recorded preview.3 Windows run took about 7 seconds longer, excluding the download. With French titles, Standard used 58 MiB instead of 53 MiB and Expanded 105 MiB instead of 97 MiB, with search speed about the same; see [performance](PERFORMANCE.md#alternative-titles). Only the chosen language is kept, and no translation or other service is involved.

## Matching and ranking

Exact canonical titles and longer alternative titles rank ahead of acronym and fuzzy matches; popularity decides between exact matches. Short Latin alternative titles (up to five letters after removing spaces) share the acronym ranking band instead. Acronyms can have several legitimate meanings. Typo matching has bounded edit distances and work limits. It does not generate fictional titles or use an LLM.

To pick one of several films with the same name, add its release year: `dune 1984`, `halloween 2018`, `it 2017`. If no title matches that year, the year is ignored. Measured accuracy per catalogue size, and the known weak spots, are in [search quality](SEARCH-QUALITY.md).

## Where the catalogue is stored

By default the catalogue lives in the helper's settings folder:

| System | Folder |
| --- | --- |
| Windows | `%AppData%\StremioSmartSearch` |
| macOS | `~/Library/Application Support/StremioSmartSearch` |
| Linux | `~/.config/StremioSmartSearch` (or `$XDG_CONFIG_HOME/StremioSmartSearch`) |

The **Storage** card in settings shows catalogue and settings sizes, free space on the catalogue drive, and a Go runtime memory estimate that includes the search index. That estimate is not the same counter as process memory in Task Manager or the performance tables. **Open folder** (**Show in Explorer** on Windows, **Show in Finder** on a Mac) opens the catalogue folder.

During a save, the old catalogue is temporarily renamed to `catalog.json.gz.previous` so it can be recovered if replacement is interrupted. A successful save removes that file. **Previous catalogue** appears in Storage only when a recovery file exists; it is not a permanent backup or version history.

To keep the catalogue on another drive, choose **Change location…** and pick a folder. Smart Search creates a `Stremio Smart Search` folder inside it, so choosing a drive root never scatters files there. It checks for free space, copies the catalogue, switches over, then removes only its own catalogue files from the old folder. Search keeps working throughout because the index is in memory. **Use default** moves it back. Settings and the private control key always stay in the settings folder. On Linux the folder picker needs `zenity` or `kdialog`; without one you can type a full folder path instead. A move is refused while a catalogue update is running.

Uninstalling leaves the catalogue where it is. Delete the folder yourself if you no longer need it.

## Updating and changing size

The public preview.3 download updates manually through **Refresh catalogue**. In unreleased main, that button is **Refresh now**, and optional automatic updates are described below.

To change size, select the option and choose **Apply size & rebuild**. If you also change alternative titles, the button reads **Apply changes & rebuild**. On main, **Refresh now** downloads the latest IMDb data with your current choices. The current index keeps serving during preparation; new choices are saved with the replacement catalogue only after success. A failed refresh keeps the previous working catalogue. Existing catalogues retain their saved size. Legacy catalogues without a size field are treated as Compact.

### Automatic updates

**Unreleased main only; absent from the preview.3 download.**

**Update automatically** keeps the catalogue current without you thinking about it. It is on for new installations; an installation that already had a catalogue before this option existed stays manual until you switch it on, so nobody's data use changes unannounced. Choose how often under **Check for new titles**: every day, every 2 days (default), every 3 days or every week.

How it behaves:

- There is no separate service, scheduled task or start-up entry. Updates happen only while the helper is running, which it normally is in the background.
- Start-up never waits for an update. A saved catalogue is searchable at once, and the first check comes a few minutes after start-up. While the helper keeps running it re-evaluates every half hour; nothing happens unless an update is due.
- When the chosen interval has passed since the last successful update, Smart Search sends one `HEAD` request per IMDb file with the validators it saved last time (`If-None-Match`, `If-Modified-Since`). If nothing changed, it downloads nothing and asks again no sooner than six hours later. If IMDb's reply has no validators to compare, it falls back to the schedule alone: one download per interval.
- IMDb normally publishes new files every day, so at the default 2-day interval most checks do lead to a download: about 240 MB (ratings 9 MB, titles 230 MB in the 1 November 2026 snapshot). IMDb only publishes whole files, so there is no smaller "changes only" download.
- With alternative titles on, routine updates keep the alternative titles you already have, matched by IMDb ID, and download IMDb's 520 MB alternative-title file only once it is a week old. A title that is new since then is still found by its main and original titles, and gets its regional names at the next weekly download. **Refresh now** and any size or language change always download everything.
- Roughly per month: every 2 days 3.6 GB, or 5.8 GB with alternative titles; every day 7.1 GB, or 9.4 GB; every week 1 GB, or 3.3 GB. These are rough transfer estimates, not a cap; unchanged files, retries, time offline and manual refreshes change the total.
- Only one update runs at a time. **Refresh now** with the same size and language joins an automatic update while it can still request fresh alternative titles. If that update has already carried the old alternatives forward and is finishing, settings ask you to choose **Refresh now** again shortly.
- The new catalogue replaces the old one only after it has been completely downloaded, parsed, validated and saved. An automatic update also refuses a replacement with less than half the current number of titles, which would point to a partial file from IMDb.
- After a failure (offline, IMDb unavailable, a cut-off or damaged download), retries back off by 1 hour, then 2, 4, 8, 16 and at most 24 hours; the next check also respects the selected update interval. This is saved, so restarting the helper does not repeat a large download straight away. An update cut short by a crash or forced close counts as a failure too.
- Saved times that lie in the future, for example after the clock was set back, are treated as now, so a clock change cannot pause updates for long or cause more than one extra update.
- On Windows, a connection marked as metered (Settings → Network → Metered connection) pauses automatic updates. macOS and Linux have no detection here, so updates follow the schedule on any connection; turn them off if you are on a capped connection. **Refresh now** always works.

The settings are stored in `updates.json` and the record of checks, failures and IMDb validators in `update-state.json`, both in the settings folder.

IMDb data terms are covered in [DATA.md](../DATA.md). The matching internals are in [ARCHITECTURE.md](ARCHITECTURE.md#search).
