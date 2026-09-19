package auth_test

import (
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
)

func TestRateLimiter(t *testing.T) {
	t.Parallel()

	l := auth.NewRateLimiter(2, time.Minute)
	now := time.Now().UTC()
	if !l.Allow("a", now) || !l.Allow("a", now.Add(time.Second)) {
		t.Fatal("expected first two allows")
	}
	if l.Allow("a", now.Add(2*time.Second)) {
		t.Fatal("expected rate limit")
	}
	if !l.Allow("b", now) {
		t.Fatal("other key should be independent")
	}
}
