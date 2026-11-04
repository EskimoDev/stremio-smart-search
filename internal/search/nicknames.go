package search

// nicknames are the short names fans use for a few popular titles that no
// acronym rule produces: "b99" turns a spelled-out number into digits, "jjk"
// and "jjba" take two letters from one word, and "svu" abbreviates only the
// subtitle. They are keyed by normalized name, or the normalized part before
// a colon, and indexed like acronyms, so a nickname ranks in the acronym band
// where popularity decides between titles that share it.
//
// Only fan nicknames in wide use belong here, for titles that rank among
// the most popular of their kind; a rule that derives a nickname is better
// than an entry.
var nicknames = map[string][]string{
	"brooklyn nine nine":              {"b99"},
	"fullmetal alchemist":             {"fma"},
	"fullmetal alchemist brotherhood": {"fmab", "fma"},
	"jojos bizarre adventure":         {"jjba"},
	"jujutsu kaisen":                  {"jjk"},
	"law order special victims unit":  {"svu"},
}
