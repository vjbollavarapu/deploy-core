package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSanitizePayloadStripsSecrets(t *testing.T) {
	out := sanitizePayload(map[string]any{
		"deploymentId": "d1",
		"password":     "x",
		"accessToken":  "t",
		"ok":           true,
	})
	if _, ok := out["password"]; ok {
		t.Fatal("password leaked")
	}
	if _, ok := out["accessToken"]; ok {
		t.Fatal("token leaked")
	}
	if out["ok"] != true || out["deploymentId"] != "d1" {
		t.Fatalf("%#v", out)
	}
}

func TestSignBody(t *testing.T) {
	secret := []byte("super-secret")
	body := []byte(`{"event":"backup.failed"}`)
	sig := signBody(secret, body)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if sig != want {
		t.Fatalf("sig=%s want=%s", sig, want)
	}
}

func TestKnownEvents(t *testing.T) {
	if len(KnownEvents()) != 6 {
		t.Fatalf("events=%v", KnownEvents())
	}
}
