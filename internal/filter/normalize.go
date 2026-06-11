package filter

import "strings"

// lookalikesMap maps Cyrillic visually-identical characters to their Latin equivalents. [FEAT-003]
// Applied to both stored words (at insert time) and incoming messages (before matching).
var lookalikesMap = map[rune]rune{
	// Lowercase — visually identical to Latin counterparts
	'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c',
	'х': 'x', 'у': 'y', 'і': 'i',
	// Lowercase — close enough to be used as bypass in sans-serif fonts
	'м': 'm', 'т': 't',
	// Uppercase — visually identical
	'А': 'A', 'В': 'B', 'Е': 'E', 'К': 'K', 'М': 'M',
	'Н': 'H', 'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T',
	'Х': 'X', 'У': 'Y',
}

// Normalize replaces Cyrillic lookalike characters with Latin equivalents, then lowercases.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if latin, ok := lookalikesMap[r]; ok {
			b.WriteRune(latin)
		} else {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(b.String())
}
