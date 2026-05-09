// Package vkg defines the cross-package data types shared by the VKG
// server and client: targets, matches, key material, client registration,
// and heartbeats. All JSON-facing structs use snake_case field names.
package vkg

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Target represents a search pattern for generated keys.
// Type is "word" (simple word match with server-generated regex) or "regex" (raw regex).
// CaseModes controls word matching: any combination of "insensitive", "sensitive", "capitalized".
// MatchScope controls what is tested: "fingerprint", "pubkey", or "both".
type Target struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`    // "word" or "regex"
	Pattern    string    `json:"pattern"` // word or raw regex
	Label      string    `json:"label"`
	Active     bool      `json:"active"`
	CaseModes  []string  `json:"case_modes"`  // any of: "insensitive", "sensitive", "capitalized"
	MatchScope string    `json:"match_scope"` // "fingerprint", "pubkey", or "both"
	CreatedAt  time.Time `json:"created_at"`
}

// CompiledPattern is a regex pattern sent to clients for key testing.
type CompiledPattern struct {
	Pattern            string `json:"pattern"`
	MatchFingerprint   bool   `json:"match_fingerprint"`
	MatchAuthorizedKey bool   `json:"match_authorized_key"`
}

// Key holds the cryptographic material for a generated SSH key.
type Key struct {
	PrivateKey       []byte `json:"private_key"`
	PublicKey        []byte `json:"public_key"`
	EncodedKey       []byte `json:"encoded_key"`
	PrivateString    string `json:"private_string"`
	AuthorizedString string `json:"authorized_string"`
	Fingerprint      string `json:"fingerprint"`
}

// Match records a successful key match against a target pattern.
type Match struct {
	ID                   string    `json:"id"`
	TargetID             string    `json:"target_id"`
	ClientID             string    `json:"client_id"`
	Timestamp            time.Time `json:"timestamp"`
	Hostname             string    `json:"hostname"`
	SeekerID             int       `json:"seeker_id"`
	MatchString          string    `json:"match_string"`
	MatchedAuthorizedKey bool      `json:"matched_authorized_key"`
	MatchedFingerprint   bool      `json:"matched_fingerprint"`
	Key                  Key       `json:"key"`
}

// MatchSummary is the public-safe view of a Match used for SSE
// broadcasts and any other context where the full Key payload must
// not leave the server. The fingerprint is included because it is
// the public identifier shown to operators; private key material is
// only ever returned via the authenticated GET /api/matches/{id}.
type MatchSummary struct {
	ID                   string    `json:"id"`
	TargetID             string    `json:"target_id"`
	ClientID             string    `json:"client_id"`
	Timestamp            time.Time `json:"timestamp"`
	Hostname             string    `json:"hostname"`
	MatchString          string    `json:"match_string"`
	MatchedAuthorizedKey bool      `json:"matched_authorized_key"`
	MatchedFingerprint   bool      `json:"matched_fingerprint"`
	Fingerprint          string    `json:"fingerprint"`
}

// Summary returns the public-safe projection of a Match.
func (m Match) Summary() MatchSummary {
	return MatchSummary{
		ID:                   m.ID,
		TargetID:             m.TargetID,
		ClientID:             m.ClientID,
		Timestamp:            m.Timestamp,
		Hostname:             m.Hostname,
		MatchString:          m.MatchString,
		MatchedAuthorizedKey: m.MatchedAuthorizedKey,
		MatchedFingerprint:   m.MatchedFingerprint,
		Fingerprint:          m.Key.Fingerprint,
	}
}

// ClientInfo describes a connected client in the fleet.
type ClientInfo struct {
	ID       string    `json:"id"`
	Hostname string    `json:"hostname"`
	Version  string    `json:"version"`
	Seekers  int       `json:"seekers"`
	KeyRate  float64   `json:"key_rate"`
	KeyCount int64     `json:"key_count"`
	LastSeen time.Time `json:"last_seen"`
	Status   string    `json:"status"` // "active", "idle", "offline"
}

// Heartbeat is sent periodically by clients to report status.
type Heartbeat struct {
	ClientID string  `json:"client_id"`
	Hostname string  `json:"hostname"`
	Version  string  `json:"version"`
	Seekers  int     `json:"seekers"`
	KeyRate  float64 `json:"key_rate"`
	KeyCount int64   `json:"key_count"`
}

// RegisterRequest is sent by a client on startup.
// If ClientID is set, the server reuses it (stable across restarts).
type RegisterRequest struct {
	ClientID string `json:"client_id,omitempty"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Seekers  int    `json:"seekers"`
}

// RegisterResponse is returned after client registration.
type RegisterResponse struct {
	ClientID string `json:"client_id"`
}

// NewID generates a random 16-character hex ID.
func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}
