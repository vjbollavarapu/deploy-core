package docker

import "testing"

func TestEndpointSettingsForNetworks_AliasOnlyOnNamedNetwork(t *testing.T) {
	mode, endpoints := endpointSettingsForNetworks(
		[]string{"dc-modulyn-production-private", "deploycore-proxy"},
		map[string][]string{"dc-modulyn-production-private": {"redis"}},
	)
	if mode != "dc-modulyn-production-private" {
		t.Fatalf("network mode = %s", mode)
	}
	private := endpoints["dc-modulyn-production-private"]
	if private == nil || len(private.Aliases) != 1 || private.Aliases[0] != "redis" {
		t.Fatalf("private endpoint = %#v", private)
	}
	proxy := endpoints["deploycore-proxy"]
	if proxy == nil {
		t.Fatal("proxy endpoint missing")
	}
	if len(proxy.Aliases) != 0 {
		t.Fatalf("proxy aliases = %#v", proxy.Aliases)
	}
}
