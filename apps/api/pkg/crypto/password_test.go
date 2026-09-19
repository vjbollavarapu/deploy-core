package crypto_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := crypto.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := crypto.VerifyPassword(hash, "correct-horse-battery")
	if err != nil || !ok {
		t.Fatalf("verify good: ok=%v err=%v", ok, err)
	}
	ok, err = crypto.VerifyPassword(hash, "wrong-password")
	if err != nil || ok {
		t.Fatalf("verify bad: ok=%v err=%v", ok, err)
	}
}

func TestTokenHashStable(t *testing.T) {
	t.Parallel()

	token, err := crypto.RandomURLToken(32)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	a := crypto.HashTokenSHA256(token)
	b := crypto.HashTokenSHA256(token)
	if a != b || a == "" {
		t.Fatalf("hash mismatch %s vs %s", a, b)
	}
	if crypto.HashTokenSHA256(token+"x") == a {
		t.Fatal("expected different hash")
	}
}
