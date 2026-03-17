package server

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("targetId")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	matches, err := s.store.ListMatches(r.Context(), targetID, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if matches == nil {
		matches = []vkg.Match{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": matches})
}

func (s *Server) handleGetMatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.store.GetMatch(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		s.writeError(w, http.StatusNotFound, "match not found")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": m})
}

func (s *Server) handlePostMatch(w http.ResponseWriter, r *http.Request) {
	var m vkg.Match
	if err := s.readJSON(r, &m); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	s.logger.Info("match received",
		"hostname", m.Hostname,
		"clientId", m.ClientID,
		"fingerprint", m.Key.Fingerprint,
		"authorizedKey", m.Key.AuthorizedString,
		"matchString", m.MatchString,
	)

	// Attribute match to a specific target if not already set.
	if m.TargetID == "" {
		if tid := s.attributeMatch(r, &m); tid != "" {
			m.TargetID = tid
		}
	}

	if err := s.store.RecordMatch(r.Context(), &m); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.hub.Broadcast("match", m)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": m})
}

// attributeMatch tests a match against active targets to find which one it belongs to.
func (s *Server) attributeMatch(r *http.Request, m *vkg.Match) string {
	targets, err := s.store.ListActiveTargets(r.Context())
	if err != nil {
		return ""
	}

	for _, t := range targets {
		scope := normalizeScope(t.MatchScope)

		if t.Type == "word" {
			modes := t.CaseModes
			if len(modes) == 0 {
				modes = []string{"insensitive"}
			}

			for _, mode := range modes {
				var word string
				var caseInsensitive bool

				switch mode {
				case "insensitive":
					word = t.Pattern
					caseInsensitive = true
				case "sensitive":
					word = t.Pattern
					caseInsensitive = false
				case "capitalized":
					word = capitalize(t.Pattern)
					caseInsensitive = false
				default:
					continue
				}

				if (scope == "fingerprint" || scope == "both") && m.MatchedFingerprint {
					if containsWord(m.Key.Fingerprint, word, caseInsensitive) {
						return t.ID
					}
				}
				if (scope == "pubkey" || scope == "both") && m.MatchedAuthorizedKey {
					if containsWord(m.Key.AuthorizedString, word, caseInsensitive) {
						return t.ID
					}
				}
			}
		} else {
			re, err := regexp.Compile(t.Pattern)
			if err != nil {
				continue
			}
			if (scope == "fingerprint" || scope == "both") && m.MatchedFingerprint {
				if re.MatchString(m.Key.Fingerprint) {
					return t.ID
				}
			}
			if (scope == "pubkey" || scope == "both") && m.MatchedAuthorizedKey {
				if re.MatchString(m.Key.AuthorizedString) {
					return t.ID
				}
			}
		}
	}
	return ""
}

// containsWord checks if haystack contains needle, optionally case-insensitive.
func containsWord(haystack, needle string, caseInsensitive bool) bool {
	if caseInsensitive {
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
	}
	return strings.Contains(haystack, needle)
}
