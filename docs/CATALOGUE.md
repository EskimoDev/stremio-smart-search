# Title catalogue

Smart Search matches against a catalogue built on your own computer from IMDb's public datasets. This page covers what goes into it, the optional alternative titles, where it is stored and how updates work. For memory and speed per size, see [PERFORMANCE.md](PERFORMANCE.md).

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

This adds IMDb's alternative-title file to each preparation: about 520 MB more to download (2026-11-01), streamed and discarded like the other files. Preparation takes about 7 seconds longer. With French titles, Standard uses 58 MiB instead of 53 MiB and Expanded 105 MiB instead of 97 MiB, with search speed about the same; see [performance](PERFORMANCE.md#alternative-titles). Only the chosen language is kept, and no translation or other service is involved.

## Matching and ranking

Exact titles are ranked first, whether canonical or, with alternative titles on, regional; popularity decides between them. Acronyms can have several legitimate meanings; popularity helps rank the alternatives. Typo matching has bounded edit distances and work limits. It does not generate fictional titles or use an LLM.

To pick one of several films with the same name, add its release year: `dune 1984`, `halloween 2018`, `it 2017`. If no title matches that year, the year is ignored. Measured accuracy per catalogue size, and the known weak spots, are in [search quality](SEARCH-QUALITY.md).

## Where the catalogue is stored

By default the catalogue lives in the helper's settings folder:

| System | Folder |
| --- | --- |
| Windows | `%AppData%\StremioSmartSearch` |
| macOS | `~/Library/Application Support/StremioSmartSearch` |
| Linux | `~/.config/StremioSmartSearch` (or `$XDG_CONFIG_HOME/StremioSmartSearch`) |

The **Storage** card in settings shows the space used by the catalogue, the previous catalogue kept for recovery and the settings, plus the free space on that drive and the helper's current memory, including the search index. **Open folder** (**Show in Explorer** on Windows, **Show in Finder** on a Mac) opens the folder.

To keep the catalogue on another drive, choose **Change location…** and pick a folder. Smart Search creates a `Stremio Smart Search` folder inside it, so choosing a drive root never scatters files there. It checks for free space, copies the catalogue, switches over, then removes only its own catalogue files from the old folder. Search keeps working throughout because the index is in memory. **Use default** moves it back. Settings and the private control key always stay in the settings folder. On Linux the folder picker needs `zenity` or `kdialog`; without one you can type a full folder path instead. A move is refused while a catalogue update is running.

Uninstalling leaves the catalogue where it is. Delete the folder yourself if you no longer need it.

## Updating and changing size

Catalogue updates are manual from settings. They do not run during every startup or search. To change size, select the option and choose **Apply size & rebuild**. The current index keeps serving during preparation; the new size is saved with the replacement catalogue only after success. A failed refresh keeps the previous working catalogue. Existing catalogues retain their Compact selection until you change it.

IMDb data terms are covered in [DATA.md](../DATA.md). The matching internals are in [ARCHITECTURE.md](ARCHITECTURE.md#search).
