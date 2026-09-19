package db

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"usage-gauge/internal/types"
)

func TestHistoryRetentionAndFailures(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "gauge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	cutoff := now.Add(-Retention).UnixMilli()
	ok := types.UsageResult{Status: types.StatusOK, Tiers: []types.UsageTier{{Name: "weekly", Utilization: 42}}}
	failed := types.UsageResult{Status: types.StatusError, Error: "offline", Tiers: []types.UsageTier{}}
	for _, sample := range []struct {
		name   string
		at     int64
		result types.UsageResult
	}{
		{"codex", cutoff - 1, ok}, {"codex", cutoff, ok}, {"codex", now.Add(-time.Minute).UnixMilli(), failed}, {"codex", now.UnixMilli(), ok}, {"removed", now.UnixMilli(), ok},
	} {
		if err := s.Upsert(sample.name, sample.result, sample.at); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prune(now); err != nil {
		t.Fatal(err)
	}
	history, err := s.History(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history["codex"]) != 3 || history["codex"][0].UpdatedAt != cutoff || history["codex"][1].Status != types.StatusError {
		t.Fatalf("bad retained history: %+v", history)
	}
	if _, err := s.DeleteNotIn([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	history, _ = s.History(0)
	if len(history["removed"]) != 0 {
		t.Fatal("removed endpoint history retained")
	}
	latest, _ := s.All()
	if len(latest) != 1 || latest[0].UpdatedAt != now.UnixMilli() {
		t.Fatalf("latest: %+v", latest)
	}
	if err := s.Prune(now.Add(Retention + time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	history, _ = s.History(0)
	latest, _ = s.All()
	if len(history) != 0 || len(latest) != 0 {
		t.Fatal("expired rows remain")
	}
}

func TestUpgradePreservesLatestWithoutDuplicating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gauge.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`CREATE TABLE usage (name TEXT PRIMARY KEY, payload TEXT NOT NULL, status TEXT NOT NULL, queried_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(types.UsageResult{Status: types.StatusOK, Tiers: []types.UsageTier{{Name: "five_hour", Utilization: 12}}})
	now := time.Now().UnixMilli()
	if _, err := raw.Exec(`INSERT INTO usage VALUES (?, ?, ?, ?, ?)`, "zai", string(payload), "ok", now, now); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	for range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		history, err := s.History(0)
		if err != nil {
			t.Fatal(err)
		}
		if len(history["zai"]) != 1 || history["zai"][0].Tiers[0].Utilization != 12 {
			t.Fatalf("migration: %+v", history)
		}
		s.Close()
	}
}
