package gitproviders

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProviderGitHub    = "github"
	ProviderGitLab    = "gitlab"
	ProviderBitbucket = "bitbucket"
	ProviderGeneric   = "generic"
)

const (
	StatusActive   = "active"
	StatusError    = "error"
	StatusRevoked  = "revoked"
	StatusDisabled = "disabled"
)

const (
	DeliveryReceived  = "received"
	DeliveryProcessed = "processed"
	DeliveryIgnored   = "ignored"
	DeliveryDuplicate = "duplicate"
	DeliveryFailed    = "failed"
)

// Connection is an org-scoped git provider integration (credentials encrypted at rest).
type Connection struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Provider       string
	AccountLogin   string
	DisplayName    string
	Status         string
	LastSyncAt     *time.Time
	Metadata       map[string]any
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// HasWebhookSecret is true when a webhook signing secret is configured.
	HasWebhookSecret bool
	// WebhookSecretPlain is returned once on create/rotate; never persisted plaintext.
	WebhookSecretPlain *string
}

type Repository struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ConnectionID   uuid.UUID
	ExternalID     string
	FullName       string
	DefaultBranch  string
	CloneURL       string
	HTMLURL        string
	Metadata       map[string]any
	LastSyncAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type WebhookDelivery struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	ConnectionID       uuid.UUID
	Provider           string
	DeliveryID         string
	EventType          string
	RepositoryFullName string
	Branch             string
	CommitSHA          string
	Status             string
	DeploymentIDs      []uuid.UUID
	ErrorMessage       *string
	CreatedAt          time.Time
}

type CreateConnectionInput struct {
	OrganizationID uuid.UUID
	Provider       string
	AccountLogin   string
	DisplayName    string
	AccessToken    string
	WebhookSecret  *string
	Metadata       map[string]any
}

type UpdateConnectionInput struct {
	AccountLogin  *string
	DisplayName   *string
	Status        *string
	AccessToken   *string
	WebhookSecret *string
	Metadata      map[string]any
}

type UpsertRepositoryInput struct {
	ExternalID    string
	FullName      string
	DefaultBranch string
	CloneURL      string
	HTMLURL       string
	Metadata      map[string]any
}

// PushEvent is a normalized provider push webhook.
type PushEvent struct {
	DeliveryID         string
	EventType          string
	RepositoryFullName string
	CloneURL           string
	HTMLURL            string
	Branch             string
	CommitSHA          string
	Deleted            bool
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
