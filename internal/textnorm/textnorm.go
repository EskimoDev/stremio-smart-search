// Package textnorm holds the single title normalization shared by catalogue
// building (alias deduplication) and the search index, so both agree exactly.
package textnorm

import (
	"strings"
	"unicode"
)

// fold maps common precomposed Latin letters to ASCII. Decomposed input loses
// its combining marks (unicode.Mn) below, so either form normalizes the same way.
var fold = func() map[rune]string {
	m := map[rune]string{}
	for target, letters := range map[string]string{
		"a": "àáâãäåāăąǎ", "e": "èéêëēĕėęě", "i": "ìíîïīĭįıǐ", "o": "òóôõöøōŏőǒ", "u": "ùúûüūŭůűųǔ",
		"c": "çćĉċč", "n": "ñńņňŉ", "y": "ýÿŷ", "s": "śŝşšș", "z": "źżž", "l": "ĺļľŀł",
		"d": "ďđð", "g": "ĝğġģ", "r": "ŕŗř", "t": "ţťŧț", "h": "ĥħ", "j": "ĵ", "k": "ķ", "w": "ŵ",
		"ss": "ß", "ae": "æ", "oe": "œ", "th": "þ",
	} {
		for _, r := range letters {
			m[r] = target
		}
	}
	return m
}()

// Normalize lowercases, removes accents and apostrophes, and collapses every
// other non-letter, non-digit run into one space. Non-Latin letters are kept.
func Normalize(text string) string {
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

// Meaningful reports whether a normalized alternative title is distinctive
// enough to index. Very short Latin aliases ("Up", "Lo") match too much; short
// CJK/Hangul titles such as "기생충" are legitimate and kept.
func Meaningful(normal string) bool {
	letters, nonLatin := 0, false
	for _, r := range normal {
		if r == ' ' {
			continue
		}
		letters++
		if r > 0x24F {
			nonLatin = true
		}
	}
	return letters >= 3 || (nonLatin && letters >= 2)
}
