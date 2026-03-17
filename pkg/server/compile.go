package server

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// compilePatterns builds the set of regex patterns that clients should test keys against.
// Word targets are grouped into case-insensitive and case-sensitive regex groups.
// A single word target can contribute to multiple groups based on its CaseModes.
// Regex targets are passed through individually.
func compilePatterns(targets []vkg.Target) []vkg.CompiledPattern {
	type wordKey struct {
		caseInsensitive bool
		matchScope      string
	}
	wordGroups := make(map[wordKey][]string)
	wordGroupOrder := []wordKey{}

	var patterns []vkg.CompiledPattern

	for _, t := range targets {
		scope := normalizeScope(t.MatchScope)

		if t.Type == "word" {
			modes := t.CaseModes
			if len(modes) == 0 {
				modes = []string{"insensitive"}
			}

			for _, mode := range modes {
				var word string
				var ci bool

				switch mode {
				case "insensitive":
					word = t.Pattern
					ci = true
				case "sensitive":
					word = t.Pattern
					ci = false
				case "capitalized":
					word = capitalize(t.Pattern)
					ci = false
				default:
					continue
				}

				k := wordKey{caseInsensitive: ci, matchScope: scope}
				if _, exists := wordGroups[k]; !exists {
					wordGroupOrder = append(wordGroupOrder, k)
				}
				// Avoid duplicates within the same group
				quoted := regexp.QuoteMeta(word)
				if !contains(wordGroups[k], quoted) {
					wordGroups[k] = append(wordGroups[k], quoted)
				}
			}
		} else {
			patterns = append(patterns, vkg.CompiledPattern{
				Pattern:            t.Pattern,
				MatchFingerprint:   scope == "fingerprint" || scope == "both",
				MatchAuthorizedKey: scope == "pubkey" || scope == "both",
			})
		}
	}

	for _, k := range wordGroupOrder {
		words := wordGroups[k]
		alternation := strings.Join(words, "|")
		var pattern string
		if k.caseInsensitive {
			pattern = fmt.Sprintf("(?i)%s(%s)%s", wordPrefix, alternation, wordSuffix)
		} else {
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

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
