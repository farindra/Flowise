package ratelimit

import (
	"sync"
	"time"
)

const window = time.Minute

// Limiter is a per-key sliding-window rate limiter (port of
// AIService.isRateLimited/addRateLimitCall).
type Limiter struct {
	mu    sync.Mutex
	calls map[string][]time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{calls: make(map[string][]time.Time)}
}

// IsRateLimited returns true if key already has maxRequests calls within the
// last minute. It also prunes expired timestamps for key.
func (l *Limiter) IsRateLimited(key string, maxRequests int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	valid := pruneExpired(l.calls[key], now)
	l.calls[key] = valid

	return len(valid) >= maxRequests
}

// AddCall records a call for key at the current time.
func (l *Limiter) AddCall(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls[key] = append(l.calls[key], time.Now())
}

func pruneExpired(timestamps []time.Time, now time.Time) []time.Time {
	valid := timestamps[:0:0]
	for _, ts := range timestamps {
		if now.Sub(ts) < window {
			valid = append(valid, ts)
		}
	}
	return valid
}

// Cache is a simple in-memory TTL cache (port of the database.setCache/
// getCache/isCacheValid helpers used for AI result caching).
type Cache struct {
	mu    sync.Mutex
	items map[string]cacheItem
}

type cacheItem struct {
	value     any
	expiresAt time.Time
}

func NewCache() *Cache {
	return &Cache{items: make(map[string]cacheItem)}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = cacheItem{value: value, expiresAt: time.Now().Add(ttl)}
}
