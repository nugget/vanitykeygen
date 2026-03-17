package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// base64Chars is the set of characters that can appear in SSH fingerprints and
// authorized key strings (standard base64 alphabet plus the SHA256: prefix chars).
const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="

// validateWordPattern checks that a word target only contains characters that
// could actually appear in fingerprint or pubkey output.
func validateWordPattern(word string) error {
	for _, r := range word {
		if !strings.ContainsRune(base64Chars, r) {
			return fmt.Errorf("character %q cannot appear in SSH fingerprints or public keys", string(r))
		}
	}
	return nil
}

// validateRegexPattern checks that a regex target compiles.
func validateRegexPattern(pattern string) error {
	_, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	return nil
}

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
	if t.Type == "word" {
		if err := validateWordPattern(t.Pattern); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := validateRegexPattern(t.Pattern); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	if t.Type == "word" {
		if err := validateWordPattern(t.Pattern); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if t.Pattern != existing.Pattern {
		if err := validateRegexPattern(t.Pattern); err != nil {
			s.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	matchesDeleted, err := s.store.DeleteTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if matchesDeleted > 0 {
		s.logger.Info("cascade deleted matches with target", "targetId", id, "matchesDeleted", matchesDeleted)
	}
	s.hub.Broadcast("target_update", map[string]string{"deleted": id})
	w.WriteHeader(http.StatusNoContent)
}
