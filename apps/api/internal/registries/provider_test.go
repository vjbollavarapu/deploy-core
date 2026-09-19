package registries_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/registries"
)

func TestProviderNormalizeDefaults(t *testing.T) {
	ghcr, err := registries.Lookup("ghcr")
	if err != nil {
		t.Fatal(err)
	}
	url, err := ghcr.NormalizeURL("")
	if err != nil || url != "ghcr.io" {
		t.Fatalf("ghcr default=%s err=%v", url, err)
	}

	hub, _ := registries.Lookup("dockerhub")
	url, err = hub.NormalizeURL("https://index.docker.io/v1/")
	if err != nil || url != "index.docker.io/v1" {
		t.Fatalf("dockerhub url=%s err=%v", url, err)
	}

	oci, _ := registries.Lookup("oci")
	if _, err := oci.NormalizeURL(""); err == nil {
		t.Fatal("oci requires url")
	}
	url, err = oci.NormalizeURL("registry.example.com:5000")
	if err != nil || url != "registry.example.com:5000" {
		t.Fatalf("oci url=%s err=%v", url, err)
	}
}

func TestReservedProvidersExist(t *testing.T) {
	for _, p := range []string{"gcp", "ecr", "acr"} {
		if _, err := registries.Lookup(p); err != nil {
			t.Fatalf("lookup %s: %v", p, err)
		}
		if registries.SupportedInitially(p) {
			t.Fatalf("%s should not be initially enabled", p)
		}
	}
	if !registries.SupportedInitially("ghcr") {
		t.Fatal("ghcr should be supported")
	}
}
