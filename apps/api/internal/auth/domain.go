package auth

import (
	"time"

	"github.com/google/uuid"
)

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
	UserStatusPending  = "pending"
)

// User is the auth domain user (no password hash in API responses).
type User struct {
	ID              uuid.UUID
	Email           string
	DisplayName     string
	Status          string
	MFAEnabled      bool
	EmailVerifiedAt *time.Time
	LastLoginAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// UserRecord includes secrets for repository/service use only.
type UserRecord struct {
	User
	PasswordHash string
	DeletedAt    *time.Time
}

// Session is a refresh-token-backed login session.
type Session struct {
	ID                   uuid.UUID
	UserID               uuid.UUID
	FamilyID             uuid.UUID
	ReplacedBySessionID  *uuid.UUID
	RefreshTokenHash     string
	UserAgent            *string
	IPAddress            *string
	ExpiresAt            time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// PasswordResetToken stores only a hash of the reset secret.
type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RequestID *string
	CreatedAt time.Time
}

// TokenPair is returned after register/login/refresh.
// Access tokens are short-lived and carry no permissions.
type TokenPair struct {
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken"`
	AccessExpiresAt  time.Time `json:"accessExpiresAt"`
	RefreshExpiresAt time.Time `json:"refreshExpiresAt"`
	TokenType        string    `json:"tokenType"`
}

// MFAChallenge is a placeholder for future MFA enrollment/verification.
// Full MFA is not implemented in B3; login still succeeds when MFA is off.
type MFAChallenge struct {
	Required bool     `json:"required"`
	Methods  []string `json:"methods,omitempty"`
}
