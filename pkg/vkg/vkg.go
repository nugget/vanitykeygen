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
	CaseModes  []string  `json:"caseModes"`  // any of: "insensitive", "sensitive", "capitalized"
	MatchScope string    `json:"matchScope"` // "fingerprint", "pubkey", or "both"
	CreatedAt  time.Time `json:"createdAt"`
}

// CompiledPattern is a regex pattern sent to clients for key testing.
type CompiledPattern struct {
	Pattern            string `json:"pattern"`
	MatchFingerprint   bool   `json:"matchFingerprint"`
	MatchAuthorizedKey bool   `json:"matchAuthorizedKey"`
}

// Key holds the cryptographic material for a generated SSH key.
type Key struct {
	PrivateKey       []byte `json:"privateKey"`
	PublicKey        []byte `json:"publicKey"`
	EncodedKey       []byte `json:"encodedKey"`
	PrivateString    string `json:"privateString"`
	AuthorizedString string `json:"authorizedString"`
	Fingerprint      string `json:"fingerprint"`
}

// Match records a successful key match against a target pattern.
type Match struct {
	ID                   string    `json:"id"`
	TargetID             string    `json:"targetId"`
	ClientID             string    `json:"clientId"`
	Timestamp            time.Time `json:"timestamp"`
	Hostname             string    `json:"hostname"`
	SeekerID             int       `json:"seekerID"`
	MatchString          string    `json:"matchString"`
	MatchedAuthorizedKey bool      `json:"matchedAuthorizedKey"`
	MatchedFingerprint   bool      `json:"matchedFingerprint"`
	Key                  Key       `json:"key"`
}

// ClientInfo describes a connected client in the fleet.
type ClientInfo struct {
	ID       string    `json:"id"`
	Hostname string    `json:"hostname"`
	Version  string    `json:"version"`
	Seekers  int       `json:"seekers"`
	KeyRate  float64   `json:"keyRate"`
	KeyCount int64     `json:"keyCount"`
	LastSeen time.Time `json:"lastSeen"`
	Status   string    `json:"status"` // "active", "idle", "offline"
}

// Heartbeat is sent periodically by clients to report status.
type Heartbeat struct {
	ClientID string  `json:"clientId"`
	Hostname string  `json:"hostname"`
	Version  string  `json:"version"`
	Seekers  int     `json:"seekers"`
	KeyRate  float64 `json:"keyRate"`
	KeyCount int64   `json:"keyCount"`
}

// RegisterRequest is sent by a client on startup.
// If ClientID is set, the server reuses it (stable across restarts).
type RegisterRequest struct {
	ClientID string `json:"clientId,omitempty"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Seekers  int    `json:"seekers"`
}

// RegisterResponse is returned after client registration.
type RegisterResponse struct {
	ClientID string `json:"clientId"`
}

// NewID generates a random 16-character hex ID.
func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}
