// Package refresh periodically fetches every endpoint, parses each response,
// and persists the result. Failures are recorded as error results and are not
// retried; each endpoint is isolated from the others.
package refresh

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"usage-gauge/internal/config"
	"usage-gauge/internal/db"
	"usage-gauge/internal/fetch"
	"usage-gauge/internal/parser"
	"usage-gauge/internal/types"
)

// DefaultInterval is used when REFRESH_INTERVAL_MS is unset or invalid.
const DefaultInterval = 5 * time.Minute

// Refresher coordinates background refreshes.
type Refresher struct {
	store   *db.Store
	engine  *parser.Engine
	running atomic.Bool
}

// New creates a Refresher backed by the given store and parser engine.
func New(store *db.Store, engine *parser.Engine) *Refresher {
	return &Refresher{store: store, engine: engine}
}

// Start runs refreshAll once immediately and then every interval, in a
// background goroutine, until ctx is done. It returns without blocking.
func (r *Refresher) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	go func() {
		r.refreshAll(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.refreshAll(ctx)
			}
		}
	}()
	log.Printf("[usage-gauge] background refresh started (interval=%s)", interval)
}

// refreshAll fetches every configured endpoint concurrently. Each endpoint is
// isolated and nothing is retried; if any endpoint succeeds the global
// last-success timestamp is advanced.
func (r *Refresher) refreshAll(ctx context.Context) {
	// Guard against overlapping rounds (a slow endpoint vs a short interval).
	if !r.running.CompareAndSwap(false, true) {
		return
	}
	defer r.running.Store(false)

	eps, err := config.LoadEndpoints()
	if err != nil {
		log.Printf("[usage-gauge] load endpoints: %v", err)
		return
	}
	if len(eps) == 0 {
		return
	}

	now := time.Now().UnixMilli()
	var wg sync.WaitGroup
	var okCount int64

	for i := range eps {
		ep := eps[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.refreshOne(ctx, ep, now) {
				atomic.AddInt64(&okCount, 1)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&okCount) > 0 {
		if err := r.store.MarkLastSuccess(now); err != nil {
			log.Printf("[usage-gauge] mark last success: %v", err)
		}
	}
}

// refreshOne fetches + parses + stores one endpoint. Returns true if the
// resulting status was OK.
func (r *Refresher) refreshOne(ctx context.Context, ep types.EndpointConfig, now int64) bool {
	res, err := fetch.Endpoint(ctx, ep)
	if err != nil {
		r.recordError(ep.Name, now, "Network error: "+err.Error())
		return false
	}

	result, err := r.engine.Parse(ep.ParserName(), res.Body, types.ParseContext{
		HTTPStatus: res.Status,
		RawBody:    res.Raw,
		Endpoint:   ep.Public(),
	})
	if err != nil {
		r.recordError(ep.Name, now, err.Error())
		return false
	}

	if err := r.store.Upsert(ep.Name, result, now); err != nil {
		log.Printf("[usage-gauge] upsert %s: %v", ep.Name, err)
		return false
	}
	return result.Status == types.StatusOK
}

func (r *Refresher) recordError(name string, now int64, msg string) {
	result := types.UsageResult{
		Status:    types.StatusError,
		Tiers:     []types.UsageTier{},
		Error:     msg,
		QueriedAt: now,
	}
	if err := r.store.Upsert(name, result, now); err != nil {
		log.Printf("[usage-gauge] upsert (error) %s: %v", name, err)
	}
}
