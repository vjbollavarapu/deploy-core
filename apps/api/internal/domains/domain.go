package domains

import (
	"time"

	"github.com/google/uuid"
)

const (
	DNSPending = "PENDING"
	DNSValid   = "VALID"
	DNSInvalid = "INVALID"
)

const (
	TLSPending  = "PENDING"
	TLSIssuing  = "ISSUING"
	TLSActive   = "ACTIVE"
	TLSExpiring = "EXPIRING"
	TLSFailed   = "FAILED"
)

type Domain struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ApplicationID  uuid.UUID
	EnvironmentID  uuid.UUID
	Hostname       string
	InternalPort   int
	IsPrimary      bool
	ForceHTTPS     bool
	DNSStatus      string
	TLSStatus      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Routing        RoutingConfig
}

// RoutingConfig is the control-plane desired Traefik routing configuration.
// Agents apply equivalent runtime labels/config; this is never agent-executed here.
type RoutingConfig struct {
	Provider string            `json:"provider"`
	Labels   map[string]string `json:"labels"`
}

type CreateInput struct {
	ApplicationID uuid.UUID
	Hostname      string
	InternalPort  *int
	IsPrimary     *bool
	ForceHTTPS    *bool
}

type UpdateInput struct {
	Hostname     *string
	InternalPort *int
	IsPrimary    *bool
	ForceHTTPS   *bool
	DNSStatus    *string
	TLSStatus    *string
}

type ApplicationRef struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	EnvironmentID  uuid.UUID
	Slug           string
	InternalPort   *int
}

type AuditMeta struct {
	IP        string
	UserAgent string
}
