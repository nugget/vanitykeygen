package server

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// compilePatterns builds the set of regex patterns that clients should test keys against.
// Word targets are grouped by (caseSensitive, matchScope) into combined regexes.
// Regex targets are passed through individually.
func compilePatterns(targets []vkg.Target) []vkg.CompiledPattern {
	// Group word targets by (caseSensitive, matchScope).
	type wordKey struct {
		caseSensitive bool
		matchScope    string
	}
	wordGroups := make(map[wordKey][]string)
	wordGroupOrder := []wordKey{} // preserve insertion order

	var patterns []vkg.CompiledPattern

	for _, t := range targets {
		scope := normalizeScope(t.MatchScope)

		if t.Type == "word" {
			k := wordKey{caseSensitive: t.CaseSensitive, matchScope: scope}
			if _, exists := wordGroups[k]; !exists {
				wordGroupOrder = append(wordGroupOrder, k)
			}
			wordGroups[k] = append(wordGroups[k], regexp.QuoteMeta(t.Pattern))
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
		if k.caseSensitive {
			pattern = fmt.Sprintf("%s(%s)%s", wordPrefix, alternation, wordSuffix)
		} else {
			pattern = fmt.Sprintf("(?i)%s(%s)%s", wordPrefix, alternation, wordSuffix)
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
