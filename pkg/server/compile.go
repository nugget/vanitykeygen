package server

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// compilePatterns builds the set of regex patterns that clients should test keys against.
// Word targets are grouped by (caseMode, matchScope) into combined regexes.
// Regex targets are passed through individually.
func compilePatterns(targets []vkg.Target) []vkg.CompiledPattern {
	// Group word targets by (caseMode, matchScope).
	type wordKey struct {
		caseMode   string
		matchScope string
	}
	wordGroups := make(map[wordKey][]string)
	wordGroupOrder := []wordKey{} // preserve insertion order

	var patterns []vkg.CompiledPattern

	for _, t := range targets {
		scope := normalizeScope(t.MatchScope)

		if t.Type == "word" {
			caseMode := normalizeCaseMode(t.CaseMode)
			k := wordKey{caseMode: caseMode, matchScope: scope}
			if _, exists := wordGroups[k]; !exists {
				wordGroupOrder = append(wordGroupOrder, k)
			}
			word := t.Pattern
			if caseMode == "capitalized" {
				word = capitalize(word)
			}
			wordGroups[k] = append(wordGroups[k], regexp.QuoteMeta(word))
		} else {
			// Raw regex — pass through as-is.
			patterns = append(patterns, vkg.CompiledPattern{
				Pattern:            t.Pattern,
				MatchFingerprint:   scope == "fingerprint" || scope == "both",
				MatchAuthorizedKey: scope == "pubkey" || scope == "both",
			})
		}
	}

	// Build combined regex for each word group.
	for _, k := range wordGroupOrder {
		words := wordGroups[k]
		alternation := strings.Join(words, "|")
		var pattern string
		switch k.caseMode {
		case "insensitive":
			pattern = fmt.Sprintf("(?i)%s(%s)%s", wordPrefix, alternation, wordSuffix)
		default: // "sensitive" and "capitalized" are both case-sensitive regex
			pattern = fmt.Sprintf("%s(%s)%s", wordPrefix, alternation, wordSuffix)
		}
		patterns = append(patterns, vkg.CompiledPattern{
			Pattern:            pattern,
			MatchFingerprint:   k.matchScope == "fingerprint" || k.matchScope == "both",
			MatchAuthorizedKey: k.matchScope == "pubkey" || k.matchScope == "both",
		})
	}

	return patterns
}

func normalizeScope(scope string) string {
	switch scope {
	case "fingerprint", "pubkey":
		return scope
	default:
		return "both"
	}
}

func normalizeCaseMode(mode string) string {
	switch mode {
	case "sensitive", "capitalized":
		return mode
	default:
		return "insensitive"
	}
}

// capitalize returns the word with the first letter uppercased and the rest lowercased.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}
