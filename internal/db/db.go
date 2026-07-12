// Package db persists usage results and metadata in SQLite (modernc.org/sqlite).
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver

	"usage-gauge/internal/types"
)

const schema = `
CREATE TABLE IF NOT EXISTS usage (
  name       TEXT PRIMARY KEY,
  payload    TEXT NOT NULL,
  status     TEXT NOT NULL,
  queried_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

const (
	metaLastSuccessAt = "last_success_at"
)

// Store wraps a SQLite connection.
type Store struct {
	db *sql.DB
}

// Open opens or creates the database at path and initializes the schema.
// The parent directory is created if missing.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A single writer connection avoids "database is locked" under concurrent access.
	d.SetMaxOpenConns(1)
	if _, err := d.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;`); err != nil {
		d.Close()
		return nil, err
	}
	if _, err := d.Exec(schema); err != nil {
		d.Close()
		return nil, err
	}
	return &Store{db: d}, nil
}

// Close closes the underlying connection.
func (s *Store) Close() error { return s.db.Close() }

// Upsert stores the latest result for an endpoint.
func (s *Store) Upsert(name string, r types.UsageResult, updatedAt int64) error {
	payload, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO usage (name, payload, status, queried_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   payload = excluded.payload,
		   status = excluded.status,
		   queried_at = excluded.queried_at,
		   updated_at = excluded.updated_at`,
		name, string(payload), string(r.Status), r.QueriedAt, updatedAt,
	)
	return err
}

// All returns every stored usage record, ordered by name for stable display.
func (s *Store) All() ([]types.UsageRecord, error) {
	rows, err := s.db.Query(`SELECT name, payload, updated_at FROM usage ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.UsageRecord
	for rows.Next() {
		var name, payload string
		var updatedAt int64
		if err := rows.Scan(&name, &payload, &updatedAt); err != nil {
			return nil, err
		}
		var r types.UsageResult
		if err := json.Unmarshal([]byte(payload), &r); err != nil {
			return nil, fmt.Errorf("unmarshal payload for %s: %w", name, err)
		}
		out = append(out, types.UsageRecord{Name: name, UpdatedAt: updatedAt, UsageResult: r})
	}
	return out, rows.Err()
}

// SetMeta sets a meta key/value (upsert).
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// GetMeta returns a meta value, or "" when absent.
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// LastSuccessAt returns the epoch-ms timestamp of the most recent successful
// refresh, or 0 if none has succeeded yet.
func (s *Store) LastSuccessAt() (int64, error) {
	v, err := s.GetMeta(metaLastSuccessAt)
	if err != nil {
		return 0, err
	}
	if v == "" {
		return 0, nil
	}
	var n int64
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// MarkLastSuccess records the epoch-ms timestamp of a successful refresh.
func (s *Store) MarkLastSuccess(at int64) error {
	return s.SetMeta(metaLastSuccessAt, fmt.Sprintf("%d", at))
}

// DeleteNotIn deletes usage records whose name is not in keep. Returns the
// number of rows deleted.
func (s *Store) DeleteNotIn(keep []string) (int64, error) {
	if len(keep) == 0 {
		res, err := s.db.Exec(`DELETE FROM usage`)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	placeholders := make([]string, len(keep))
	args := make([]any, len(keep))
	for i, name := range keep {
		placeholders[i] = "?"
		args[i] = name
	}
	query := `DELETE FROM usage WHERE name NOT IN (` + strings.Join(placeholders, ",") + `)`
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
