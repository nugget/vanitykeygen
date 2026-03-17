package server

import (
	"net/http"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if targets == nil {
		targets = []vkg.Target{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": targets})
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var t vkg.Target
	if err := s.readJSON(r, &t); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if t.Pattern == "" {
		s.writeError(w, http.StatusBadRequest, "pattern is required")
		return
	}
	if err := s.store.CreateTarget(r.Context(), &t); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast("target_update", t)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": t})
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.GetTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		s.writeError(w, http.StatusNotFound, "target not found")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": t})
}

func (s *Server) handleGetActiveTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListActiveTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	compiled := compilePatterns(targets)
	if compiled == nil {
		compiled = []vkg.CompiledPattern{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": compiled})
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.store.GetTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		s.writeError(w, http.StatusNotFound, "target not found")
		return
	}

	var t vkg.Target
	if err := s.readJSON(r, &t); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t.ID = id
	if t.Pattern == "" {
		t.Pattern = existing.Pattern
	}
	if err := s.store.UpdateTarget(r.Context(), &t); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast("target_update", t)
	s.writeJSON(w, http.StatusOK, map[string]any{"data": t})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteTarget(r.Context(), id); err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast("target_update", map[string]string{"deleted": id})
	w.WriteHeader(http.StatusNoContent)
}
