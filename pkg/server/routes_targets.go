package server

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"
)

// base64Chars is the standard base64 alphabet (the character set that
// can appear in raw SSH fingerprints and authorized_keys output). The
// "SHA256:" prefix that ssh-keygen renders is stripped before
// matching, so its characters are not included here.
const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="

// Allowed enum values for Target fields.
var (
	allowedTargetTypes = map[string]bool{"word": true, "regex": true}
	allowedMatchScopes = map[string]bool{"fingerprint": true, "pubkey": true, "both": true}
	allowedCaseModes   = map[string]bool{"insensitive": true, "sensitive": true, "capitalized": true}
)

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

// validateTargetEnums checks that Type, MatchScope, and CaseModes are
// from the supported sets. Empty values are tolerated; defaults are
// applied later in the store layer.
func validateTargetEnums(t *vkg.Target) error {
	if t.Type != "" && !allowedTargetTypes[t.Type] {
		return fmt.Errorf("type must be one of word, regex (got %q)", t.Type)
	}
	if t.MatchScope != "" && !allowedMatchScopes[t.MatchScope] {
		return fmt.Errorf("match_scope must be one of fingerprint, pubkey, both (got %q)", t.MatchScope)
	}
	for _, m := range t.CaseModes {
		if !allowedCaseModes[m] {
			return fmt.Errorf("case_modes must be from {insensitive, sensitive, capitalized} (got %q)", m)
		}
	}
	return nil
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if targets == nil {
		targets = []vkg.Target{}
	}
	limit := parseLimit(r, 100)
	if len(targets) > limit {
		targets = targets[:limit]
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": targets})
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var t vkg.Target
	if err := s.readJSON(r, &t); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Server owns ID and CreatedAt — clear any client-supplied values.
	t.ID = ""
	t.CreatedAt = time.Time{}
	if t.Pattern == "" {
		s.writeError(w, r, http.StatusBadRequest, "pattern is required")
		return
	}
	if err := validateTargetEnums(&t); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if t.Type == "word" {
		if err := validateWordPattern(t.Pattern); err != nil {
			s.writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := validateRegexPattern(t.Pattern); err != nil {
			s.writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.store.CreateTarget(r.Context(), &t); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast("target_update", t)
	s.writeJSON(w, http.StatusCreated, map[string]any{"data": t})
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.GetTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		s.writeError(w, r, http.StatusNotFound, "target not found")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"data": t})
}

func (s *Server) handleGetActiveTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListActiveTargets(r.Context())
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
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
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		s.writeError(w, r, http.StatusNotFound, "target not found")
		return
	}

	var t vkg.Target
	if err := s.readJSON(r, &t); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "invalid JSON")
		return
	}
	// PUT replaces the resource. Server owns ID and CreatedAt; everything
	// else must be supplied by the caller. There is no field-level merge,
	// so an omitted boolean or empty string is treated as the new value.
	t.ID = id
	t.CreatedAt = existing.CreatedAt
	if t.Type == "" {
		s.writeError(w, r, http.StatusBadRequest, "type is required")
		return
	}
	if t.Pattern == "" {
		s.writeError(w, r, http.StatusBadRequest, "pattern is required")
		return
	}
	if err := validateTargetEnums(&t); err != nil {
		s.writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if t.Type == "word" {
		if err := validateWordPattern(t.Pattern); err != nil {
			s.writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		if err := validateRegexPattern(t.Pattern); err != nil {
			s.writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.store.UpdateTarget(r.Context(), &t); err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.hub.Broadcast("target_update", t)
	s.writeJSON(w, http.StatusOK, map[string]any{"data": t})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	matchesDeleted, targetDeleted, err := s.store.DeleteTarget(r.Context(), id)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !targetDeleted {
		s.writeError(w, r, http.StatusNotFound, "target not found")
		return
	}
	if matchesDeleted > 0 {
		s.logger.Info("cascade deleted matches with target", "target_id", id, "matches_deleted", matchesDeleted)
	}
	s.hub.Broadcast("target_update", map[string]string{"deleted": id})
	w.WriteHeader(http.StatusNoContent)
}
