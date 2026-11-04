// Package textnorm holds the single title normalization shared by catalogue
// building (alias deduplication) and the search index, so both agree exactly.
package textnorm

import (
	"unicode"
	"unicode/utf8"
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

// Normalize lowercases, removes accents and apostrophes, folds full-width
// ASCII and decimal digits, and collapses separators into one space.
// Superscript digits and Roman numeral characters become separate words.
// Non-Latin letters are kept.
// ASCII, by far the most common input, takes a fast path with no table lookups.
func Normalize(text string) string {
	out := make([]byte, 0, len(text))
	space := false
	word := func() {
		if space && len(out) > 0 {
			out = append(out, ' ')
		}
		space = false
	}
	for _, r := range text {
		// Full-width Latin letters, digits and punctuation use the same rules
		// as their ASCII forms, including apostrophes and case folding.
		if r >= '！' && r <= '～' {
			r -= 0xFEE0
		}
		if r < utf8.RuneSelf {
			switch {
			case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
				word()
				out = append(out, byte(r))
			case r >= 'A' && r <= 'Z':
				word()
				out = append(out, byte(r+'a'-'A'))
			case r == '\'':
			default:
				space = true
			}
			continue
		}
		r = unicode.ToLower(r)
		if unicode.Is(unicode.Mn, r) || r == '\'' || r == '’' {
			continue
		}
		// A superscript digit is a word of its own: Alien³ is "alien 3".
		if digit := superscriptDigit(r); digit != 0 {
			space = true
			word()
			out = append(out, digit)
			space = true
			continue
		}
		if numeral := romanNumeral(r); numeral != "" {
			space = true
			word()
			out = append(out, numeral...)
			space = true
			continue
		}
		if replacement, ok := fold[r]; ok {
			word()
			out = append(out, replacement...)
			continue
		}
		if unicode.IsLetter(r) {
			word()
			out = utf8.AppendRune(out, r)
		} else if unicode.IsDigit(r) {
			word()
			out = append(out, decimalDigit(r))
		} else {
			space = true
		}
	}
	return string(out)
}

// Decimal-digit ranges consist of runs of 0–9 in order. Some ranges group
// several such alphabets, including mathematical digits, hence the modulo.
// Called only after unicode.IsDigit, so a matching range is guaranteed.
func decimalDigit(r rune) byte {
	if r <= 0xFFFF {
		for _, block := range unicode.Digit.R16 {
			if c := uint16(r); c >= block.Lo && c <= block.Hi && (c-block.Lo)%block.Stride == 0 {
				return '0' + byte((c-block.Lo)/block.Stride%10)
			}
		}
	} else {
		for _, block := range unicode.Digit.R32 {
			if c := uint32(r); c >= block.Lo && c <= block.Hi && (c-block.Lo)%block.Stride == 0 {
				return '0' + byte((c-block.Lo)/block.Stride%10)
			}
		}
	}
	return 0
}

var romanLetters = [...]string{"i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x", "xi", "xii", "l", "c", "d", "m"}

// r has already been lowercased; compatibility numerals are separate tokens
// so a sequel suffix such as RockyⅡ is treated as "rocky ii".
func romanNumeral(r rune) string {
	if r >= 'ⅰ' && r <= 'ⅿ' {
		return romanLetters[r-'ⅰ']
	}
	return ""
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

// superscriptDigit returns the ASCII digit a superscript digit stands for, or 0.
func superscriptDigit(r rune) byte {
	switch r {
	case '¹':
		return '1'
	case '²':
		return '2'
	case '³':
		return '3'
	}
	if r >= '⁰' && r <= '⁹' && r != 'ⁱ' && r != '⁲' && r != '⁳' {
		return byte('0' + r - '⁰')
	}
	return 0
}
