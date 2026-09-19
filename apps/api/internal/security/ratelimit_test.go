package security

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(2, time.Minute)
	now := time.Now()
	if !l.Allow("k", now) || !l.Allow("k", now) {
		t.Fatal("expected allow")
	}
	if l.Allow("k", now) {
		t.Fatal("expected rate limit")
	}
}
