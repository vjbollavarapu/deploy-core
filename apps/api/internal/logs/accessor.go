package logs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAccessor resolves org ownership and deployment events (authoritative PG data).
// Build/runtime log bodies are never stored here.
type PostgresAccessor struct {
	pool *pgxpool.Pool
}

func NewPostgresAccessor(pool *pgxpool.Pool) *PostgresAccessor {
	return &PostgresAccessor{pool: pool}
}

func (a *PostgresAccessor) GetApplicationOrg(ctx context.Context, appID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := a.pool.QueryRow(ctx, `
		SELECT organization_id FROM applications
		WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(&orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return orgID, err
}

// GetApplicationPlacement returns org and assigned server for Agent scoping.
func (a *PostgresAccessor) GetApplicationPlacement(ctx context.Context, appID uuid.UUID) (orgID uuid.UUID, serverID *uuid.UUID, err error) {
	err = a.pool.QueryRow(ctx, `
		SELECT organization_id, target_server_id FROM applications
		WHERE id = $1 AND deleted_at IS NULL`, appID).Scan(&orgID, &serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, ErrNotFound
	}
	return orgID, serverID, err
}

type deploymentRef struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ApplicationID  uuid.UUID
}

func (a *PostgresAccessor) GetDeployment(ctx context.Context, deploymentID uuid.UUID) (deploymentRef, error) {
	var d deploymentRef
	err := a.pool.QueryRow(ctx, `
		SELECT id, organization_id, application_id FROM deployments WHERE id = $1`, deploymentID).
		Scan(&d.ID, &d.OrganizationID, &d.ApplicationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return deploymentRef{}, ErrNotFound
	}
	return d, err
}

type pgEvent struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	DeploymentID   uuid.UUID
	FromStatus     *string
	ToStatus       string
	Message        string
	CreatedAt      time.Time
}

func (a *PostgresAccessor) ListEvents(ctx context.Context, deploymentID uuid.UUID) ([]pgEvent, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, organization_id, deployment_id, from_status, to_status, message, created_at
		FROM deployment_events
		WHERE deployment_id = $1
		ORDER BY created_at ASC, id ASC`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pgEvent
	for rows.Next() {
		var e pgEvent
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.DeploymentID, &e.FromStatus, &e.ToStatus, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func eventToEntry(e pgEvent, appID uuid.UUID) Entry {
	msg := e.Message
	if msg == "" {
		msg = e.ToStatus
	} else {
		msg = fmt.Sprintf("%s: %s", e.ToStatus, msg)
	}
	if e.FromStatus != nil && *e.FromStatus != "" {
		msg = fmt.Sprintf("%s → %s", *e.FromStatus, msg)
	}
	depID := e.DeploymentID
	return Entry{
		Cursor:         "e" + e.ID.String(),
		Timestamp:      e.CreatedAt.UTC(),
		OrganizationID: e.OrganizationID,
		ApplicationID:  &appID,
		DeploymentID:   &depID,
		Kind:           KindDeploymentEvents,
		Stream:         StreamSystem,
		Message:        msg,
	}
}
