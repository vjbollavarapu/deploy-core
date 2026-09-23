package appcontainer

import (
	"context"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
)

// ContainerReader abstracts container listing from the Docker client.
type ContainerReader interface {
	ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error)
}

// DiscoveryFilter specifies criteria for discovering platform application containers.
type DiscoveryFilter struct {
	OrganizationID string // Enforces tenant boundary if specified
	ApplicationID  string // Matches specific application
	EnvironmentID  string // Matches specific environment
	RevisionID     string // Matches specific revision
	DeploymentID   string // Matches specific deployment
	Instance       *int   // Matches specific instance/replica index
	IncludeStopped bool   // If true, stopped and exited containers are included
}

// DiscoveredContainer represents a platform application container whose ownership has been verified.
type DiscoveredContainer struct {
	ID       string
	Name     string
	Metadata Metadata
	State    string
	Status   string
	Created  time.Time
	Ports    []docker.PortBinding
	Labels   map[string]string
}

// DiscoveredCollision represents an untrusted or colliding container discovered on the host.
type DiscoveredCollision struct {
	ID              string
	Name            string
	OwnershipStatus OwnershipStatus
	Labels          map[string]string
}

// DiscoveryResult summarizes discovered owned containers and untrusted collisions.
type DiscoveryResult struct {
	Containers []DiscoveredContainer
	Collisions []DiscoveredCollision
}

// Discover queries host containers and reconciles ownership using trusted platform labels.
// Never trusts container names alone as ownership proof.
func Discover(ctx context.Context, reader ContainerReader, filter DiscoveryFilter) (DiscoveryResult, error) {
	summaries, err := reader.ListContainers(ctx, filter.IncludeStopped)
	if err != nil {
		return DiscoveryResult{}, err
	}

	res := DiscoveryResult{
		Containers: make([]DiscoveredContainer, 0, len(summaries)),
		Collisions: make([]DiscoveredCollision, 0),
	}

	for _, c := range summaries {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		status := VerifyOwnership(name, c.Labels, filter.OrganizationID)
		switch status {
		case OwnershipValid:
			meta, err := ExtractMetadata(c.Labels)
			if err != nil {
				continue
			}

			// Apply optional filters
			if filter.ApplicationID != "" && meta.ApplicationID != filter.ApplicationID {
				continue
			}
			if filter.EnvironmentID != "" && meta.EnvironmentID != filter.EnvironmentID {
				continue
			}
			if filter.RevisionID != "" && meta.RevisionID != filter.RevisionID {
				continue
			}
			if filter.DeploymentID != "" && meta.DeploymentID != filter.DeploymentID {
				continue
			}
			if filter.Instance != nil && meta.Instance != *filter.Instance {
				continue
			}

			res.Containers = append(res.Containers, DiscoveredContainer{
				ID:       c.ID,
				Name:     name,
				Metadata: meta,
				State:    c.State,
				Status:   c.Status,
				Created:  c.Created,
				Ports:    c.Ports,
				Labels:   c.Labels,
			})

		case OwnershipUntrustedCollision:
			res.Collisions = append(res.Collisions, DiscoveredCollision{
				ID:              c.ID,
				Name:            name,
				OwnershipStatus: status,
				Labels:          c.Labels,
			})

		case OwnershipTenantMismatch:
			// If a container with a platform name belongs to another tenant, record as collision
			if IsPlatformName(name) {
				res.Collisions = append(res.Collisions, DiscoveredCollision{
					ID:              c.ID,
					Name:            name,
					OwnershipStatus: status,
					Labels:          c.Labels,
				})
			}
		}
	}

	return res, nil
}
