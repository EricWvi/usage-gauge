// Package server serves the embedded dashboard and its sampled usage data.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"usage-gauge/internal/config"
	"usage-gauge/internal/db"
	"usage-gauge/internal/types"
	"usage-gauge/web"
)

type Server struct {
	store    *db.Store
	assets   http.Handler
	interval time.Duration
}

func New(store *db.Store, interval time.Duration) (*Server, error) {
	assets, err := web.Handler()
	if err != nil {
		return nil, err
	}
	return &Server{store: store, assets: assets, interval: interval}, nil
}

func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/usage", s.handleAPI)
	if s.assets != nil {
		mux.Handle("GET /", s.assets)
	}
	return mux
}

type endpointData struct {
	Name     string              `json:"name"`
	Provider string              `json:"provider"`
	Latest   *types.UsageRecord  `json:"latest"`
	History  []types.UsageRecord `json:"history"`
}

type apiResponse struct {
	LastUpdatedAt    int64          `json:"lastUpdatedAt"`
	ServerTime       int64          `json:"serverTime"`
	RetentionHours   int            `json:"retentionHours"`
	SampleIntervalMs int64          `json:"sampleIntervalMs"`
	Endpoints        []endpointData `json:"endpoints"`
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	data, err := s.data()
	if err != nil {
		log.Printf("[usage-gauge] load dashboard: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Unable to load usage data. Check the server configuration and logs."})
		return
	}
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) data() (apiResponse, error) {
	now := time.Now()
	out := apiResponse{ServerTime: now.UnixMilli(), RetentionHours: 48, SampleIntervalMs: s.interval.Milliseconds(), Endpoints: []endpointData{}}
	eps, err := config.LoadEndpoints()
	if err != nil {
		return out, err
	}
	history, err := s.store.History(now.Add(-db.Retention).UnixMilli())
	if err != nil {
		return out, err
	}
	out.LastUpdatedAt, err = s.store.LastSuccessAt()
	if err != nil {
		return out, err
	}
	for _, ep := range eps {
		samples := history[ep.Name]
		if samples == nil {
			samples = []types.UsageRecord{}
		}
		entry := endpointData{Name: ep.Name, Provider: ep.ParserName(), History: samples}
		if len(samples) > 0 {
			entry.Latest = &samples[len(samples)-1]
		}
		out.Endpoints = append(out.Endpoints, entry)
	}
	return out, nil
}
