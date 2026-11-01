# Documentation

New here? Start with the [project README](../README.md) for screenshots and a quick install.

The public download is **0.1.0-preview.3**. These guides also cover development on `main`; sound-alike search, automatic updates, dark mode and the new icons are marked as unreleased. See the [changelog](../CHANGELOG.md#unreleased) before expecting a newer feature in an installed download.

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
| Accuracy cases, scoring rules and known search gaps | [Search quality](SEARCH-QUALITY.md) |
| How the helper works, its HTTP routes and command-line options | [Architecture](ARCHITECTURE.md) |
| Local setup-page styling and interaction rules | [Design](DESIGN.md) |
| IMDb sources and data terms | [Title data](../DATA.md) |

## Working on the project

See [Contributing](../CONTRIBUTING.md) for development and release checks, [Scripts](../scripts/README.md) for build and test helpers, and [Tools](../tools/README.md) for packaging and evaluation commands.

The `images/` folder holds README screenshots. `benchmarks/` holds historical Windows measurements and synthetic Linux results; see [measurement records](PERFORMANCE.md#measurement-records) for which figures each file supports. Production catalogues are kept outside the repository.
