# Documentation

New here? Start with the [project README](../README.md) for screenshots and a quick install.

**Latest public release: [0.1.0-preview.5](https://github.com/EskimoDev/stremio-smart-search/releases/tag/v0.1.0-preview.5).** These pages also track [unreleased changes on main](../CHANGELOG.md#unreleased), labelled where behavior differs. For the exact instructions shipped with a download, use its tag-pinned docs: [preview.5](https://github.com/EskimoDev/stremio-smart-search/tree/v0.1.0-preview.5/docs), [preview.4](https://github.com/EskimoDev/stremio-smart-search/tree/v0.1.0-preview.4/docs) or [preview.3](https://github.com/EskimoDev/stremio-smart-search/tree/v0.1.0-preview.3/docs).

## Setup and everyday use

| I want to… | Read |
|---|---|
| Install SSS, connect it to Stremio or fix setup problems | [Installation](INSTALLATION.md) |
| Put Smart Search above my other search rows | [Addon order](INSTALLATION.md#put-smart-search-first) |
| Choose a catalogue size or enable regional titles | [Catalogue](CATALOGUE.md) |
| Move downloaded data to another drive | [Catalogue storage](CATALOGUE.md#where-the-catalogue-is-stored) |
| Use a TV, phone or home server | [Other devices](OTHER-DEVICES.md) |
| Check which platforms have actually been tested | [Platform testing](PLATFORM-TESTING.md) |

## Results and technical reference

| Topic | Read |
|---|---|
| Memory, search latency, download sizes and reproduction steps | [Performance](PERFORMANCE.md) |
| Latest measured accuracy, scoring rules and known search gaps | [Search quality](SEARCH-QUALITY.md) |
| Examples compared with Stremio’s default search | [Cinemeta comparison](SEARCH-QUALITY.md#compared-with-stremios-default-search-2026-11-02) |
| Measuring a build and comparing two versions | [Benchmarking](BENCHMARKING.md) |
| How the helper works, its HTTP routes and command-line options | [Architecture](ARCHITECTURE.md) |
| Local setup-page styling and interaction rules | [Design](DESIGN.md) |
| IMDb sources and data terms | [Title data](../DATA.md) |

## Working on the project

See [Contributing](../CONTRIBUTING.md) for development and release checks, [Scripts](../scripts/README.md) for build and test helpers, and [Tools](../tools/README.md) for packaging and evaluation commands.

The `images/` folder holds README screenshots. `benchmarks/` holds historical Windows measurements and synthetic Linux results; see [measurement records](PERFORMANCE.md#measurement-records) for which figures each file supports. Production catalogues are kept outside the repository.
