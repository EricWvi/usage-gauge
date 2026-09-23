package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeCredentials(t *testing.T, path, token string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"claudeAiOauth": map[string]string{"accessToken": token}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".new", data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".new", path); err != nil {
		t.Fatal(err)
	}
}

func query(handler http.Handler) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/usage", nil))
	return w
}

func TestFreshTokenAndUsageEveryRequest(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/oauth/usage" || r.Header.Get("Authorization") != fmt.Sprintf("Bearer token-%d", n) || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			t.Error("unexpected upstream request or stale token")
		}
		fmt.Fprintf(w, `{"five_hour":{"utilization":%d,"resets_at":"2026-09-23T12:00:00Z"}}`, n)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), ".credentials.json")
	c := newClaude(path)
	c.url = upstream.URL + "/api/oauth/usage"
	handler := routes(c, time.Second)
	for n := 1; n <= 3; n++ {
		writeCredentials(t, path, fmt.Sprintf("token-%d", n))
		w := query(handler)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), fmt.Sprintf(`"utilization":%d`, n)) {
			t.Fatalf("query %d: %d %s", n, w.Code, w.Body)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("expected one upstream call per query")
	}
	// Removing credentials must not fall back to the previously successful token.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if w := query(handler); w.Code != 502 || calls.Load() != 3 {
		t.Fatal("reused credentials after removal")
	}
}

func TestUpstreamFailures(t *testing.T) {
	for _, tc := range []struct {
		name           string
		upstream, want int
		body           string
	}{
		{"unauthorized", 401, 401, "private-token"},
		{"forbidden", 403, 403, "private-token"},
		{"limited", 429, 429, "private-token"},
		{"server", 500, 502, "private-token"},
		{"redirect", 302, 502, "private-token"},
		{"invalid", 200, 502, "private-token"},
		{"null", 200, 502, "null"},
		{"array", 200, 502, "[]"},
		{"oversized", 200, 502, `{"value":"` + strings.Repeat("x", 1<<20) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "/redirected")
				w.Header().Set("Retry-After", "120")
				w.WriteHeader(tc.upstream)
				fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "credentials")
			writeCredentials(t, path, "private-token")
			c := newClaude(path)
			c.url = upstream.URL
			w := query(routes(c, time.Second))
			if w.Code != tc.want || !json.Valid(w.Body.Bytes()) || strings.Contains(w.Body.String(), "private-token") || calls.Load() != 1 {
				t.Fatalf("unexpected response: %d %s; calls %d", w.Code, w.Body, calls.Load())
			}
			if tc.upstream == 429 && w.Header().Get("Retry-After") != "120" {
				t.Fatal("missing retry-after")
			}
		})
	}
}

func TestInvalidCredentialsAndRecovery(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"five_hour":null,"seven_day":null}`)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "credentials")
	c := newClaude(path)
	c.url = upstream.URL
	handler := routes(c, time.Second)
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`{"claudeAiOauth":`, 502},
		{`{"mcpOAuth":{"accessToken":"private-token"}}`, 401},
		{`{"claudeAiOauth":{"accessToken":"\r\nprivate-token"}}`, 200},
		{`{"claudeAiOauth":{"accessToken":"private\r\ntoken"}}`, 401},
	} {
		if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		w := query(handler)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private-token") {
			t.Fatalf("credentials error: %d %s", w.Code, w.Body)
		}
	}
	writeCredentials(t, path, "valid-token")
	if w := query(handler); w.Code != 200 || calls.Load() != 2 {
		t.Fatalf("recovery failed: %d %s", w.Code, w.Body)
	}
}

func TestTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "credentials")
	writeCredentials(t, path, "token")
	c := newClaude(path)
	c.url = upstream.URL
	if w := query(routes(c, 50*time.Millisecond)); w.Code != 504 {
		t.Fatalf("timeout: %d %s", w.Code, w.Body)
	}
}
