package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func fakeCodex(t *testing.T, query string) (*codex, string) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "codex")
	script := `#!/bin/sh
test "$1" = app-server || exit 1
pwd > observed-cwd
id=0
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialized"'*) continue ;;
    *'"method":"initialize"'*)
      id=$((id+1))
      printf '{"id":%s,"result":{}}\n' "$id"
      ;;
    *'"method":"account/rateLimits/read"'*)
      id=$((id+1))
` + query + `
      ;;
    *) exit 2 ;;
  esac
done
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := newCodex(binary, dir)
	t.Cleanup(c.close)
	return c, dir
}

func TestLiveQueriesAndConcurrentRequests(t *testing.T) {
	c, dir := fakeCodex(t, `printf '{"method":"account/rateLimits/updated","params":{}}\n'
printf '{"id":%s,"result":{"rateLimits":{"primary":{"usedPercent":%s,"windowDurationMins":10080,"resetsAt":1800000000}}}}\n' "$id" "$id"`)
	handler := routes(c, 5*time.Second)
	var wg sync.WaitGroup
	values := make(chan int, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/usage", nil))
			if w.Code != http.StatusOK {
				t.Errorf("status %d: %s", w.Code, w.Body)
				return
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Error("response must not be cached")
			}
			var result struct {
				RateLimits struct{ Primary struct{ UsedPercent int } }
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Error(err)
				return
			}
			values <- result.RateLimits.Primary.UsedPercent
		}()
	}
	wg.Wait()
	close(values)
	seen := map[int]bool{}
	for value := range values {
		seen[value] = true
	}
	if len(seen) != 8 {
		t.Fatalf("wanted eight fresh responses, got %v", seen)
	}
	observed, err := os.ReadFile(filepath.Join(dir, "observed-cwd"))
	if err != nil || strings.TrimSpace(string(observed)) != dir {
		t.Fatalf("cwd %q: %v", observed, err)
	}
}

func TestQueryFailuresAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		status      int
	}{
		{"rpc", `printf '{"id":%s,"error":{"code":-1,"message":"login required"}}\n' "$id"`, 502},
		{"exit", `exit 1`, 502},
		{"timeout", `while IFS= read -r ignored; do :; done`, 504},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := fakeCodex(t, tc.query)
			handler := routes(c, 100*time.Millisecond)
			for range 2 {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage", nil))
				if w.Code != tc.status || !json.Valid(w.Body.Bytes()) {
					t.Fatalf("status %d: %s", w.Code, w.Body)
				}
				if c.process != nil {
					t.Fatal("failed process was not discarded")
				}
			}
			// The next request starts a new process and can recover.
			good, _ := fakeCodex(t, `printf '{"id":%s,"result":{"rateLimits":{}}}\n' "$id"`)
			c.binary = good.binary
			w := httptest.NewRecorder()
			routes(c, time.Second).ServeHTTP(w, httptest.NewRequest("GET", "/api/usage", nil))
			if w.Code != 200 {
				t.Fatal(fmt.Sprintf("recovery: %d %s", w.Code, w.Body))
			}
		})
	}
}
