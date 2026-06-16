package jurnal

import (
	"context"
	"sync"
	"time"
)

// rateLimiter paces requests to roughly perMinute requests per minute by
// spacing each call out by 60s/perMinute, mirroring the effect of
// new RateLimiter(20) used for Jurnal.id profile fetches.
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func newRateLimiter(perMinute int) *rateLimiter {
	return &rateLimiter{interval: time.Minute / time.Duration(perMinute)}
}

func (r *rateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	earliest := r.last.Add(r.interval)
	if now.Before(earliest) {
		select {
		case <-time.After(earliest.Sub(now)):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	r.last = time.Now()
	return nil
}
