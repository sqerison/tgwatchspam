package filter

import "github.com/sqerison/tgwatchspam/internal/storage"

// MatchResult describes which filter triggered and what pattern matched.
type MatchResult struct {
	Type    string // "word" or "regex"
	Pattern string // the matched word or regex pattern
}

// Filter orchestrates the full spam detection pipeline. [FEAT-001, FEAT-002, FEAT-003]
type Filter struct {
	storage *storage.Storage
}

func New(s *storage.Storage) *Filter {
	return &Filter{storage: s}
}

// Check runs the message through word and regex filters for the given chat.
// Returns nil if no match. Pipeline: normalize -> word match -> regex match.
func (f *Filter) Check(chatID int64, text string) (*MatchResult, error) {
	words, err := f.storage.GetWords(chatID)
	if err != nil {
		return nil, err
	}
	if matched, ok := MatchWord(text, words); ok {
		return &MatchResult{Type: "word", Pattern: matched}, nil
	}

	patterns, err := f.storage.GetRegexes(chatID)
	if err != nil {
		return nil, err
	}
	if matched, ok := MatchRegex(text, patterns); ok {
		return &MatchResult{Type: "regex", Pattern: matched}, nil
	}

	return nil, nil
}
