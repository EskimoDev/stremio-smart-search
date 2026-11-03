# Title data

The application source is MIT licensed. That license does not cover IMDb data or third-party artwork.

The initial catalogue is prepared only after the user chooses **Prepare catalogue**. Subsequent downloads can be manual or automatic, as described below. The following files are streamed directly over HTTPS:

- `https://datasets.imdbws.com/title.ratings.tsv.gz`
- `https://datasets.imdbws.com/title.basics.tsv.gz`
- `https://datasets.imdbws.com/title.akas.tsv.gz`, only when **Alternative titles** is set to a language. It is not downloaded otherwise.

**Since preview.4:** optional automatic updates also contact IMDb in the background while the helper runs. New installations default to a 48-hour interval; existing catalogues stay manual until enabled. Conditional `HEAD` requests check for changes before full downloads; without usable validators the schedule permits a download. Routine updates carry existing alternative titles forward, then download `title.akas` at the next due update once it is at least a week old. Manual **Refresh now** downloads all selected sources. See [automatic updates and bandwidth](https://github.com/EskimoDev/stremio-smart-search/blob/main/docs/CATALOGUE.md#automatic-updates).

HTTP validators, dataset sizes and update timestamps are saved in the helper's local update-state file, separately from the catalogue. They contain no Stremio account or viewing data.

IMDb describes these as datasets for personal and non-commercial use and provides the applicable terms at [IMDb Non-Commercial Datasets](https://developer.imdb.com/non-commercial-datasets/). `title.akas` is part of the same dataset set and is covered by the same terms; enabling alternative titles does not change the licence position. The dataset page (checked 2026-11-01) permits local copies subject to its personal, non-commercial terms. It refers to IMDb's [Non-Commercial Licensing](https://help.imdb.com/article/imdb/general-information/can-i-use-imdb-data-in-my-software/G5JTRESSHJBBHTGX) and [copyright/licence](https://www.imdb.com/Copyright) pages. It does not prescribe a separate attribution line. Users should consult those terms. This project does not redistribute those datasets or generated production snapshots in its repository, installers or releases.

Compressed source streams are read on the user's device. The stored catalogue contains identifiers, primary/original titles, type, year, rating and vote count for a bounded selection. With alternative titles enabled, it also stores up to four regional or translated titles per selected movie or series in the chosen language, within a per-size limit, plus the chosen language and region. Alternative titles for other languages, episodes and unselected titles are discarded while streaming. The raw `title.akas` file is never written to disk. No user ratings, demographic data, account details or viewing history are involved.

The small test fixtures are handwritten, include illustrative vote counts and one fictional punctuation example, and are not a production catalogue. `testdata/aliases` holds IMDb-shaped rows written for tests, not copied from IMDb. Its localized titles are well-known illustrative examples, its vote counts are invented, and one identifier is fictional. None of these fixtures is loaded automatically in a normal installation.

Result poster links use `images.metahub.space`, following the style used in Stremio's addon examples. Those images are not bundled. Stremio's normal metadata and stream providers supply the rest of the viewing experience.
