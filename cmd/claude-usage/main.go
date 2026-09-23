// Command claude-usage exposes Claude subscription limits using local OAuth credentials.
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
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("listen", "0.0.0.0:55667", "HTTP listen address")
	credentials := flag.String("credentials", "", "Claude credentials JSON file (default: $CLAUDE_CONFIG_DIR/.credentials.json or ~/.claude/.credentials.json)")
	timeout := flag.Duration("timeout", 30*time.Second, "timeout for each upstream query")
	flag.Parse()
	if *timeout <= 0 {
		log.Fatal("timeout must be positive")
	}
	if *credentials == "" {
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Fatal("cannot determine home directory; pass -credentials")
			}
			dir = filepath.Join(home, ".claude")
		}
		*credentials = filepath.Join(dir, ".credentials.json")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *addr, *credentials, *timeout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, credentials string, timeout time.Duration) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	c := newClaude(credentials)
	srv := &http.Server{
		Handler:           routes(c, timeout),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	log.Printf("[claude-usage] listening on %s", listener.Addr())
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

func routes(c *claude, timeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := c.read(ctx)
		if err != nil {
			status := http.StatusBadGateway
			var failure *queryError
			if errors.As(err, &failure) {
				status = failure.status
				if failure.retryAfter != "" {
					w.Header().Set("Retry-After", failure.retryAfter)
				}
			}
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
