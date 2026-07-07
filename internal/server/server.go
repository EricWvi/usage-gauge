package server

import (
	"encoding/json"
	"io/fs"
	"net/http"

	"usage-gauge/internal/db"
	"usage-gauge/internal/ui"
)

// Server holds shared dependencies for the HTTP handlers.
type Server struct {
	store    *db.Store
	renderer *Renderer
}

// New creates a Server with a freshly parsed renderer.
func New(store *db.Store) (*Server, error) {
	r, err := NewRenderer()
	if err != nil {
		return nil, err
	}
	return &Server{store: store, renderer: r}, nil
}

// Routes returns the HTTP mux with the app routes registered.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/usage", s.handleAPI)

	staticSub, err := fs.Sub(ui.Files, "static")
	if err != nil {
		// Should never happen: static/ is embedded at build time.
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	return mux
}

func (s *Server) pageData() (PageData, error) {
	records, err := s.store.All()
	if err != nil {
		return PageData{}, err
	}
	last, err := s.store.LastSuccessAt()
	if err != nil {
		return PageData{}, err
	}
	return PageData{
		LastSuccessAt:   last,
		LastUpdatedText: lastUpdatedText(last),
		Records:         records,
	}, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := s.pageData()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out, err := s.renderer.RenderPage(data)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(out))
}

// apiResponse is what /api/usage returns for client polling.
type apiResponse struct {
	LastUpdatedAt   int64  `json:"lastUpdatedAt"`
	LastUpdatedText string `json:"lastUpdatedText"`
	HTML            string `json:"html"`
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	data, err := s.pageData()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	html, err := s.renderer.RenderCards(data)
	if err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	resp := apiResponse{
		LastUpdatedAt:   data.LastSuccessAt,
		LastUpdatedText: data.LastUpdatedText,
		HTML:            html,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}
