package registration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScrubRegistrationEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registration.env")
	if err := os.WriteFile(path, []byte("AGENT_REGISTRATION_TOKEN=secret\n"), 0640); err != nil {
		t.Fatal(err)
	}

	prev := registrationEnvPath
	registrationEnvPath = path
	t.Cleanup(func() { registrationEnvPath = prev })

	scrubRegistrationEnvFile()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected registration env file removed, stat err=%v", err)
	}

	// Idempotent when already absent
	scrubRegistrationEnvFile()
}
