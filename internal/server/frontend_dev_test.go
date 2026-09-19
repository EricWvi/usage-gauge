//go:build !production

package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestDevelopmentDoesNotServeFrontend(t *testing.T) {
	s, err := New(nil, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/assets/index.js"} {
		w := httptest.NewRecorder()
		s.Routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("development route %s: got %d, want 404", path, w.Code)
		}
	}
}
