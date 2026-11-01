# Search quality

How accurately Smart Search finds the title someone meant, measured on real
catalogues with known IMDb answers.

## The cases

[`testdata/quality/cases.json`](../testdata/quality/cases.json) holds 248
curated queries in 15 categories:

| Category | What it tests | Example |
| --- | --- | --- |
| exact | canonical title as typed | `the shawshank redemption` |
| original | IMDb original title of a non-English film | `cidade de deus` |
| fuzzy | one or two wrong, missing or extra letters | `shawshank redemtion` |
| transposition | two swapped letters | `game of thornes` |
| acronym | initials | `himym`, `hotd` |
| joined | words typed without spaces | `strangerthings` |
| punctuation | hyphens, colons, apostrophes, stylised names | `wall-e`, `oceans eleven` |
| accent | accents typed or left out | `les miserables`, `léon` |
| short | one short or very common word | `up`, `her`, `dark` |
| similar | remakes, sequels and shared names | `the office`, `toy story 3` |
| year | a title followed by its release year | `dune 1984` |
| alias | regional title (fr, en, es, de) | `le trone de fer` |
| alias-fuzzy | misspelled regional title | `la rene des neige` |
| non-latin | Japanese titles | `千と千尋の神隠し` |
| negative | queries with no right answer | `qzxqvvnonexistent` |

Each case lists every acceptable answer as an IMDb ID together with its title
and start year. Where several answers are right, such as both versions of *The
Office*, top-1 counts any of them. Cases taken from existing tests or the
benchmark's quality checks say so in `source`. The rest were curated by hand.

**Ground truth is checked, not assumed.** Before scoring, the harness checks
every expected ID against the catalogue being scored, which is built from
IMDb's own data. The type, title (primary or original) and year must all
match, or the run stops with a ground-truth error. A regional-title case only
runs on a catalogue built with that language, and only if its title is among
the regional titles stored for that film. A title missing from a smaller
catalogue counts as out of coverage, not as a miss. For this baseline, all 121
IDs were also checked against IMDb's `title.basics` file. That check caught
one wrong case before any scoring: *Harry Potter and the Philosopher's Stone*
is a regional title in IMDb, not the original title. It moved to the English
regional cases.

## Metrics

- **Top-1**: an acceptable answer is first.
- **Recall@5**: an acceptable answer is in the first five, about one Stremio
  row without scrolling.
- **MRR**: mean of 1/rank, with rank taken over the first 20 results.
- **Missing**: no acceptable answer in the first 20.
- **False positives**: results in the first five that are known to be wrong.
  For a negative case that is any result. Otherwise it is a listed `forbid`
  fragment: "hotd" must not list *Hot Fuzz*, *Robin Hood* or *Kuch Kuch Hota
  Hai*.
- **Ranking regressions**: with `-baseline`, every case whose rank got worse
  or that gained a false positive, listed by name.

The case suite tests intent that a person judged. The benchmark's generated
`-phase accuracy` queries are complementary: thousands of typo, prefix and
joined forms derived from the catalogue itself. Both are reported below.

## Reproduce

```sh
go run ./tools/quality -v -out results/after -baseline results/before \
    path/to/compact.json.gz path/to/standard-fr.json.gz ...
```

Snapshots come from `tools/benchmark -phase prepare` (see
[PERFORMANCE.md](PERFORMANCE.md#reproduce)) and must stay outside the
repository. The harness is deterministic: the same build and snapshot always
give the same report. `go test ./internal/quality` checks the case file and
the metric code without real data.

## Results, 2026-11-01

These are real IMDb snapshots prepared on 2026-11-01: Compact, Standard and
Expanded, French versions of all three, and Standard with English, Spanish
(all regions and Spain only), German and Japanese regional titles. "Before"
is the compact-index build ([#7](https://github.com/EskimoDev/stremio-smart-search/pull/7)),
which returns the same results as the previous release. "After" includes the
fixes below.

| Catalogue | Cases | Top-1 | Recall@5 | MRR | Missing | False-positive cases |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Compact | 194 | 92.7% → 98.4% | 93.8% → 99.5% | 0.932 → 0.990 | 12 → 1 | 1 → 0 |
| Standard | 194 | 92.2% → 97.9% | 93.8% → 99.5% | 0.930 → 0.987 | 12 → 1 | 0 → 0 |
| Expanded | 194 | 91.1% → 96.9% | 93.8% → 99.5% | 0.924 → 0.982 | 12 → 1 | 0 → 0 |
| Compact, French | 216 | 92.5% → 98.1% | 93.9% → 99.1% | 0.932 → 0.986 | 13 → 2 | 1 → 0 |
| Standard, French | 216 | 92.1% → 97.7% | 93.9% → 99.1% | 0.929 → 0.984 | 13 → 2 | 0 → 0 |
| Expanded, French | 216 | 90.2% → 96.7% | 93.5% → 99.1% | 0.917 → 0.979 | 14 → 2 | 0 → 0 |
| Standard, English | 198 | 92.3% → 98.0% | 93.9% → 99.5% | 0.931 → 0.987 | 12 → 1 | 0 → 0 |
| Standard, Spanish | 203 | 91.0% → 98.0% | 93.5% → 99.5% | 0.923 → 0.988 | 13 → 1 | 0 → 0 |
| Standard, Spanish (Spain) | 203 | 91.0% → 98.0% | 93.5% → 99.5% | 0.923 → 0.988 | 13 → 1 | 0 → 0 |
| Standard, German | 205 | 92.6% → 98.0% | 94.1% → 99.5% | 0.933 → 0.988 | 12 → 1 | 0 → 0 |
| Standard, Japanese | 202 | 92.5% → 98.0% | 94.0% → 99.5% | 0.932 → 0.988 | 12 → 1 | 0 → 0 |

No case got worse on any catalogue. The "hotd" movie case scores top-1 either
way, so its fix shows only in the false-positive column. On Compact, *Hot
Fuzz*, *Some Like It Hot* and *Robin Hood* filled the rest of its first five.

By category, on Expanded for the language-independent categories, and over
every catalogue that has them for the regional ones:

| Category | Cases | Top-1 | Recall@5 |
| --- | ---: | ---: | ---: |
| exact, original, acronym, joined, punctuation, accent, short, non-latin | 131 | 100% → 100% | 100% → 100% |
| fuzzy | 27 | 81.5% → 81.5% | 96.3% → 96.3% |
| transposition | 14 | 92.9% → 100% | 92.9% → 100% |
| similar | 18 | 94.4% → 94.4% | 100% → 100% |
| year | 10 | 0% → 100% | 0% → 100% |
| alias (regional) | 74 | 86.5% → 100% | 97.3% → 100% |
| alias-fuzzy | 25 | 88.0% → 88.0% | 88.0% → 88.0% |

Generated queries (`tools/benchmark -phase accuracy`, 3,000 sampled titles,
seed 1), top-1 / top-5:

| Size | Exact | One typo | Two typos | Prefix | Joined |
| --- | --- | --- | --- | --- | --- |
| Compact | 95.4/99.4 → 96.0/100 | 82.0/93.4 → 83.1/94.5 | 79.4/87.9 → 80.5/89.1 | 68.8/84.4 → 69.2/84.8 | unchanged |
| Standard | 93.4/99.3 → 93.9/99.8 | 76.9/89.8 → 78.3/91.6 | 71.8/80.2 → 72.6/81.0 | 52.1/68.2 → 52.6/69.0 | unchanged |
| Expanded | 91.7/98.6 → 92.5/99.4 | 73.6/86.4 → 75.9/88.9 | 68.4/75.9 → 69.7/77.1 | 43.8/56.8 → 44.1/57.5 | unchanged |

Those figures are without the regional-title change. That change moves
generated regional-title exact queries from 86.8% to 89.7% top-1 on Expanded
with French titles, and costs 0.1–0.3 points of top-1 on generated canonical
exact queries. In those few queries, a popular film's regional title is the
same as another film's canonical name. Search latency and heap after startup
did not change measurably: Standard and Expanded, 30 iterations, before and
after, with the same 31.6 and 64.9 MiB heap.

## What changed

1. **Year in the query.** "dune 1984" used to return nothing, because every
   word had to appear in a title. A four-digit year from 1880 to 2099, typed
   with other words, now matches the title's start year or a title word
   (*Blade Runner 2049*). If no candidate matches the year, it is ignored. A
   year on its own is still just a word.
2. **Long-word transpositions and doubled letters.** In a word of seven or
   more letters, a swap breaks up to four trigrams. "thornes" shares one with
   "thrones", so it lost the bounded candidate scan to thousands of unrelated
   words. Deletions and adjacent swaps of 7–20-letter words are now looked up
   directly, at about two binary searches per letter.
3. **Long translated titles.** "el señor de los anillos la comunidad del
   anillo" has nine words and was rejected outright. The eight-word bound now
   counts only words that are actually looked up.
4. **Short acronyms.** A single word of five letters or fewer that is a
   title's acronym ("hotd") no longer expands to one-letter variants such as
   "hot", "hood" and "hota". Prefix matches are kept.
5. **Exact regional titles rank by popularity** alongside exact canonical
   names, so "cadena perpetua" lists *The Shawshank Redemption* before a 1979
   film with that original title, which stays second. Short regional titles
   still rank with acronyms.

## Remaining weaknesses

- **An exact obscure title beats a near-miss of a famous one.** "interstelar"
  lists the 2016 film *Interstelar* before *Interstellar*, and "the dark
  night" does the same with *The Dark Night*. Both are listed second. Making
  popularity outweigh an exact match would need a vote-ratio rule that the
  current cases cannot justify.
- **Franchise ties.** "the terminater" and "harry poter" match every film in
  the series equally, so the most-voted one wins: *Terminator 2*, *Deathly
  Hallows: Part 2*. The intended first film is second. A small bonus for
  titles whose words are all covered by the query would probably help, but it
  needs a per-title word count in the index and wider testing.
- **Phonetic spellings.** "incepshun" is three edits from "inception" and is
  not found. Edit distance cannot cover this without a phonetic index.
- **Three-letter typos.** "retour ver le futur" misses because "ver" is too
  short for fuzzy matching. Its prefix scan stops after the first 20
  vocabulary words, which come before "vers".
- **Shared exact names.** "avatar" as a series lists the 2024 series
  *Avatar* before *Avatar: The Last Airbender*. The exact name is a fair
  answer.
- **Coverage.** Regional cases exist for five languages, and only Japanese
  tests non-Latin script. No case covers Cyrillic, Korean or other scripts
  yet: IMDb's original titles are romanised, and no snapshot was built with
  those languages.
