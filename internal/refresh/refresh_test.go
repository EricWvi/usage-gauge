package refresh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"usage-gauge/internal/config"
	"usage-gauge/internal/db"
	"usage-gauge/internal/parser"
)

func TestURLOnlyCodexSampling(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("expected unauthenticated GET")
		}
		if calls == 2 {
			http.Error(w, "offline", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"rateLimits":{"primary":{"usedPercent":25,"windowDurationMins":300},"secondary":{"usedPercent":60,"windowDurationMins":10080}}}`))
	}))
	defer upstream.Close()
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "endpoints.yaml"), []byte("endpoints:\n  - name: codex\n    url: "+upstream.URL+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	eps, err := config.LoadEndpoints()
	if err != nil {
		t.Fatal(err)
	}
	s, err := db.Open(filepath.Join(dir, "gauge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := New(s, parser.New())
	now := time.Now().UnixMilli()
	if !r.refreshOne(context.Background(), eps[0], now) {
		t.Fatal("first sample failed")
	}
	if r.refreshOne(context.Background(), eps[0], now+1) {
		t.Fatal("failed sample marked OK")
	}
	history, err := s.History(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history["codex"]) != 2 || history["codex"][0].Tiers[1].Utilization != 60 || len(history["codex"][1].Tiers) != 0 {
		t.Fatalf("history: %+v", history)
	}
}
