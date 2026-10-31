# Title data

The application source is MIT licensed. That license does not cover IMDb data or third-party artwork.

On the user's explicit **Prepare catalogue** or **Refresh catalogue** action, the application downloads these public sources directly over HTTPS:

- `https://datasets.imdbws.com/title.ratings.tsv.gz`
- `https://datasets.imdbws.com/title.basics.tsv.gz`

IMDb describes these as datasets for personal and non-commercial use and provides the applicable terms at [IMDb Non-Commercial Datasets](https://developer.imdb.com/non-commercial-datasets/). Users should consult those terms. This project does not redistribute those datasets or generated production snapshots in its repository, installers or releases.

Compressed source streams are read on the user's device. The stored catalogue contains identifiers, primary/original titles, type, year, rating and vote count for a bounded selection. No user ratings, demographic data, account details or viewing history are involved.

The small test fixture is handwritten, includes illustrative vote counts and one fictional punctuation example, and is not a production catalogue. It is never loaded automatically in a normal installation.

Result poster links use `images.metahub.space`, following the style used in Stremio's addon examples. Those images are not bundled. Stremio's normal metadata and stream providers supply the rest of the viewing experience.
