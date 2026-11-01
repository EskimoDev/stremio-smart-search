# Title catalogue

Smart Search matches against a catalogue built on your own computer from IMDb's public datasets. This page covers what goes into it, the optional alternative titles and how updates work. For memory and speed per size, see [PERFORMANCE.md](PERFORMANCE.md).

## Catalogue sizes

Choose the catalogue size in setup:

| Size | Movie ceiling | Series ceiling | Minimum IMDb votes |
| --- | ---: | ---: | ---: |
| Compact | 25,000 | 25,000 | 250 |
| Standard (new-setup default) | 100,000 | 100,000 | 25 |
| Expanded | 250,000 | 250,000 | 5 |

These are ceilings, not promised counts. Titles are selected by vote count from qualifying movies, TV movies, series and miniseries. Adult-marked titles and individual episodes are excluded; titles without sufficient votes are excluded even with Expanded. Primary and original titles are always searchable. Optionally, **Alternative titles** adds regional and translated titles in one chosen language, for example *La Reine des neiges* for *Frozen* or *Zootropolis* for *Zootopia*. Coverage depends on IMDb's data; this does not promise every regional spelling or alternate title. Normal results from other addons remain available.

In the 2026-11-01 measurements, the three sizes contained **50,000 / 184,588 / 390,449 titles** respectively. See the [resource comparison and reproducible measurements](PERFORMANCE.md) before choosing a larger option, particularly on a low-memory device.

## Alternative titles (optional)

Under **Catalogue → Alternative titles**, choose a language. Where titles commonly differ by country, you can also choose a **Regional preference**, such as Spain or Mexico for Spanish, or France or Canada for French. Then choose **Apply titles & rebuild**. Search then also matches those titles, including misspellings and missing accents. Results keep their normal IMDb identifiers and names, and English and original titles keep working. Off is the default, and existing catalogues keep their current behaviour until you change it.

This adds IMDb's alternative-title file to each preparation: about 520 MB more to download (2026-11-01), roughly tripling the download, and it is streamed and discarded like the other files. Preparation takes about 10 seconds longer. With French titles, measured RAM rose by about 8–10 MiB (Compact) and 20–30 MiB (Standard). Search speed was unchanged on those sizes; Expanded, already the most memory-hungry, became slightly slower, so the settings page recommends Standard when alternative titles are on. See [performance](PERFORMANCE.md#alternative-titles). Only the chosen language is kept. It is matched against your own installed catalogue, not translated, and no other service is involved.

## Matching and ranking

Exact titles are ranked first, whether canonical or, with alternative titles on, regional; popularity decides between them. Acronyms can have several legitimate meanings; popularity helps rank the alternatives. Typo matching has bounded edit distances and work limits. It does not generate fictional titles or use an LLM.

To pick one of several films with the same name, add its release year: `dune 1984`, `halloween 2018`, `it 2017`. If no title matches that year, the year is ignored. Measured accuracy per catalogue size, and the known weak spots, are in [search quality](SEARCH-QUALITY.md).

## Updating and changing size

Catalogue updates are manual from settings. They do not run during every startup or search. To change size, select the option and choose **Apply size & rebuild**. The current index keeps serving during preparation; the new size is saved with the replacement catalogue only after success. A failed refresh keeps the previous working catalogue. Existing catalogues retain their Compact selection until you change it.

IMDb data terms are covered in [DATA.md](../DATA.md). The matching internals are in [ARCHITECTURE.md](ARCHITECTURE.md#search).
