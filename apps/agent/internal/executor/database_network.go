package executor

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/database"
	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

type privateNetworkEnsurer interface {
	EnsurePrivateNetwork(ctx context.Context, meta network.Metadata) (docker.NetworkDetail, error)
}

func validateDatabaseNetworkIdentity(req database.ProvisionRequest) error {
	if strings.TrimSpace(req.DatabaseID) == "" {
		return fmt.Errorf("databaseId is required")
	}
	if strings.TrimSpace(req.ProjectID) == "" || strings.TrimSpace(req.EnvironmentID) == "" || strings.TrimSpace(req.OrganizationID) == "" {
		return fmt.Errorf("project, environment, and organization identity are required")
	}
	canonical, err := protocol.FormatPrivateNetworkName(req.ProjectSlug, req.EnvironmentSlug)
	if err != nil {
		return fmt.Errorf("private network identity is invalid: %w", err)
	}
	if strings.TrimSpace(req.NetworkName) != canonical {
		return fmt.Errorf("networkName %q does not match private network %q", req.NetworkName, canonical)
	}
	if req.NetworkName == protocol.ProxyNetworkName {
		return fmt.Errorf("database cannot join %s", protocol.ProxyNetworkName)
	}
	if !protocol.ValidDNSAlias(req.DNSAlias) {
		return fmt.Errorf("dnsAlias %q is not a valid DNS label", req.DNSAlias)
	}
	return nil
}

// establishDatabaseNetwork checks the command identity and ensures the shared
// private network before a database container is created.
func establishDatabaseNetwork(ctx context.Context, ensurer privateNetworkEnsurer, req database.ProvisionRequest) error {
	if err := validateDatabaseNetworkIdentity(req); err != nil {
		return err
	}
	if ensurer == nil {
		return fmt.Errorf("private network manager is required")
	}
	detail, err := ensurer.EnsurePrivateNetwork(ctx, network.Metadata{
		OrganizationID:  req.OrganizationID,
		ProjectID:       req.ProjectID,
		ProjectSlug:     req.ProjectSlug,
		EnvironmentID:   req.EnvironmentID,
		EnvironmentSlug: req.EnvironmentSlug,
		NetworkType:     protocol.NetworkTypePrivate,
	})
	if err != nil {
		return fmt.Errorf("private network %s could not be established: %w", req.NetworkName, err)
	}
	if detail.Name != req.NetworkName {
		return fmt.Errorf("ensured network %q does not match %q", detail.Name, req.NetworkName)
	}
	return nil
}
