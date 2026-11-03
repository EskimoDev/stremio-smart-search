package textnorm

import (
	"strings"
	"testing"
	"unicode"
)

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

// referenceNormalize is the original strings.ToLower-based implementation; the
// fast path must agree with it exactly.
func referenceNormalize(text string) string {
	var out strings.Builder
	space := false
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' {
			continue
		}
		if replacement, ok := fold[r]; ok {
			if space && out.Len() > 0 {
				out.WriteByte(' ')
			}
			space = false
			out.WriteString(replacement)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && out.Len() > 0 {
				out.WriteByte(' ')
			}
			space = false
			out.WriteRune(r)
		} else {
			space = true
		}
	}
	return out.String()
}

func FuzzNormalizeMatchesReference(f *testing.F) {
	for _, seed := range []string{"La Reine des Neiges", "L'Âge de glace", "Straße", "Ре́вность", "千と千尋", "\xff\xfeA", "KELVIN K", "İstanbul", "ǅ", "  A/B & 100%  "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if got, want := Normalize(text), referenceNormalize(text); got != want {
			t.Fatalf("Normalize(%q) = %q, reference %q", text, got, want)
		}
	})
}

func TestSuperscriptDigitsAreWords(t *testing.T) {
	for in, want := range map[string]string{"Alien³": "alien 3", "Alien³ Redux": "alien 3 redux", "x²": "x 2", "E=mc²": "e mc 2"} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
