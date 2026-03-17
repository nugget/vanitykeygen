package store

import (
	"context"
	"database/sql"
	"fmt"
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
	case_mode   TEXT NOT NULL DEFAULT 'insensitive',
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
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// SQLite performance pragmas
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %s: %w", pragma, err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// --- Targets ---

func (s *Store) CreateTarget(ctx context.Context, t *vkg.Target) error {
	if t.ID == "" {
		t.ID = vkg.NewID()
	}
	if t.Type == "" {
		t.Type = "regex"
	}
	if t.MatchScope == "" {
		t.MatchScope = "both"
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO targets (id, type, pattern, label, active, case_mode, match_scope, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Type, t.Pattern, t.Label, t.Active, t.CaseMode, t.MatchScope,
		t.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) ListTargets(ctx context.Context) ([]vkg.Target, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, pattern, label, active, case_mode, match_scope, created_at
		 FROM targets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTargets(rows)
}

func (s *Store) GetTarget(ctx context.Context, id string) (*vkg.Target, error) {
	var t vkg.Target
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, type, pattern, label, active, case_mode, match_scope, created_at
		 FROM targets WHERE id = ?`, id).
		Scan(&t.ID, &t.Type, &t.Pattern, &t.Label, &t.Active, &t.CaseMode, &t.MatchScope, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

func (s *Store) ListActiveTargets(ctx context.Context) ([]vkg.Target, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, pattern, label, active, case_mode, match_scope, created_at
		 FROM targets WHERE active = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTargets(rows)
}

func scanTargets(rows *sql.Rows) ([]vkg.Target, error) {
	var targets []vkg.Target
	for rows.Next() {
		var t vkg.Target
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Type, &t.Pattern, &t.Label, &t.Active,
			&t.CaseMode, &t.MatchScope, &createdAt); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *Store) UpdateTarget(ctx context.Context, t *vkg.Target) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE targets SET type = ?, pattern = ?, label = ?, active = ?,
		 case_mode = ?, match_scope = ? WHERE id = ?`,
		t.Type, t.Pattern, t.Label, t.Active, t.CaseMode, t.MatchScope, t.ID)
	return err
}

func (s *Store) DeleteTarget(ctx context.Context, id string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM matches WHERE target_id = ?`, id)
	if err != nil {
		return 0, err
	}
	deleted, _ := res.RowsAffected()
	_, err = s.db.ExecContext(ctx, `DELETE FROM targets WHERE id = ?`, id)
	return deleted, err
}

// --- Matches ---

func (s *Store) RecordMatch(ctx context.Context, m *vkg.Match) error {
	if m.ID == "" {
		m.ID = vkg.NewID()
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
	defer rows.Close()
	return scanMatches(rows)
}

func (s *Store) GetMatch(ctx context.Context, id string) (*vkg.Match, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, target_id, client_id, timestamp, hostname, seeker_id,
		 match_string, matched_authorized_key, matched_fingerprint,
		 private_key, public_key, encoded_key, private_string, authorized_string, fingerprint
		 FROM matches WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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

func (s *Store) UpsertClient(ctx context.Context, c *vkg.ClientInfo) error {
	if c.ID == "" {
		c.ID = vkg.NewID()
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

func (s *Store) ListClients(ctx context.Context) ([]vkg.ClientInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, hostname, version, seekers, key_rate, key_count, last_seen, status
		 FROM clients ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (s *Store) MarkOfflineClients(ctx context.Context, threshold time.Duration) error {
	cutoff := time.Now().UTC().Add(-threshold).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE clients SET status = 'offline' WHERE last_seen < ? AND status != 'offline'`, cutoff)
	return err
}

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
