# Search quality

How accurately Smart Search finds the title someone meant, measured on real
catalogues with known IMDb answers.

## Preview.4 search quality

The sound-alike matcher is included in preview.4 but is not in preview.3. The suite then contained **288 cases in 16 categories** (346 in 18 since the [several-misspelled-words change](#several-misspelled-words-2026-11-02)). With alternative
titles off, the current engine produced the following results on the existing
local real catalogues during documentation verification:

| Catalogue | Titles | Expected answer first | Expected answer in the top five |
| --- | ---: | ---: | ---: |
| Compact | 50,000 | 224/228 (98.2%) | 225/228 (98.7%) |
| Standard | 184,588 | 222/229 (96.9%) | 226/229 (98.7%) |
| Expanded | 390,449 | 222/231 (96.1%) | 228/231 (98.7%) |

Each row also ran three no-answer checks with no false positives. The 54
language-specific cases did not apply; three additional expected titles were
outside Compact's coverage and two outside Standard's. Those are excluded from
the ranking denominator. The current suite adds difficult phonetic cases, so
these aggregate percentages cannot be compared directly with the smaller
preview.3 suite below. These snapshots also slightly predate the larger title
counts in the recorded Windows performance baseline.

The [near-code and confidence results](#sound-alike-follow-up-near-codes-and-confidence-2026-11-01)
cover the latest phonetic-specific checks, including a separate blind validation
set with known false positives. Tests do not establish accuracy on every search
or language.

## Released preview.3 baseline

On the original **248-case suite**, after the coverage-ranking follow-up and
before phonetic matching, the recorded results with alternative titles **off**
were:

| Catalogue | Expected answer first | Expected answer in the top five |
| --- | ---: | ---: |
| Compact | 191/192 (99.5%) | 191/192 (99.5%) |
| Standard | 190/192 (99.0%) | 191/192 (99.5%) |
| Expanded | 188/192 (97.9%) | 191/192 (99.5%) |

These percentages describe a curated test set, not all searches someone might
make. Each catalogue runs 194 applicable cases: 192 with an expected answer and
two no-answer checks. The other 54 cases require a particular alternative-title
language; they are not included in these percentages. The release-era suite had
248 cases; the current expanded suite is described above. Titles outside a
catalogue's coverage and unavailable regional names are reported separately
rather than counted as ranking failures.

The earlier-ranking tables below preserve historical before/after comparisons.
The [coverage-ranking follow-up](#follow-up-prefer-titles-the-query-covers) produced
this release baseline; the phonetic sections describe later preview.4 work.
See [remaining weaknesses](#remaining-weaknesses) for examples that still fail.

## The cases

[`testdata/quality/cases.json`](../testdata/quality/cases.json) holds 346
curated queries in 18 categories:

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
| phonetic | a title spelled the way it sounds | `pulp fikshun`, `nite at the museum` |
| dropped-vowel | words typed without their vowels | `strngr thngs`, `gdfthr` |
| multi-typo | several misspelled words, or a space in the wrong place | `gma e of thr`, `bak to teh futre` |
| negative | queries with no right answer | `qzxqvvnonexistent` |

A separate file, [`phonetic-holdout.json`](../testdata/quality/phonetic-holdout.json),
holds 31 more phonetic queries that were written before sound-alike matching
was designed and were never used to tune it. Score it with
`-cases testdata/quality/phonetic-holdout.json`. A third file,
[`phonetic-validation.json`](../testdata/quality/phonetic-validation.json),
holds 71 cases (45 sound-alikes, 11 exact-title guards, 15 negatives) written
before the second round of tuning and scored only after each variant was
fixed; its per-case results were not inspected while tuning.

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

Top-1, Recall@5 and MRR use only scored cases with an expected answer as their
denominator. No-answer cases contribute to false-positive checks, not ranking
percentages. The tables' **Cases** column includes both kinds.

## Reproduce

```sh
go run ./tools/quality -v -out ../sss-quality/after -baseline ../sss-quality/before \
    path/to/compact.json.gz path/to/standard-fr.json.gz ...
```

Snapshots come from `tools/benchmark -phase prepare` (see
[PERFORMANCE.md](PERFORMANCE.md#reproduce)) and must stay outside the
repository. The harness is deterministic: the same build and snapshot always
give the same report. `go test ./internal/quality` checks the case file and
the metric code without real data.

## Earlier ranking fixes, 2026-11-01

These are real IMDb snapshots prepared on 2026-11-01: Compact, Standard and
Expanded, French versions of all three, and Standard with English, Spanish
(all regions and Spain only), German and Japanese regional titles. "Before"
is the compact-index build ([#7](https://github.com/EskimoDev/stremio-smart-search/pull/7)),
which preserved the pre-rewrite search results. "After" includes the first five
fixes below, before the later coverage-ranking follow-up. These are historical
comparisons, not the final preview.3 totals.

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

## Follow-up: prefer titles the query covers

When several titles match a query's words equally well, the one with the
fewest extra words now gets up to 10 extra points on a 0–100 word-match
scale. So "the terminater" lists *The Terminator* before *Terminator 2:
Judgment Day*, and "harry poter" lists *Sorcerer's Stone* first. Exact,
joined and acronym bands are unaffected. The per-title word count fits in
existing struct padding: the entry stays 48 bytes, and heap after startup
grew by 0.1 MiB on Expanded.

Curated top-1 went from 98.4% to 99.5% on Compact, 97.9% to 99.0% on
Standard and 96.9% to 97.9% on Expanded, with no case worse on any of the 11
snapshots. Generated queries, top-1 / top-5:

| Size | One typo | Two typos | Prefix | Exact, joined |
| --- | --- | --- | --- | --- |
| Compact | 83.1/94.5 → 85.7/95.3 | 80.5/89.1 → 82.0/89.3 | 69.2/84.8 → 74.4/88.2 | unchanged |
| Standard | 78.3/91.6 → 81.5/92.8 | 72.6/81.0 → 74.5/81.9 | 52.6/69.0 → 57.0/73.5 | unchanged |
| Expanded | 75.9/88.9 → 77.9/90.2 | 69.7/77.1 → 71.0/77.6 | 44.1/57.5 → 47.0/60.7 | unchanged |

A second idea was tried and rejected: letting a famous near-miss outrank an
obscure exact title, so that "interstelar" lists *Interstellar* first. It
fixed four curated cases, but generated exact-title top-5 on Expanded fell
from 99.4% to 97.7%. People who type an obscure title exactly would lose it.

## Remaining weaknesses

- **An exact obscure title beats a near-miss of a famous one.** "interstelar"
  lists the 2016 film *Interstelar* before *Interstellar*, and "the dark
  night" does the same with *The Dark Night*. Both are listed second. Making
  popularity outweigh an exact match would need a vote-ratio rule that the
  current cases cannot justify.
- **Phonetic spellings, partly (preview.4).** "rattatooey", "fone
  booth", "the dark nite" and "nite rider" still miss. The near-code follow-up
  finds "back to the fucher", "the wocking dead" and "brekin bad"; "pinokkio"
  reaches rank four on Expanded. Three-letter words such as "hoo" and long
  consonant runs such as "sicksth" are excluded. See the
  [latest follow-up](#sound-alike-follow-up-near-codes-and-confidence-2026-11-01).
- **Three-letter typos, partly.** Since the
  [several-misspelled-words change](#several-misspelled-words-2026-11-02), a
  rare three-letter word gets a swapped pair or one missing letter ("gyu",
  "bak"), but only when the query as typed matches poorly. Other three-letter
  typos still rely on the rest of the query.
- **Shared exact names.** "avatar" as a series lists the 2024 series
  *Avatar* before *Avatar: The Last Airbender*. The exact name is a fair
  answer.
- **Coverage.** Regional cases exist for five languages, and only Japanese
  tests non-Latin script. No case covers Cyrillic, Korean or other scripts
  yet: IMDb's original titles are romanised, and no snapshot was built with
  those languages.

## Compared with Stremio's default search, 2026-11-02

A spot check of the development version (main after the
[several-misspelled-words change](#several-misspelled-words-2026-11-02))
against Stremio's built-in search (Cinemeta), on hard queries: dropped vowels,
several typos, acronyms, sound-alikes and number words. Smart Search used the
real 2026-11-01 Standard catalogue; Cinemeta's answers came from its public
search API on the same day and may differ between requests or apps.

| Result on 38 queries | Before the 2026-11-02 search fixes | Development version |
| --- | ---: | ---: |
| Only Smart Search lists it first | 2 | 12 |
| Both list it first | 15 | 24 |
| Only Stremio lists it first | 10 | 1 |
| Both miss | 11 | 1 |

Examples where only Smart Search finds the title first:

| You type | Smart Search | Stremio's default search |
| --- | --- | --- |
| `intrstllr` | *Interstellar* | nothing |
| `plp fctn` | *Pulp Fiction* | nothing |
| `gdfthr` | *The Godfather* | *Lust, Caution*, *Hollow Man* |
| `brkng bd` | *Breaking Bad* | three unrelated series |
| `bttr cll sl` | *Better Call Saul* | three unrelated series |
| `teh dakr knight` | *The Dark Knight* | *A Knight's Tale*, *First Knight* |
| `hous of teh dragn` | *House of the Dragon* | *Dragnet* |
| `bak to teh futre` | *Back to the Future* | *Ong-Bak* |
| `hotd` | *House of the Dragon* | *Highschool of the Dead* |

Both now find `fther thd`, `star war`, `strngr thngs`, `twelve angry men`,
`gma e of thr` and `rik and mortey` first; before these changes only Stremio
did. Stremio still wins `toy story three` (*Toy Story 3* is second in Smart
Search), and both miss `too fast too furious`.

Many of these queries helped build the dropped-vowel and
several-misspelled-words changes, so this is a demonstration, not a blind
test; the held-back results are in those sections. On the wider set of 243
hard queries this run comes from, the expected title came first for 205 on
Standard, against 174 for the earlier engine, with no query worse. Standard,
Expanded and six alternative-title catalogues all improved by 30–32 queries.

Still weak in Smart Search, from the same run:

- **Number words.** "twenty one jump street", "eight mile" and "too fast too
  furious" find nothing, and "toy story three" lists *Toy Story* first:
  "three" and "3" are not linked.
- **Sequels** list the first film before the numbered one: "rocky 4",
  "godfather 2" and "terminator 2" give the sequel at rank two or three.
- **An exact obscure title beats a near-miss of a famous one**: "avenger",
  "transformer", "gremlin", "euforia" and "kasablanka" rank the famous title
  second or third.
- **Mister and doctor** are not linked to Mr. and Dr. ("mister robot",
  "doctor no"), and some acronyms ("b99", "jjk", "fmab") miss unless English
  alternative titles are on.
- **Joined words with a typo** ("brkingbad") and **split sound-alikes**
  ("gost busters", "wun peace") still find nothing.

## Several misspelled words, 2026-11-02

A title has to match every looked-up query word, so one word that matches
nothing empties the result. With several typos that is common: a three-letter
word had no typo matching at all ("bda", "rik", "teh"), and a letter pushed
across a space left a fragment too short to look up ("gma e of thr" for "game
of thr"). Main returned nothing for 11 of 55 such tuning queries.

The query as typed is still scored first. Only when no title matches every
word at least as well as a one-letter typo are other readings tried, and each
title keeps its best score:

- **One word left out**, scoring nothing for it and 10 points less. In a
  two-word query only a word of up to four letters may be left out, and the
  words that do match must average a two-letter typo or better, so gibberish
  such as "zarnet bluve" still finds nothing.
- **A fragment joined to its neighbour**: "gma e" is read as "gmae", one swap
  from "game". A fragment is one letter, or two letters few titles use, so
  "de" and "la" are not joined to everything.
- **Spaces moved or added**: two words that form an indexed word when joined
  ("shaw shank"), two words split at another point into indexed words ("gam
  eof" as "game of"), and a word that is two indexed words run together
  ("maxfury"). Words found this way weigh .8.
- **Three-letter typos**: a rare three-letter word is tried with a swapped pair
  (.72, "gyu" for "guy") or one missing letter (.68, "bak" for "back"). Stop
  words are not targets.

Results on real 2026-11-01 snapshots, Standard and Expanded:

| Set | Main | This change |
| --- | ---: | ---: |
| Tuning set, 55 queries | 44 | 55 |
| Held-back set 1, 30 queries | 28 | 29 |
| Held-back set 2, 32 queries | 21 | 32 |

Set 1 was written before any change and never tuned on; set 2 was written
after the first version, and two of its misses ("family gyu", "bak to teh
futre") led to the three-letter typo probes, so it is not fully blind. The
remaining set-1 miss, "the witch" for *The Witcher*, ranks third behind two
films titled *The Witch*, which is a fair answer.

33 of these queries are now the `multi-typo` category in `cases.json`, and the
25 dropped-vowel showcase queries are the `dropped-vowel` category. Top-1 on
the new categories, main → this change, on Compact, Standard and Expanded:
multi-typo 30.3% → 100%, dropped-vowel 96.0% → 100%. Across the existing
suites (`cases.json`, the phonetic held-out and validation files) on all 11
snapshots no case got worse; 67 case results improved, among them "doctor
hoo", "the sicksth sense", "lay mizerables" and "fone booth".

Speed: the first version tried the other readings for every query and made
typical searches about 28% slower in `sss-bench`. Trying them only when the
query as typed matches poorly brought the real Standard A/B back to no
significant change on all 12 primary metrics (exit 0). Uncorrected hints
remain: engine-only throughput 7–10% lower and 6% more bytes per search, from
the queries that do take the slower path.

## Sound-alike fallback, 2026-11-01

**Preview.4 development, first-round measurements.** This section preserves the original
implementation and comparisons. The near-code and confidence follow-up below
supersedes its scores and list of missed titles.

Some searches are spelled the way the title sounds: `incepshun`, `pulp
fikshun`, `nite at the museum`. They are three or more edits from the title,
so edit distance cannot reach them without also matching everything else.
Smart Search now files every indexed word by its Double Metaphone code and
uses that as a weak, last-resort match.

### How it works

- **Index.** At build time ASCII vocabulary words of 3–64 letters get their
  Double Metaphone primary and alternate codes, capped at 12 sounds rather
  than the usual four. Codes shorter than two sounds are omitted. Codes are packed four bits per
  sound into a `uint64`, stored once each, sorted, with a posting list of the
  words that share them. Words with digits, accents that did not fold to
  ASCII, or non-Latin script get no code.
- **When it runs.** Only for a query word of four or more letters, when typo
  matching is on (not for a short acronym such as `hotd`), when the word has a
  vowel and no run of more than three consonants (`xqzvwkplm` is skipped), and
  when the word is not already in 50 or more titles. So `bear` is taken as
  meant, while a rare indexed spelling such as `nite` still gets sound-alikes.
- **What it adds.** From the words sharing either code, those within three
  letters of the query's length and within about half its letters by edit
  distance; at most 8 per query word, most-used words first. They are added
  after the usual 32 spelling terms, so typo matches cannot crowd them out.
- **How they rank.** A sound-alike word scores 0.4 of a full word match,
  below one edit (0.76) and two edits (0.6). Exact, joined, acronym and
  regional-title bands are untouched, so an exact title keeps first place:
  `momento` still lists the 2013 film *Momento* before *Memento*. Popularity
  lets a famous sound-alike beat an obscure spelling match: `fite club` finds
  *Fight Club* rather than *Madam Yankelova's Fine Literature Club*.
  Results are deduplicated by IMDb ID as before.

**Languages.** The rules are English, with the Romance, Germanic and Slavic
spellings that Double Metaphone already handles. They help most for English
titles and English-like respellings. A non-English Latin word can still get
English sound-alikes; that only adds low-ranked candidates, and no regional
or original-title case changed on any of the eleven catalogues. Non-Latin
titles and queries are never matched phonetically.

### Alternatives compared

All on the real Expanded catalogue, with the same inputs. "Phonetic" is the
39 curated phonetic cases, "Held-out" the 31 untouched ones, "One typo" the
generated one-typo top-1 (3,000 sampled titles, seed 1). "Random" is the share
of 3,000 random 5–10-letter strings that return any result, a rough measure
of over-matching.

| Approach | Phonetic top-1 / top-5 | Held-out top-1 / top-5 | One typo top-1 | Random |
| --- | --- | --- | --- | --- |
| None (main) | 28.2% / 30.8% | 43.3% / 46.7% | 77.9% | 3.6% |
| **Double Metaphone, precomputed, weight 0.4 (chosen)** | **76.9% / 82.1%** | **63.3% / 73.3%** | **77.8%** | **13.8%** |
| Same, primary code only | 74.4% / 79.5% | 63.3% / 73.3% | 77.9% | 12.9% |
| Same, stricter letter limit (a third of the word) | 71.8% / 76.9% | 63.3% / 66.7% | 77.9% | 8.4% |
| Same, only for words not in the index | 61.5% / 64.1% | 60.0% / 66.7% | 77.8% | 13.8% |
| Same, weight 0.7 | 76.9% / 82.1% | 70.0% / 76.7% | 74.8% | 13.8% |
| Soundex, four characters | 61.5% / 69.2% | 60.0% / 63.3% | 77.8% | 22.3% |
| Soundex, untruncated | 56.4% / 64.1% | 60.0% / 63.3% | 77.9% | 18.8% |
| Double Metaphone at query time, on the trigram candidates only (no index) | 43.6% / 46.2% | 53.3% / 56.7% | 77.9% | 5.6% |

Weight 0.7 finds more held-out titles but loses 3.1 points of one-typo
accuracy: a random typo in an obscure title often sounds like a famous
one. Query-time coding needs no memory but only sees words that already
share trigrams with the query, which most respellings do not. Soundex
merges too much. The chosen configuration was the best on the curated set
without costing typo accuracy, and the held-out set, scored only after it
was picked, confirms most of the gain.

### Results

Curated cases, before → after, on all eleven real snapshots. No case got
worse on any catalogue in either file, and no false positive was added.

| Catalogue | All cases top-1 | All cases top-5 | Phonetic top-1 | Phonetic top-5 | Held-out top-1 | Held-out top-5 |
| --- | --- | --- | --- | --- | --- | --- |
| Compact | 89.0% → 96.9% | 89.0% → 97.4% | 33.3% → 80.6% | 33.3% → 83.3% | 43.3% → 70.0% | 46.7% → 76.7% |
| Standard | 87.3% → 95.6% | 88.2% → 96.9% | 27.0% → 75.7% | 29.7% → 81.1% | 43.3% → 63.3% | 46.7% → 73.3% |
| Expanded | 86.1% → 94.8% | 87.9% → 97.0% | 28.2% → 76.9% | 30.8% → 82.1% | 43.3% → 63.3% | 46.7% → 73.3% |
| Compact, French | 90.4% → 96.8% | 90.4% → 97.2% | 38.9% → 80.6% | 38.9% → 83.3% | 43.3% → 66.7% | 46.7% → 76.7% |
| Standard, French | 88.8% → 95.6% | 89.6% → 96.8% | 32.4% → 75.7% | 35.1% → 81.1% | 43.3% → 66.7% | 46.7% → 73.3% |
| Expanded, French | 87.7% → 94.9% | 89.3% → 96.8% | 33.3% → 76.9% | 35.9% → 82.1% | 43.3% → 66.7% | 46.7% → 73.3% |
| Standard, English | 87.6% → 95.7% | 88.4% → 97.0% | 27.0% → 75.7% | 29.7% → 81.1% | 43.3% → 63.3% | 46.7% → 73.3% |
| Standard, Spanish | 87.4% → 95.8% | 88.2% → 97.5% | 24.3% → 75.7% | 27.0% → 83.8% | 43.3% → 63.3% | 50.0% → 76.7% |
| Standard, Spanish (Spain) | 87.8% → 95.8% | 88.7% → 97.5% | 27.0% → 75.7% | 29.7% → 83.8% | 40.0% → 60.0% | 46.7% → 73.3% |
| Standard, German | 87.5% → 95.8% | 88.3% → 97.1% | 24.3% → 75.7% | 27.0% → 81.1% | 43.3% → 63.3% | 46.7% → 70.0% |
| Standard, Japanese | 88.2% → 95.8% | 89.5% → 97.5% | 29.7% → 75.7% | 35.1% → 83.8% | 43.3% → 63.3% | 46.7% → 73.3% |

"All cases" now includes the phonetic category, so its "before" is lower
than in the tables above. On the original 248 cases alone, the only change
is `incepshun` (a fuzzy case): Expanded top-1 goes from 97.9% to 98.4% and
top-5 from 99.5% to 100%.

Found first on Expanded now: `incepshun`, `nite at the museum`, `fite club`,
`pulp fikshun`, `gaim of thrones`, `saving privet ryan`, `the lion keeng`,
`sykho`, `the shyning`, `yuforia`, `arcain`, `fraysure`, `the fantom
menace`, `the matricks`, `the termanater`, `hows of the dragon`, `amaylee`,
`skool of rock`, `a kwiet place`; `toy storee 3` is second. Held out: `the
green mial`, `the trooman show`, `the offis`, `the eksorsist`, `nite of the
living dead`, `filadelfia`, with `the kraun` fifth and `nite and day` fourth.

Still missed: `the dark nite`, `the wocking dead`, `back to the fucher`,
`rattatooey`, `brekin bad`, `pinokkio`, `fone booth` and `the prestij`
(second), and held out `the sicksth sense` (five consonants in a row),
`fyuchurama`, `nite rider`, `doctor hoo` (three letters), `goast` (*GOAT*
is one edit away), `the born identity`, `mishun impossible` (an exact
*Mishan Impossible* exists), `fineas and ferb`, and `aladin` (second, after
an exact *Aladin*).

Generated queries, top-1 / top-5:

| Catalogue | Exact | One typo | Two typos | Prefix | Regional title, one typo |
| --- | --- | --- | --- | --- | --- |
| Compact | unchanged | unchanged | 82.0/89.3 → 82.9/90.5 | unchanged | – |
| Standard | unchanged | 81.5/92.8 → 81.4/92.8 | 74.5/81.9 → 76.7/84.5 | 57.0/73.5 → 57.0/73.4 | – |
| Expanded | unchanged | 77.9/90.2 → 77.8/90.2 | 71.0/77.6 → 71.8/78.9 | 47.0/60.7 → 47.1/60.9 | – |
| Expanded, French | unchanged | 77.4/89.8 → 77.2/89.8 | 70.5/77.3 → 71.2/78.5 | 45.8/59.6 → 45.8/59.7 | 71.7/86.9 → 71.2/86.8 |
| Standard, German | unchanged | 81.3/92.7 → 81.2/92.7 | 73.8/81.2 → 75.9/83.6 | 55.0/71.0 → 55.1/71.0 | 65.7/72.8 → 65.5/72.8 |

The first result changed for under 1% of generated queries (89 of 10,879 on
Expanded). About 12% of them gained or changed a low-ranked result, mostly
where fewer than 20 titles matched before. Joined and exact regional-title
queries are unchanged.

### Costs

Real snapshots on the cloud Linux test machine (4 vCPU, `GOMAXPROCS=2`),
medians of three interleaved runs. Search latency is measured directly on
the engine over 2,319 queries (every curated and held-out case, 1,500
generated one-typo queries and 500 random strings), five passes each.

| Catalogue | Heap after load | Index build | Search p50 | Search p95 | Search p99 |
| --- | --- | --- | --- | --- | --- |
| Compact | 9.2 → 9.5 MiB | 172 → 205 ms | 0.113 → 0.124 ms | 0.338 → 0.365 ms | 0.528 → 0.570 ms |
| Standard | 31.6 → 32.6 MiB | 735 → 794 ms | 0.212 → 0.233 ms | 0.647 → 0.700 ms | 0.996 → 1.083 ms |
| Expanded | 65.0 → 66.6 MiB | 1,730 → 1,814 ms | 0.314 → 0.352 ms | 1.010 → 1.090 ms | 1.641 → 1.749 ms |

The fixed HTTP workload in [PERFORMANCE.md](PERFORMANCE.md) showed no change
beyond run-to-run noise, and RSS stayed within noise. The Windows figures
there have not been re-run.

### Limits

- (Before the follow-up below.) Random pronounceable strings now return
  something more often: on Expanded, 36% of random strings that pass the vowel and consonant check
  get a result, against 9% before. A query with no right answer is more
  likely to show a loose sound-alike. The three curated negative cases still
  return nothing.
- The letter limit and the 50-title rule are judgement calls measured on
  these cases, not universal constants. The stricter letter limit above
  halves the random-string rate at the cost of two held-out titles.
- Words of three letters (`hoo`) and words with long consonant runs
  (`sicksth`) get no sound-alikes.

## Sound-alike follow-up: near codes and confidence, 2026-11-01

Of the 23 curated and held-out sound-alike misses on Expanded, 17 never
reached the candidate list and 6 were found but ranked below another title.
Most of the never-found ones had a code one sound away from the title's:
`brekin` PRKN against *breaking* PRKNK, `wocking` AKNK against *walking*
ALKNK, `fucher` FXR against *future* FTR. Separately, most of what random
strings matched were obscure one-word titles that shared a sound and little
else. Two changes address those.

- **Near codes.** For a code of three or more sounds, the words filed under
  each variant with one K or L inserted, or with X swapped for T, S or K (and
  back), are added at weight 0.3, below an exact sound-alike. At most 8 per
  query word, and they must be within half the query's letters by edit
  distance, one fewer than an exact sound-alike. No index change: each
  variant is one more lookup in the same sorted code list.
- **Confidence.** A sound-alike more than a third of the query's letters away
  by edit distance must belong to a title with at least 20,000 votes, since
  people respell titles they have heard of. A query of one word (after
  stopwords and years) of five letters or more gets one edit less, because no
  other word confirms it.

A third experiment, widening the candidate list for words that have exact
company in the query, gained three held-out titles but raised multi-word
random matches and doubled the time of queries with no result; it was not
kept.

Real snapshots, PR #20's fallback → this follow-up. "Random" is the share of
2,240 speakable random 5–10-letter strings (of 6,000, seed 42) that return
anything. "Validation" counts the 56 positive cases of the new set; "fp"
the negatives that returned something.

| Catalogue | Phonetic top-1 / top-5 | Held-out top-1 / top-5 | Validation top-1 (fp) | Two typos top-1 | Random |
| --- | --- | --- | --- | --- | --- |
| Compact | 80.6 / 83.3 → 88.9 / 91.7% | 70.0 / 76.7 → 76.7 / 80.0% | 75.0% (3) → 80.4% (1) | 82.9 → 83.1% | 23.1 → 6.9% |
| Standard | 75.7 / 81.1 → 83.8 / 91.9% | 63.3 / 73.3 → 73.3 / 80.0% | 73.2% (5) → 78.6% (3) | 76.6 → 77.3% | 31.8 → 10.2% |
| Expanded | 76.9 / 82.1 → 84.6 / 92.3% | 63.3 / 73.3 → 73.3 / 80.0% | 71.4% (5) → 76.8% (3) | 71.8 → 73.5% | 36.8 → 12.7% |

On main, before any sound-alike matching, Expanded scored 41.1% on the
validation set and 8.1% on random strings. Exact, one-typo, prefix and joined
generated accuracy did not change on any catalogue. No curated case got worse
on any of the eleven snapshots. One held-out case did: `the kraun` no longer
finds *The Crown* (three edits, and alone after the stopword), in exchange
for `fyuchurama`, `mishun impossible` and `fineas and ferb`. `pinokkio` now
reaches *Pinocchio* at rank 4.

Heap and index build are unchanged, since nothing new is stored. Search
latency, medians of three interleaved runs at `GOMAXPROCS=2`: Expanded p50
0.212 → 0.224 ms, p95 0.943 → 0.989 ms, p99 1.54 → 1.61 ms; Standard p95
0.610 → 0.698 ms. Still left: `rattatooey` (RTT against RTTL, a dropped
final L), `goast` and `the prestij` (found but ranked low), `doctor hoo`
(three letters) and `the sicksth sense` (consonant run).

## Dropped vowels, 2026-11-02

Queries typed without most vowels, such as `strngr thngs`, `gdfthr` or
`blck mrrr`, are three or more edits from the words they mean, past the typo
limit (one edit up to six letters, two from seven), and they do not sound like
anything, so neither typo nor sound-alike matching found them. Of 25 such
queries in a hard real-data set, 2 ranked the right title first.

- **How it works.** The vocabulary is also kept in consonant-skeleton order:
  a word without a, e, i, o, u (and y after the first letter), writing a
  consonant that repeats the one before it once (`stranger` is `strngr`,
  `succession` and `sccssn` are both `scsn`, `mirror` and `mrrr` are both
  `mr`). Only word numbers are stored; skeletons are worked out while
  comparing, as joined keys are. A query word with at most one vowel looks up
  words with the same skeleton, or, from four consonants, one more consonant
  at the end (`thron` for `thrones`). Every typed letter must appear in the
  word in order, so `frnds` never means *fiends*. Up to 8 words per query word
  are kept, those of the most popular titles first, at weight 0.7 (0.64 for
  the longer skeleton): between one typo (0.76) and two (0.6). Words already
  matched by spelling keep their weight. A query word that is itself used by
  50 or more titles (`star`, `dark`, `born`) is taken as typed and skips the
  lookup, which keeps ordinary queries as fast as before.
- **Results.** On the 2026-11-01 Standard and Expanded snapshots, 24 of the
  25 missing-vowel queries now rank the right title first, against 2 before,
  and two combined cases (`lrd of th rngs retrn kng`, `gdfthr 1972`) are
  fixed too. Across the whole 261-query set, top-1 went from 176 to 200 on
  Standard and from 173 to 197 on Expanded, with no query ranked worse. The
  curated cases, held-out and validation sets had no regression on any of the
  eleven snapshots. `fther thd` still misses: `thd` has an extra h rather than a
  missing vowel.
- **Costs.** Heap after indexing grows by 0.39 MiB on Standard (28.52 →
  28.91 MiB), 0.69 MiB on Expanded and 0.74 MiB on Expanded French, about
  1.2%. Index build changes within run-to-run noise (best of five, Expanded:
  2.52–2.57 → 2.56–2.81 s on the cloud VM). A first version that also looked
  up common words cost 6% on warm median engine time on Standard on a Ryzen
  5600X (0.191 → 0.203 ms, the one regression `sss-bench ab` flagged), with
  the international class up 8–12%. With common words skipped, `sss-bench ab
  --profile standard` on the real Standard snapshot shows warm engine p50
  0.401 → 0.403 ms, p95 0.968 → 0.990 ms and international p95 0.685 → 0.684
  ms on the cloud VM, with no primary metric changed (exit 0). Random strings
  of four to seven letters with at most one vowel, the only inputs the new
  lookup sees, return something 20.8% of the time on Standard instead of
  19.6%, and 24.9% instead of 23.5% on Expanded French; two such words
  together, 0.5% instead of 0.3%.
