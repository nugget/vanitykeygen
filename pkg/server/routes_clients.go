package server

import (
	"net/http"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req vkg.RegisterRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}

	clientID := req.ClientID
	if clientID == "" {
		id, err := vkg.NewID()
		if err != nil {
			s.writeError(w, r, http.StatusInternalServerError, "failed to generate client ID")
			return
		}
		clientID = id
	}

	c := &vkg.ClientInfo{
		ID:       clientID,
		Hostname: req.Hostname,
		Version:  req.Version,
		Seekers:  req.Seekers,
		LastSeen: time.Now().UTC(),
		Status:   "active",
	}

	if err := s.store.UpsertClient(r.Context(), c); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("client registered", "client_id", c.ID, "hostname", c.Hostname, "seekers", c.Seekers)
	s.hub.Broadcast("client_update", c)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": vkg.RegisterResponse{ClientID: c.ID}})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var hb vkg.Heartbeat
	if err := s.readJSON(r, &hb); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}
	if hb.ClientID == "" {
		s.writeError(w, r, http.StatusBadRequest, "client_id is required")
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
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	s.hub.Broadcast("client_update", c)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.store.ListClients(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if clients == nil {
		clients = []vkg.ClientInfo{}
	}
	limit := parseLimit(r, 100)
	if len(clients) > limit {
		clients = clients[:limit]
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": clients})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	clients, err := s.store.ListClients(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	matchCount, err := s.store.MatchCount(r.Context(), "")
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
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
			"active_clients":  activeClients,
			"total_key_rate":  totalKeyRate,
			"total_key_count": totalKeyCount,
			"total_matches":   matchCount,
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
