package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/agent/internal/database"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type fakeNetworkEnsurer struct {
	calls int
	name  string
	err   error
}

func (f *fakeNetworkEnsurer) EnsurePrivateNetwork(ctx context.Context, meta network.Metadata) (docker.NetworkDetail, error) {
	f.calls++
	if f.err != nil {
		return docker.NetworkDetail{}, f.err
	}
	name, err := protocol.FormatPrivateNetworkName(meta.ProjectSlug, meta.EnvironmentSlug)
	if err != nil {
		return docker.NetworkDetail{}, err
	}
	f.name = name
	return docker.NetworkDetail{ID: "net-1", Name: name}, nil
}

func validProvisionRequest() database.ProvisionRequest {
	name, _ := protocol.FormatPrivateNetworkName("modulyn", "production")
	return database.ProvisionRequest{
		DatabaseID:      "db-1",
		NetworkName:     name,
		DNSAlias:        "db-modulyn",
		ProjectID:       "proj-1",
		ProjectSlug:     "modulyn",
		EnvironmentID:   "env-1",
		EnvironmentSlug: "production",
		OrganizationID:  "org-1",
	}
}

func TestEstablishDatabaseNetwork_EnsuresCanonicalNetwork(t *testing.T) {
	ensurer := &fakeNetworkEnsurer{}
	req := validProvisionRequest()
	if err := establishDatabaseNetwork(context.Background(), ensurer, req); err != nil {
		t.Fatal(err)
	}
	if ensurer.calls != 1 || ensurer.name != req.NetworkName {
		t.Fatalf("ensurer calls=%d name=%s", ensurer.calls, ensurer.name)
	}
}

func TestEstablishDatabaseNetwork_RejectsMismatchedIdentity(t *testing.T) {
	ensurer := &fakeNetworkEnsurer{}
	req := validProvisionRequest()
	req.NetworkName = "dc-other-production-private"
	err := establishDatabaseNetwork(context.Background(), ensurer, req)
	if err == nil {
		t.Fatal("expected mismatched network identity to fail")
	}
	if ensurer.calls != 0 {
		t.Fatal("network ensure ran for a mismatched identity")
	}
}

func TestEstablishDatabaseNetwork_RejectsProxyAndFailedEnsure(t *testing.T) {
	ensurer := &fakeNetworkEnsurer{}
	req := validProvisionRequest()
	req.NetworkName = protocol.ProxyNetworkName
	if err := establishDatabaseNetwork(context.Background(), ensurer, req); err == nil {
		t.Fatal("expected proxy network to be rejected")
	}
	if ensurer.calls != 0 {
		t.Fatal("proxy network was ensured")
	}

	req = validProvisionRequest()
	ensurer.err = context.DeadlineExceeded
	err := establishDatabaseNetwork(context.Background(), ensurer, req)
	if err == nil || !strings.Contains(err.Error(), "could not be established") {
		t.Fatalf("expected ensure failure, got %v", err)
	}
}
