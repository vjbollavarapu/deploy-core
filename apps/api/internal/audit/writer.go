package audit

import (
	"context"
	"encoding/json"
	"net"

	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is a safe audit event. Never include secret values.
type Entry struct {
	OrganizationID *uuid.UUID
	ActorUserID    *uuid.UUID
	ActorType      string
	Action         string
	ResourceType   string
	ResourceID     string
	RequestID      string
	IPAddress      string
	UserAgent      string
	Before         map[string]any
	After          map[string]any
}

// Writer persists audit log rows.
type Writer struct {
	pool *pgxpool.Pool
}

func NewWriter(pool *pgxpool.Pool) *Writer {
	return &Writer{pool: pool}
}

func (w *Writer) Write(ctx context.Context, e Entry) error {
	if e.ActorType == "" {
		e.ActorType = "user"
	}
	var before, after []byte
	var err error
	if e.Before != nil {
		before, err = json.Marshal(e.Before)
		if err != nil {
			return err
		}
	}
	if e.After != nil {
		after, err = json.Marshal(e.After)
		if err != nil {
			return err
		}
	}
	var ip any
	if e.IPAddress != "" {
		if addr := net.ParseIP(e.IPAddress); addr != nil {
			ip = addr.String()
		}
	}
	_, err = w.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			organization_id, actor_user_id, actor_type, action, resource_type, resource_id,
			request_id, ip_address, user_agent, before_metadata, after_metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.OrganizationID, e.ActorUserID, e.ActorType, e.Action, e.ResourceType, nullIfEmpty(e.ResourceID),
		nullIfEmpty(e.RequestID), ip, nullIfEmpty(e.UserAgent), jsonOrNil(before), jsonOrNil(after),
	)
	return err
}

// WriteAuthEvent implements auth.AuditSink for login/security events (no org scope).
func (w *Writer) WriteAuthEvent(ctx context.Context, actorUserID *uuid.UUID, action, resourceType, resourceID, ip, userAgent string, after map[string]any) error {
	return w.Write(ctx, Entry{
		ActorUserID:  actorUserID,
		ActorType:    "user",
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RequestID:    requestid.FromContext(ctx),
		IPAddress:    ip,
		UserAgent:    userAgent,
		After:        after,
	})
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func jsonOrNil(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
