// Command usage-gauge runs the self-hosted usage dashboard server.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"usage-gauge/internal/config"
	"usage-gauge/internal/db"
	"usage-gauge/internal/parser"
	"usage-gauge/internal/refresh"
	"usage-gauge/internal/server"
)

func main() {
	store, err := db.Open(config.DBPath())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	cleanupStale(store)

	engine := parser.New()
	refresher := refresh.New(store, engine)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	refresher.Start(ctx, refreshInterval())

	srv, err := server.New(store)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:    ":" + port(),
		Handler: srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Printf("[usage-gauge] shutting down...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("[usage-gauge] listening on %s (config dir: %s)", httpSrv.Addr, config.Dir())
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func refreshInterval() time.Duration {
	if v := os.Getenv("REFRESH_INTERVAL_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return refresh.DefaultInterval
}

func port() string {
	if v := os.Getenv("PORT"); v != "" {
		return v
	}
	return "3000"
}

func cleanupStale(store *db.Store) {
	eps, err := config.LoadEndpoints()
	if err != nil {
		log.Printf("[usage-gauge] skip cleanup, load endpoints: %v", err)
		return
	}
	names := make([]string, len(eps))
	for i := range eps {
		names[i] = eps[i].Name
	}
	n, err := store.DeleteNotIn(names)
	if err != nil {
		log.Printf("[usage-gauge] cleanup stale records: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[usage-gauge] removed %d stale record(s)", n)
	}
}
