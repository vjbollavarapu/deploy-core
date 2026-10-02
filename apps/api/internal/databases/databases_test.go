package databases

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/projects"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestSlugifyIdent(t *testing.T) {
	cases := map[string]string{
		"My DB":    "my_db",
		"123start": "db_123start",
		"":         "db",
		"hello!!!": "hello",
		"a":        "a",
	}
	for in, want := range cases {
		if got := slugifyIdent(in); got != want {
			t.Fatalf("slugifyIdent(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDNSAliasUsesProjectSlugRules(t *testing.T) {
	got, err := DNSAlias("Primary PG")
	if err != nil {
		t.Fatal(err)
	}
	if want := "db-" + projects.Slugify("Primary PG"); got != want || got != "db-primary-pg" {
		t.Fatalf("alias=%q want db-primary-pg (%s)", got, want)
	}
	if _, err := DNSAlias("!!!"); err == nil {
		t.Fatal("expected a name with no letters or digits to fail")
	}
}

func TestPrivateNetworkNameUsesProtocolHelper(t *testing.T) {
	got, err := protocol.FormatPrivateNetworkName("modulyn", "production")
	if err != nil {
		t.Fatal(err)
	}
	if got != "dc-modulyn-production-private" {
		t.Fatalf("network=%s", got)
	}
}
