package backups

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

// Destination allocates platform-controlled backup locations.
// S3-compatible adapters can be added later without changing the API contract.
type Destination interface {
	Type() string
	Allocate(ctx context.Context, orgID, backupID uuid.UUID) (uri string, err error)
}

type LocalDestination struct{}

func (LocalDestination) Type() string { return DestLocal }

func (LocalDestination) Allocate(_ context.Context, orgID, backupID uuid.UUID) (string, error) {
	// Logical platform URI — agents map this under a managed backup root.
	// Never accept user-supplied absolute paths.
	return fmt.Sprintf("local://org/%s/backups/%s.dump.gz", orgID.String(), backupID.String()), nil
}

type S3Destination struct {
	Bucket string
	Prefix string
}

func (d S3Destination) Type() string { return DestS3 }

func (d S3Destination) Allocate(_ context.Context, orgID, backupID uuid.UUID) (string, error) {
	return "", apierror.Validation("s3 destination is reserved but not enabled yet", map[string]any{
		"destination": DestS3,
	})
}

func LookupDestination(name string) (Destination, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", DestLocal:
		return LocalDestination{}, nil
	case DestS3:
		return S3Destination{}, nil
	default:
		return nil, apierror.Validation("unsupported backup destination", map[string]any{
			"destination": name,
			"supported":   []string{DestLocal},
		})
	}
}
