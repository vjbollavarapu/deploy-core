package crypto_test

import (
	"bytes"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	key, err := crypto.NormalizePlatformKey([]byte("deploycore-test-platform-key-material"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := crypto.Seal(key, "platform:v1", []byte("super-secret-value"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if env.KeyID != "platform:v1" || len(env.Nonce) == 0 || len(env.Ciphertext) == 0 {
		t.Fatalf("bad envelope: %#v", env)
	}
	plain, err := crypto.Open(key, env)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(plain, []byte("super-secret-value")) {
		t.Fatalf("plain=%q", plain)
	}

	// Tamper detection.
	env.Ciphertext[len(env.Ciphertext)-1] ^= 0xff
	if _, err := crypto.Open(key, env); err == nil {
		t.Fatal("expected tamper failure")
	}
}

func TestEnvelopeRejectsWrongKey(t *testing.T) {
	keyA, err := crypto.NormalizePlatformKey([]byte("deploycore-test-platform-key-material-a"))
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := crypto.NormalizePlatformKey([]byte("deploycore-test-platform-key-material-b"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := crypto.Seal(keyA, "platform:v1", []byte("classified"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypto.Open(keyB, env); err == nil {
		t.Fatal("expected wrong-key failure")
	}
}

func TestNormalizePlatformKeyExact32(t *testing.T) {
	raw := bytes.Repeat([]byte{0xab}, 32)
	got, err := crypto.NormalizePlatformKey(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("expected exact copy")
	}
}
