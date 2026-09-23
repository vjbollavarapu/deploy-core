package notifications

import (
	"time"

	"github.com/google/uuid"
)

const (
	ChannelEmail    = "EMAIL"
	ChannelWebhook  = "WEBHOOK"
	ChannelSlack    = "SLACK"
	ChannelTeams    = "TEAMS"
	ChannelDiscord  = "DISCORD"
	ChannelTelegram = "TELEGRAM"
	ChannelWhatsApp = "WHATSAPP"
)

const (
	ChannelStatusActive   = "ACTIVE"
	ChannelStatusDisabled = "DISABLED"
	ChannelStatusFailed   = "FAILED"
)

const (
	EventDeploymentFailed    = "DEPLOYMENT_FAILED"
	EventDeploymentSucceeded = "DEPLOYMENT_SUCCEEDED"
	EventServerOffline       = "SERVER_OFFLINE"
	EventServerDegraded      = "SERVER_DEGRADED"
	EventBackupFailed        = "BACKUP_FAILED"
	EventCertificateExpiring = "CERTIFICATE_EXPIRING"
	EventDiskLow             = "DISK_LOW"
)

const (
	DeliveryPending    = "PENDING"
	DeliveryQueued     = "QUEUED"
	DeliveryDelivering = "DELIVERING"
	DeliveryDelivered  = "DELIVERED"
	DeliveryFailed     = "FAILED"
	DeliverySkipped    = "SKIPPED"
)

func EnabledChannelTypes() []string {
	return []string{ChannelEmail, ChannelWebhook}
}

func ReservedChannelTypes() []string {
	return []string{ChannelSlack, ChannelTeams, ChannelDiscord, ChannelTelegram, ChannelWhatsApp}
}

func KnownEvents() []string {
	return []string{
		EventDeploymentFailed, EventDeploymentSucceeded,
		EventServerOffline, EventServerDegraded,
		EventBackupFailed, EventCertificateExpiring, EventDiskLow,
	}
}

type Channel struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Name           string
	Type           string
	Config         map[string]any
	Enabled        bool
	Status         string
	LastError      string
	HasCredential  bool
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

type Policy struct {
	ID                 uuid.UUID
	OrganizationID     uuid.UUID
	Name               string
	EventTypes         []string
	ResourceFilters    map[string]any
	EnvironmentFilters map[string]any
	ChannelIDs         []uuid.UUID
	Enabled            bool
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type Delivery struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	PolicyID       *uuid.UUID
	ChannelID      uuid.UUID
	EventType      string
	Payload        map[string]any
	Status         string
	AttemptCount   int
	JobID          *uuid.UUID
	ResponseCode   *int
	LatencyMs      *int
	LastError      string
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateChannelInput struct {
	OrganizationID uuid.UUID
	Name           string
	Type           string
	Config         map[string]any
	Credential     string // optional bearer/token sealed at rest
	Enabled        *bool
}

type UpdateChannelInput struct {
	Name            *string
	Config          map[string]any
	Credential      *string
	ClearCredential bool
	Enabled         *bool
	Status          *string
}

type CreatePolicyInput struct {
	OrganizationID     uuid.UUID
	Name               string
	EventTypes         []string
	ResourceFilters    map[string]any
	EnvironmentFilters map[string]any
	ChannelIDs         []uuid.UUID
	Enabled            *bool
}

type UpdatePolicyInput struct {
	Name               *string
	EventTypes         []string
	ResourceFilters    map[string]any
	EnvironmentFilters map[string]any
	ChannelIDs         []uuid.UUID
	Enabled            *bool
}

type EmitInput struct {
	OrganizationID uuid.UUID
	EventType      string
	Payload        map[string]any
	ApplicationID  *uuid.UUID
	EnvironmentID  *uuid.UUID
	ServerID       *uuid.UUID
	ResourceType   string
	ResourceID     *uuid.UUID
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

type credentialBlob struct {
	Ciphertext []byte
	Nonce      []byte
	KeyID      string
}
