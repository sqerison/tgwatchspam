package filter

import "strings"

// MatchWord checks if text (after normalization) contains any stored word as a substring. [FEAT-001]
// Words in the DB are already normalized at insert time, so only the message is normalized here.
// No word boundaries — "доход" matches "доходності", "binance" matches "binance/bybit".
func MatchWord(text string, words []string) (matched string, ok bool) {
	normalText := Normalize(text)
	for _, w := range words {
		if strings.Contains(normalText, w) {
			return w, true
		}
	}
	return "", false
}
