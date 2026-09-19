package auth

import (
	"sync"
	"time"
)

// RateLimiter is a simple in-process sliding-window limiter for auth endpoints.
type RateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	limit    int
	attempts map[string][]time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 30
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		window:   window,
		limit:    limit,
		attempts: make(map[string][]time.Time),
	}
}

// Allow reports whether key may proceed and records the attempt when allowed.
func (l *RateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	events := l.attempts[key]
	kept := events[:0]
	for _, ts := range events {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.limit {
		l.attempts[key] = kept
		return false
	}
	l.attempts[key] = append(kept, now)
	return true
}
