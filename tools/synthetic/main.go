// Command synthetic writes IMDb-shaped, entirely invented title.basics,
// title.ratings and title.akas files for local benchmarking where the real
// datasets cannot be downloaded. It is deterministic for a given -seed.
//
// The shape is calibrated loosely against the 2026 IMDb snapshot: about 12M
// basics rows (mostly episodes), 1.6M ratings with a heavy-tailed vote count,
// Zipf-distributed title words, some non-Latin and accented titles, and akas
// rows grouped by title. A few well-known IDs used by the benchmark quality
// checks get their real English and French titles. Results from this data are
// synthetic and must be labelled as such; they do not replace real runs.
package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"github.com/EskimoDev/stremio-smart-search/tools/internal/benchdata"
)

type anchor struct {
	kind, name, fr string
	votes          int
}

var anchors = map[int]anchor{
	11198330: {"tvSeries", "House of the Dragon", "", 600000},
	816692:   {"movie", "Interstellar", "", 2300000},
	903747:   {"tvSeries", "Breaking Bad", "", 2200000},
	944947:   {"tvSeries", "Game of Thrones", "Le Trône de fer", 2400000},
	2294629:  {"movie", "Frozen", "La Reine des neiges", 700000},
	2948356:  {"movie", "Zootopia", "Zootopie", 550000},
	211915:   {"movie", "Amélie", "Le Fabuleux Destin d'Amélie Poulain", 800000},
	245429:   {"movie", "Spirited Away", "Le Voyage de Chihiro", 900000},
	120737:   {"movie", "The Lord of the Rings: The Fellowship of the Ring", "Le Seigneur des anneaux : La Communauté de l'anneau", 2000000},
	167261:   {"movie", "The Lord of the Rings: The Two Towers", "Le Seigneur des anneaux : Les Deux Tours", 1800000},
	167260:   {"movie", "The Lord of the Rings: The Return of the King", "Le Seigneur des anneaux : Le Retour du roi", 2000000},
	145487:   {"movie", "Spider-Man", "", 900000},
	241527:   {"movie", "Harry Potter and the Sorcerer's Stone", "Harry Potter à l'école des sorciers", 900000},
	15239678: {"movie", "Dune: Part Two", "Dune : Deuxième partie", 700000},
}

func main() {
	dir := flag.String("out", "", "Directory for the generated .tsv.gz files")
	rows := flag.Int("basics", 12_000_000, "title.basics rows")
	seed := flag.Uint64("seed", 1, "Random seed")
	flag.Parse()
	if *dir == "" {
		panic("choose -out")
	}
	must(os.MkdirAll(*dir, 0o755))
	r := rand.New(rand.NewPCG(*seed, 2))
	v := benchdata.NewVocab(r, 420_000)
	fr := benchdata.NewVocab(rand.New(rand.NewPCG(*seed, 3)), 120_000)
	basics, closeBasics := create(filepath.Join(*dir, "title.basics.tsv.gz"))
	ratings, closeRatings := create(filepath.Join(*dir, "title.ratings.tsv.gz"))
	akas, closeAkas := create(filepath.Join(*dir, "title.akas.tsv.gz"))
	fmt.Fprintln(basics, "tconst\ttitleType\tprimaryTitle\toriginalTitle\tisAdult\tstartYear\tendYear\truntimeMinutes\tgenres")
	fmt.Fprintln(ratings, "tconst\taverageRating\tnumVotes")
	fmt.Fprintln(akas, "titleId\tordering\ttitle\tregion\tlanguage\ttypes\tattributes\tisOriginalTitle")
	regions := strings.Fields("US GB DE ES IT JP BR MX CA IN RU PL SE TR GR PT HU AR FI NO DK NL")
	for id := 1; id <= *rows; id++ {
		tconst := fmt.Sprintf("tt%07d", id)
		a, isAnchor := anchors[id]
		kind := "tvEpisode"
		switch p := r.IntN(1000); {
		case isAnchor:
			kind = a.kind
		case p < 62:
			kind = "movie"
		case p < 75:
			kind = "tvMovie"
		case p < 95:
			kind = "tvSeries"
		case p < 99:
			kind = "tvMiniSeries"
		case p < 160:
			kind = "short"
		case p < 190:
			kind = "video"
		}
		name := v.Title(r)
		if r.IntN(40) == 0 {
			name = benchdata.NonLatin(r)
		}
		original := name
		if r.IntN(7) == 0 {
			original = fr.Title(r)
			if r.IntN(3) == 0 {
				original = benchdata.NonLatin(r)
			}
		}
		if isAnchor {
			name, original = a.name, a.name
		}
		adult := "0"
		if r.IntN(100) == 0 && !isAnchor {
			adult = "1"
		}
		fmt.Fprintf(basics, "%s\t%s\t%s\t%s\t%s\t%d\t\\N\t%d\tDrama\n", tconst, kind, name, original, adult, 1900+r.IntN(126), 20+r.IntN(160))
		// Ratings: heavy-tailed votes (P(votes >= v) ~ v^-0.31 for main titles).
		rated := false
		switch kind {
		case "movie", "tvMovie":
			rated = r.IntN(100) < 62
		case "tvSeries", "tvMiniSeries":
			rated = r.IntN(100) < 85
		case "tvEpisode":
			rated = r.IntN(100) < 9
		default:
			rated = r.IntN(100) < 20
		}
		votes := 0
		if rated || isAnchor {
			votes = int(math.Min(3e6, 1/math.Pow(1-r.Float64(), 3.2)))
			if kind == "tvEpisode" {
				votes = 1 + votes%200
			}
			if isAnchor {
				votes = a.votes
			}
			fmt.Fprintf(ratings, "%s\t%.1f\t%d\n", tconst, 1+r.Float64()*9, votes)
		}
		// Akas: popular titles have many regional rows, most with language \N.
		if kind == "tvEpisode" && r.IntN(3) != 0 {
			continue
		}
		n := 1 + int(math.Min(40, math.Log2(float64(votes+1))*1.5))
		ordering := 1
		emit := func(title, region, language, types string) {
			fmt.Fprintf(akas, "%s\t%d\t%s\t%s\t%s\t%s\t\\N\t0\n", tconst, ordering, title, region, language, types)
			ordering++
		}
		fmt.Fprintf(akas, "%s\t%d\t%s\t\\N\t\\N\toriginal\t\\N\t1\n", tconst, ordering, original)
		ordering++
		if (isAnchor && a.fr != "") || (votes >= 5 && r.IntN(100) < 30+int(math.Min(60, float64(votes)/50))) {
			french := fr.Title(r)
			if isAnchor && a.fr != "" {
				french = a.fr
			} else if r.IntN(3) == 0 {
				french = name // many French releases keep the title
			}
			emit(french, "FR", `\N`, "imdbDisplay")
			if r.IntN(4) == 0 {
				emit(fr.Title(r), "FR", `\N`, "alternative")
			}
		}
		for i := 0; i < n; i++ {
			title := name
			if r.IntN(2) == 0 {
				title = fr.Title(r)
			}
			emit(title, regions[r.IntN(len(regions))], `\N`, `\N`)
		}
	}
	closeBasics()
	closeRatings()
	closeAkas()
}

func create(path string) (*bufio.Writer, func()) {
	f, err := os.Create(path)
	must(err)
	z, _ := gzip.NewWriterLevel(f, gzip.BestSpeed)
	w := bufio.NewWriterSize(z, 1<<20)
	return w, func() { must(w.Flush()); must(z.Close()); must(f.Close()) }
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
