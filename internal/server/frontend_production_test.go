//go:build production

package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProductionServesEmbeddedFrontend(t *testing.T) {
	s, err := New(nil, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Routes().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="root"`) || !strings.Contains(w.Body.String(), "/assets/") {
		t.Fatal("embedded dashboard not served")
	}
}
