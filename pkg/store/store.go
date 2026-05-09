// Package store provides SQLite-backed persistence for VKG targets,
// matches, and connected clients. It is the only package that issues
// SQL — all other packages call Store methods.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/nugget/vanitykeygen/pkg/vkg"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS targets (
	id          TEXT PRIMARY KEY,
	type        TEXT NOT NULL DEFAULT 'regex',
	pattern     TEXT NOT NULL,
	label       TEXT NOT NULL DEFAULT '',
	active      INTEGER NOT NULL DEFAULT 0,
	case_modes   TEXT NOT NULL DEFAULT 'insensitive',
	match_scope TEXT NOT NULL DEFAULT 'both',
	created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS matches (
	id                     TEXT PRIMARY KEY,
	target_id              TEXT NOT NULL DEFAULT '',
	client_id              TEXT NOT NULL DEFAULT '',
	timestamp              TEXT NOT NULL DEFAULT (datetime('now')),
	hostname               TEXT NOT NULL DEFAULT '',
	seeker_id              INTEGER NOT NULL DEFAULT 0,
	match_string           TEXT NOT NULL DEFAULT '',
	matched_authorized_key INTEGER NOT NULL DEFAULT 0,
	matched_fingerprint    INTEGER NOT NULL DEFAULT 0,
	private_key            BLOB,
	public_key             BLOB,
	encoded_key            BLOB,
	private_string         TEXT NOT NULL DEFAULT '',
	authorized_string      TEXT NOT NULL DEFAULT '',
	fingerprint            TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS clients (
	id        TEXT PRIMARY KEY,
	hostname  TEXT NOT NULL DEFAULT '',
	version   TEXT NOT NULL DEFAULT '',
	seekers   INTEGER NOT NULL DEFAULT 0,
	key_rate  REAL NOT NULL DEFAULT 0,
	key_count INTEGER NOT NULL DEFAULT 0,
	last_seen TEXT NOT NULL DEFAULT (datetime('now')),
	status    TEXT NOT NULL DEFAULT 'active'
);
`

// Store provides SQLite-backed persistence.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database and runs migrations.
//
// In-memory paths (":memory:" or "file::memory:...") are pinned to a
// single connection because each new database/sql connection to an
// in-memory database opens its own empty database, which would lose
// the schema and any data written through other connections.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if isInMemoryDSN(dbPath) {
		db.SetMaxOpenConns(1)
	}
	// SQLite performance pragmas
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func isInMemoryDSN(dsn string) bool {
	return dsn == ":memory:" || strings.Contains(dsn, ":memory:")
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Targets ---

// CreateTarget inserts a new target. If t.ID is empty a fresh ID is
// generated and assigned to t. Defaults are applied for Type, CaseModes,
// MatchScope, and CreatedAt when not set.
func (s *Store) CreateTarget(ctx context.Context, t *vkg.Target) error {
	if t.ID == "" {
		id, err := vkg.NewID()
		if err != nil {
			return err
		}
		t.ID = id
	}
	if t.Type == "" {
		t.Type = "regex"
	}
	if len(t.CaseModes) == 0 {
		t.CaseModes = []string{"insensitive"}
	}
	if t.MatchScope == "" {
		t.MatchScope = "both"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO targets (id, type, pattern, label, active, case_modes, match_scope, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Type, t.Pattern, t.Label, t.Active,
		strings.Join(t.CaseModes, ","), t.MatchScope,
		t.CreatedAt.Format(time.RFC3339))
	return err
}

// ListTargets returns every target ordered by creation time (newest
// first).
func (s *Store) ListTargets(ctx context.Context) ([]vkg.Target, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, pattern, label, active, case_modes, match_scope, created_at
		 FROM targets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanTargets(rows)
}

// GetTarget loads a single target by ID. It returns (nil, nil) when no
// target with the given ID exists.
func (s *Store) GetTarget(ctx context.Context, id string) (*vkg.Target, error) {
	var t vkg.Target
	var createdAt, caseModes string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, type, pattern, label, active, case_modes, match_scope, created_at
		 FROM targets WHERE id = ?`, id).
		Scan(&t.ID, &t.Type, &t.Pattern, &t.Label, &t.Active, &caseModes, &t.MatchScope, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CaseModes = splitCaseModes(caseModes)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

// ListActiveTargets returns every target where Active is true, ordered
// by creation time (newest first).
func (s *Store) ListActiveTargets(ctx context.Context) ([]vkg.Target, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, pattern, label, active, case_modes, match_scope, created_at
		 FROM targets WHERE active = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanTargets(rows)
}

func scanTargets(rows *sql.Rows) ([]vkg.Target, error) {
	var targets []vkg.Target
	for rows.Next() {
		var t vkg.Target
		var createdAt, caseModes string
		if err := rows.Scan(&t.ID, &t.Type, &t.Pattern, &t.Label, &t.Active,
			&caseModes, &t.MatchScope, &createdAt); err != nil {
			return nil, err
		}
		t.CaseModes = splitCaseModes(caseModes)
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func splitCaseModes(s string) []string {
	if s == "" {
		return []string{"insensitive"}
	}
	return strings.Split(s, ",")
}

// UpdateTarget overwrites all mutable fields of the target identified
// by t.ID. CreatedAt is not updated.
func (s *Store) UpdateTarget(ctx context.Context, t *vkg.Target) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET type = ?, pattern = ?, label = ?, active = ?,
		 case_modes = ?, match_scope = ? WHERE id = ?`,
		t.Type, t.Pattern, t.Label, t.Active,
		strings.Join(t.CaseModes, ","), t.MatchScope, t.ID)
	return err
}

// DeleteTarget removes a target and cascades deletion of all matches
// attributed to it, atomically. It returns (matchesDeleted,
// targetDeleted, error); targetDeleted is false when no row matched
// the given id, which the caller should surface as a 404.
func (s *Store) DeleteTarget(ctx context.Context, id string) (matchesDeleted int64, targetDeleted bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `DELETE FROM matches WHERE target_id = ?`, id)
	if err != nil {
		return 0, false, err
	}
	matchesDeleted, _ = res.RowsAffected()

	res, err = tx.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	if err != nil {
		return 0, false, err
	}
	tgtRows, _ := res.RowsAffected()
	if err = tx.Commit(); err != nil {
		return 0, false, err
	}
	return matchesDeleted, tgtRows > 0, nil
}

// --- Matches ---

// RecordMatch persists a match, generating an ID and timestamp if they
// are not already set.
func (s *Store) RecordMatch(ctx context.Context, m *vkg.Match) error {
	if m.ID == "" {
		id, err := vkg.NewID()
		if err != nil {
			return err
		}
		m.ID = id
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO matches (id, target_id, client_id, timestamp, hostname, seeker_id,
		 match_string, matched_authorized_key, matched_fingerprint,
		 private_key, public_key, encoded_key, private_string, authorized_string, fingerprint)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.TargetID, m.ClientID, m.Timestamp.Format(time.RFC3339),
		m.Hostname, m.SeekerID, m.MatchString,
		m.MatchedAuthorizedKey, m.MatchedFingerprint,
		m.Key.PrivateKey, m.Key.PublicKey, m.Key.EncodedKey,
		m.Key.PrivateString, m.Key.AuthorizedString, m.Key.Fingerprint)
	return err
}

// ListMatches returns matches ordered by timestamp (newest first),
// optionally filtered by targetID. A non-positive limit is clamped to
// the default of 100.
func (s *Store) ListMatches(ctx context.Context, targetID string, limit int) ([]vkg.Match, error) {
	var rows *sql.Rows
	var err error
	if limit <= 0 {
		limit = 100
	}
	if targetID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, target_id, client_id, timestamp, hostname, seeker_id,
			 match_string, matched_authorized_key, matched_fingerprint,
			 private_key, public_key, encoded_key, private_string, authorized_string, fingerprint
			 FROM matches WHERE target_id = ? ORDER BY timestamp DESC LIMIT ?`, targetID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, target_id, client_id, timestamp, hostname, seeker_id,
			 match_string, matched_authorized_key, matched_fingerprint,
			 private_key, public_key, encoded_key, private_string, authorized_string, fingerprint
			 FROM matches ORDER BY timestamp DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanMatches(rows)
}

// GetMatch loads a single match by ID, including its private key
// material. It returns (nil, nil) when no match with the given ID
// exists.
func (s *Store) GetMatch(ctx context.Context, id string) (*vkg.Match, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, target_id, client_id, timestamp, hostname, seeker_id,
		 match_string, matched_authorized_key, matched_fingerprint,
		 private_key, public_key, encoded_key, private_string, authorized_string, fingerprint
		 FROM matches WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	matches, err := scanMatches(rows)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

func scanMatches(rows *sql.Rows) ([]vkg.Match, error) {
	var matches []vkg.Match
	for rows.Next() {
		var m vkg.Match
		var ts string
		if err := rows.Scan(
			&m.ID, &m.TargetID, &m.ClientID, &ts, &m.Hostname, &m.SeekerID,
			&m.MatchString, &m.MatchedAuthorizedKey, &m.MatchedFingerprint,
			&m.Key.PrivateKey, &m.Key.PublicKey, &m.Key.EncodedKey,
			&m.Key.PrivateString, &m.Key.AuthorizedString, &m.Key.Fingerprint,
		); err != nil {
			return nil, err
		}
		m.Timestamp, _ = time.Parse(time.RFC3339, ts)
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// --- Clients ---

// UpsertClient inserts a new client row or updates the existing row
// keyed on c.ID. An empty ID generates a fresh ID; a zero LastSeen is
// set to the current time.
func (s *Store) UpsertClient(ctx context.Context, c *vkg.ClientInfo) error {
	if c.ID == "" {
		id, err := vkg.NewID()
		if err != nil {
			return err
		}
		c.ID = id
	}
	if c.LastSeen.IsZero() {
		c.LastSeen = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO clients (id, hostname, version, seekers, key_rate, key_count, last_seen, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   hostname = excluded.hostname,
		   version = excluded.version,
		   seekers = excluded.seekers,
		   key_rate = excluded.key_rate,
		   key_count = excluded.key_count,
		   last_seen = excluded.last_seen,
		   status = excluded.status`,
		c.ID, c.Hostname, c.Version, c.Seekers, c.KeyRate, c.KeyCount,
		c.LastSeen.Format(time.RFC3339), c.Status)
	return err
}

// ListClients returns every known client ordered by last-seen time
// (newest first).
func (s *Store) ListClients(ctx context.Context) ([]vkg.ClientInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, version, seekers, key_rate, key_count, last_seen, status
		 FROM clients ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var clients []vkg.ClientInfo
	for rows.Next() {
		var c vkg.ClientInfo
		var lastSeen string
		if err := rows.Scan(&c.ID, &c.Hostname, &c.Version, &c.Seekers, &c.KeyRate, &c.KeyCount, &lastSeen, &c.Status); err != nil {
			return nil, err
		}
		c.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

// MarkOfflineClients sets status = "offline" for every client whose
// last_seen is older than threshold and is not already offline.
func (s *Store) MarkOfflineClients(ctx context.Context, threshold time.Duration) error {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE clients SET status = 'offline' WHERE last_seen < ? AND status != 'offline'`, cutoff)
	return err
}

// DeleteOfflineClients removes clients that have been offline for
// longer than threshold.
func (s *Store) DeleteOfflineClients(ctx context.Context, threshold time.Duration) error {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM clients WHERE status = 'offline' AND last_seen < ?`, cutoff)
	return err
}

// MatchCount returns the total number of matches, optionally filtered by target.
func (s *Store) MatchCount(ctx context.Context, targetID string) (int, error) {
	var count int
	var err error
	if targetID != "" {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM matches WHERE target_id = ?`, targetID).Scan(&count)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM matches`).Scan(&count)
	}
	return count, err
}
