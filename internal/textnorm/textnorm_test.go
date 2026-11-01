package textnorm

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"La Reine des Neiges":                 "la reine des neiges",
		"Là-haut":                             "la haut",
		"L'Âge de glace":                      "lage de glace",
		"Ledové království":                   "ledove kralovstvi",
		"Kraina lodu: Łódź":                   "kraina lodu lodz",
		"Die Eiskönigin – völlig unverfroren": "die eiskonigin vollig unverfroren",
		"Straße":                              "strasse",
		"Cœur":                                "coeur",
		"Şahsiyet":                            "sahsiyet",
		"Ре́вность":                           "ревность", // decomposed accent on Cyrillic
		"千と千尋の神隠し":                            "千と千尋の神隠し",
		"Amélie":                              "amelie",
		"Amélie":                             "amelie",
		"  A/B & 100%  ":                      "a b 100",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
