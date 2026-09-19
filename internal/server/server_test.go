package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"usage-gauge/internal/db"
	"usage-gauge/internal/types"
)

func TestDashboardData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_DIR", dir)
	config := "endpoints:\n  - name: codex\n    url: http://private-host\n    headers:\n      Authorization: private-secret\n  - name: zai\n    url: http://another-private-host\n"
	if err := os.WriteFile(filepath.Join(dir, "endpoints.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(filepath.Join(dir, "gauge.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UnixMilli()
	for _, at := range []int64{now - int64(49*time.Hour/time.Millisecond), now} {
		if err := store.Upsert("codex", types.UsageResult{Status: types.StatusOK, Tiers: []types.UsageTier{{Name: "weekly", Utilization: 50}}}, at); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(store, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/usage", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var data apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Endpoints) != 2 || len(data.Endpoints[0].History) != 1 || data.Endpoints[1].Latest != nil || data.Endpoints[1].History == nil {
		t.Fatalf("data: %+v", data)
	}
	if strings.Contains(w.Body.String(), "private-") {
		t.Fatal("private configuration exposed")
	}
}
