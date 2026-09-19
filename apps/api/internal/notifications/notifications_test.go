package notifications

import (
	"testing"

	"github.com/google/uuid"
)

func TestSanitizePayloadStripsSecrets(t *testing.T) {
	in := map[string]any{
		"backupId":     "b1",
		"password":     "secret",
		"accessToken":  "tok",
		"dbCredential": "x",
		"normal":       "ok",
	}
	out := sanitizePayload(in)
	if _, ok := out["password"]; ok {
		t.Fatal("password should be stripped")
	}
	if _, ok := out["accessToken"]; ok {
		t.Fatal("token should be stripped")
	}
	if _, ok := out["dbCredential"]; ok {
		t.Fatal("credential should be stripped")
	}
	if out["normal"] != "ok" || out["backupId"] != "b1" {
		t.Fatalf("got %#v", out)
	}
}

func TestPolicyMatchesFilters(t *testing.T) {
	appA := uuid.New()
	appB := uuid.New()
	env := uuid.New()
	p := Policy{
		ResourceFilters: map[string]any{
			"applicationIds": []any{appA.String()},
		},
		EnvironmentFilters: map[string]any{
			"environmentIds": []any{env.String()},
		},
	}
	if policyMatches(p, EmitInput{ApplicationID: &appB, EnvironmentID: &env}) {
		t.Fatal("should reject other application")
	}
	if !policyMatches(p, EmitInput{ApplicationID: &appA, EnvironmentID: &env}) {
		t.Fatal("should accept matching filters")
	}
}

func TestEnabledAndReservedChannelTypes(t *testing.T) {
	enabled := EnabledChannelTypes()
	if len(enabled) != 2 || enabled[0] != ChannelEmail {
		t.Fatalf("enabled=%v", enabled)
	}
	reserved := ReservedChannelTypes()
	if len(reserved) < 5 {
		t.Fatalf("reserved=%v", reserved)
	}
}
