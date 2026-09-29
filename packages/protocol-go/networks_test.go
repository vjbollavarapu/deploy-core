package protocol

import "testing"

func TestFormatPrivateNetworkName(t *testing.T) {
	got, err := FormatPrivateNetworkName("modulyn", "production")
	if err != nil {
		t.Fatal(err)
	}
	if got != "dc-modulyn-production-private" {
		t.Fatalf("name = %s", got)
	}
	if _, err := FormatPrivateNetworkName("Modulyn", "production"); err == nil {
		t.Fatal("uppercase project slug accepted")
	}
}

func TestValidDNSAlias(t *testing.T) {
	if !ValidDNSAlias("redis") {
		t.Fatal("redis should be a valid alias")
	}
	for _, bad := range []string{"", "redis.internal", "Redis", "redis/cache", "has space"} {
		if ValidDNSAlias(bad) {
			t.Fatalf("accepted %q", bad)
		}
	}
}
