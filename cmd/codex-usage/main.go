// Command codex-usage exposes local Codex subscription limits over HTTP.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("listen", "0.0.0.0:55666", "HTTP listen address")
	binary := flag.String("codex", "codex", "Codex executable")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout for each query, including queueing")
	flag.Parse()
	if *timeout <= 0 {
		log.Fatal("timeout must be positive")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *addr, *binary, *timeout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, binary string, timeout time.Duration) error {
	dir, err := os.MkdirTemp("", "usage-gauge-codex-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	c := newCodex(binary, dir)
	defer c.close()
	initCtx, cancel := context.WithTimeout(ctx, timeout)
	c.process, err = startCodex(initCtx, binary, dir)
	cancel()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           routes(c, timeout),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	log.Printf("[codex-usage] listening on %s (codex cwd: %s)", listener.Addr(), dir)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		return nil
	}
}

func routes(c *codex, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := c.read(ctx)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	return mux
}
