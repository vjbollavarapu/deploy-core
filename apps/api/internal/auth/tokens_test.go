package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccessTokenRoundTrip(t *testing.T) {
	t.Parallel()

	secret := []byte("0123456789abcdef0123456789abcdef")
	userID := uuid.New()
	sessionID := uuid.New()
	now := time.Now().UTC()

	tok, exp, err := issueAccessToken(secret, userID, sessionID, time.Minute, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(now) {
		t.Fatal("expected future expiry")
	}
	gotUser, gotSession, err := parseAccessToken(secret, tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gotUser != userID || gotSession != sessionID {
		t.Fatalf("got %s/%s", gotUser, gotSession)
	}
	if _, _, err := parseAccessToken([]byte("different-secret-0123456789abcdef"), tok); err == nil {
		t.Fatal("expected signature failure")
	}
}
