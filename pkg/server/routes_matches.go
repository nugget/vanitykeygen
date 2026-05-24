package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	targetID := r.URL.Query().Get("target_id")
	limit := parseLimit(r, 100)

	matches, err := s.store.ListMatches(r.Context(), targetID, limit)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
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
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if m == nil {
		s.writeError(w, r, http.StatusNotFound, "match not found")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": m})
}

func (s *Server) handlePostMatch(w http.ResponseWriter, r *http.Request) {
	var m vkg.Match
	if err := s.readJSON(r, &m); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Server owns ID, Timestamp, and TargetID. Clear any client-supplied
	// values so the store generates a fresh ID and the server attributes
	// the match below.
	m.ID = ""
	m.Timestamp = time.Time{}
	m.TargetID = ""

	s.logger.Info("match received",
		"hostname", m.Hostname,
		"client_id", m.ClientID,
		"fingerprint", m.Key.Fingerprint,
		"authorized_key", m.Key.AuthorizedString,
		"match_string", m.MatchString,
	)

	if tid := s.attributeMatch(r, &m); tid != "" {
		m.TargetID = tid
	}

	if err := s.store.RecordMatch(r.Context(), &m); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// SSE broadcast must not include private key material.
	s.hub.Broadcast("match", m.Summary())
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
			re, err := s.cachedCompile(t.Pattern)
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
