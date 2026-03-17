package server

import (
	"net/http"
	"strconv"

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

	if err := s.store.RecordMatch(r.Context(), &m); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.hub.Broadcast("match", m)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": m})
}
