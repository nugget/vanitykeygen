package server

import (
	"net/http"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req vkg.RegisterRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	c := &vkg.ClientInfo{
		ID:       vkg.NewID(),
		Hostname: req.Hostname,
		Version:  req.Version,
		Seekers:  req.Seekers,
		LastSeen: time.Now().UTC(),
		Status:   "active",
	}

	if err := s.store.UpsertClient(r.Context(), c); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("client registered", "clientId", c.ID, "hostname", c.Hostname, "seekers", c.Seekers)
	s.hub.Broadcast("client_update", c)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": vkg.RegisterResponse{ClientID: c.ID}})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hb vkg.Heartbeat
	if err := s.readJSON(r, &hb); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if hb.ClientID == "" {
		s.writeError(w, http.StatusBadRequest, "clientId is required")
		return
	}

	c := &vkg.ClientInfo{
		ID:       hb.ClientID,
		Hostname: hb.Hostname,
		Version:  hb.Version,
		Seekers:  hb.Seekers,
		KeyRate:  hb.KeyRate,
		KeyCount: hb.KeyCount,
		LastSeen: time.Now().UTC(),
		Status:   "active",
	}

	if err := s.store.UpsertClient(r.Context(), c); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.hub.Broadcast("client_update", c)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.store.ListClients(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []vkg.ClientInfo{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": clients})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	clients, err := s.store.ListClients(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	matchCount, err := s.store.MatchCount(r.Context(), "")
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var totalKeyRate float64
	var totalKeyCount int64
	var activeClients int
	for _, c := range clients {
		if c.Status == "active" {
			activeClients++
			totalKeyRate += c.KeyRate
			totalKeyCount += c.KeyCount
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"activeClients": activeClients,
			"totalKeyRate":  totalKeyRate,
			"totalKeyCount": totalKeyCount,
			"totalMatches":  matchCount,
		},
	})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"version": Version,
		},
	})
}
