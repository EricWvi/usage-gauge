// Package db persists usage results and metadata in SQLite (modernc.org/sqlite).
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
CREATE TABLE IF NOT EXISTS samples (
  name TEXT NOT NULL,
  sampled_at INTEGER NOT NULL,
  payload TEXT NOT NULL,
  PRIMARY KEY (name, sampled_at)
);
CREATE INDEX IF NOT EXISTS samples_time ON samples(sampled_at);
`

const Retention = 48 * time.Hour

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
	s := &Store{db: d}
	// Preserve the existing latest reading when upgrading from the cache-only DB.
	if _, err := d.Exec(`INSERT OR IGNORE INTO samples (name, sampled_at, payload)
		SELECT name, updated_at, payload FROM usage WHERE updated_at >= ?`, time.Now().Add(-Retention).UnixMilli()); err != nil {
		d.Close()
		return nil, err
	}
	if err := s.Prune(time.Now()); err != nil {
		d.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying connection.
func (s *Store) Close() error { return s.db.Close() }

// Upsert atomically saves the latest result and a historical sample, including failures.
func (s *Store) Upsert(name string, r types.UsageResult, updatedAt int64) error {
	payload, err := json.Marshal(r)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(
		`INSERT INTO usage (name, payload, status, queried_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   payload = excluded.payload,
		   status = excluded.status,
		   queried_at = excluded.queried_at,
		   updated_at = excluded.updated_at`,
		name, string(payload), string(r.Status), r.QueriedAt, updatedAt,
	)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO samples (name, sampled_at, payload) VALUES (?, ?, ?)
		ON CONFLICT(name, sampled_at) DO UPDATE SET payload = excluded.payload`, name, updatedAt, string(payload)); err != nil {
		return err
	}
	return tx.Commit()
}

// Prune removes readings outside the rolling retention window, even if all endpoints fail.
func (s *Store) Prune(now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cutoff := now.Add(-Retention).UnixMilli()
	for _, table := range []string{"samples", "usage"} {
		column := "sampled_at"
		if table == "usage" {
			column = "updated_at"
		}
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE "+column+" < ?", cutoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// History returns all retained samples grouped by endpoint, oldest first.
func (s *Store) History(since int64) (map[string][]types.UsageRecord, error) {
	rows, err := s.db.Query(`SELECT name, payload, sampled_at FROM samples WHERE sampled_at >= ? ORDER BY sampled_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]types.UsageRecord)
	for rows.Next() {
		var r types.UsageRecord
		var payload string
		if err := rows.Scan(&r.Name, &payload, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(payload), &r.UsageResult); err != nil {
			return nil, err
		}
		out[r.Name] = append(out[r.Name], r)
	}
	return out, rows.Err()
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
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	condition := ""
	args := make([]any, len(keep))
	if len(keep) > 0 {
		placeholders := make([]string, len(keep))
		for i, name := range keep {
			placeholders[i] = "?"
			args[i] = name
		}
		condition = " WHERE name NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	if _, err := tx.Exec("DELETE FROM samples"+condition, args...); err != nil {
		return 0, err
	}
	res, err := tx.Exec("DELETE FROM usage"+condition, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}
