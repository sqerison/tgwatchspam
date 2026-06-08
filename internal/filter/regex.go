package filter

import "regexp"

// CompileRegex compiles a user-supplied pattern with (?i) prepended for case-insensitive matching. [FEAT-002]
// Used both to validate patterns at input time and to match messages.
func CompileRegex(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + pattern)
}

// MatchRegex checks if the normalized text matches any stored pattern. [FEAT-002]
// Normalization is applied to the message before matching (patterns are stored as-is).
func MatchRegex(text string, patterns []string) (matched string, ok bool) {
	normalText := Normalize(text)
	for _, p := range patterns {
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			continue // skip invalid patterns silently
		}
		if re.MatchString(normalText) {
			return p, true
		}
	}
	return "", false
}
