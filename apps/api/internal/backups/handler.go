package backups

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/pagination"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type Handler struct {
	svc  *Service
	auth *auth.Handler
}

func NewHandler(svc *Service, authHandler *auth.Handler) *Handler {
	return &Handler{svc: svc, auth: authHandler}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("POST /databases/{databaseId}/backups", h.auth.RequireAuth(http.HandlerFunc(h.CreateBackup)))
	mux.Handle("GET /databases/{databaseId}/backups", h.auth.RequireAuth(http.HandlerFunc(h.ListDatabaseBackups)))
	mux.Handle("GET /backups/{backupId}", h.auth.RequireAuth(http.HandlerFunc(h.GetBackup)))
	mux.Handle("DELETE /backups/{backupId}", h.auth.RequireAuth(http.HandlerFunc(h.DeleteBackup)))
	mux.Handle("POST /backups/{backupId}/restore", h.auth.RequireAuth(http.HandlerFunc(h.CreateRestore)))
	mux.Handle("GET /restores/{restoreId}", h.auth.RequireAuth(http.HandlerFunc(h.GetRestore)))
	mux.Handle("GET /restores", h.auth.RequireAuth(http.HandlerFunc(h.ListRestores)))
}

type createBackupRequest struct {
	Destination    string  `json:"destination"`
	RetentionDays  *int    `json:"retentionDays"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

type createRestoreRequest struct {
	TargetDatabaseID string  `json:"targetDatabaseId"`
	Confirm          string  `json:"confirm"`
	IdempotencyKey   *string `json:"idempotencyKey"`
}

type backupResponse struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organizationId"`
	ServerID        string         `json:"serverId"`
	ResourceType    string         `json:"resourceType"`
	ResourceID      string         `json:"resourceId"`
	Type            string         `json:"type"`
	Status          string         `json:"status"`
	StartedAt       *string        `json:"startedAt,omitempty"`
	CompletedAt     *string        `json:"completedAt,omitempty"`
	DurationMs      *int64         `json:"durationMs,omitempty"`
	SizeBytes       *int64         `json:"sizeBytes,omitempty"`
	Checksum        string         `json:"checksum,omitempty"`
	DestinationType string         `json:"destinationType"`
	DestinationURI  string         `json:"destinationUri"`
	RetentionUntil  *string        `json:"retentionUntil,omitempty"`
	JobID           *string        `json:"jobId,omitempty"`
	CommandID       *string        `json:"commandId,omitempty"`
	LastError       string         `json:"lastError,omitempty"`
	Metadata        map[string]any `json:"metadata"`
	CreatedBy       *string        `json:"createdBy,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
}

type restoreResponse struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organizationId"`
	BackupID           string         `json:"backupId"`
	TargetResourceType string         `json:"targetResourceType"`
	TargetResourceID   string         `json:"targetResourceId"`
	ServerID           string         `json:"serverId"`
	Status             string         `json:"status"`
	StartedAt          *string        `json:"startedAt,omitempty"`
	CompletedAt        *string        `json:"completedAt,omitempty"`
	DurationMs         *int64         `json:"durationMs,omitempty"`
	JobID              *string        `json:"jobId,omitempty"`
	CommandID          *string        `json:"commandId,omitempty"`
	ValidationPassed   *bool          `json:"validationPassed,omitempty"`
	LastError          string         `json:"lastError,omitempty"`
	Metadata           map[string]any `json:"metadata"`
	CreatedBy          *string        `json:"createdBy,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	dbID, err := uuid.Parse(r.PathValue("databaseId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid database id", nil))
		return
	}
	var req createBackupRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeErr(w, r, err)
			return
		}
	}
	b, err := h.svc.CreateBackup(r.Context(), user.ID, CreateBackupInput{
		DatabaseID:     dbID,
		Destination:    req.Destination,
		RetentionDays:  req.RetentionDays,
		IdempotencyKey: req.IdempotencyKey,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"backup": toBackupResponse(b)})
}

func (h *Handler) ListDatabaseBackups(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	dbID, err := uuid.Parse(r.PathValue("databaseId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid database id", nil))
		return
	}
	db, err := h.svc.repo.GetDatabase(r.Context(), dbID)
	if err != nil {
		writeErr(w, r, apierror.NotFoundCode(apierror.CodeDatabaseNotFound, "database not found"))
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListBackups(r.Context(), user.ID, db.OrganizationID, &dbID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]backupResponse, 0, len(items))
	for _, b := range items {
		out = append(out, toBackupResponse(b))
	}
	writeJSON(w, http.StatusOK, pagination.Page[backupResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) GetBackup(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndBackup(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	b, err := h.svc.GetBackup(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backup": toBackupResponse(b)})
}

func (h *Handler) DeleteBackup(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndBackup(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.DeleteBackup(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateRestore(w http.ResponseWriter, r *http.Request) {
	user, backupID, err := actorAndBackup(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req createRestoreRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	targetID, err := uuid.Parse(strings.TrimSpace(req.TargetDatabaseID))
	if err != nil {
		writeErr(w, r, apierror.Validation("targetDatabaseId is required", nil))
		return
	}
	rest, err := h.svc.CreateRestore(r.Context(), user.ID, CreateRestoreInput{
		BackupID:         backupID,
		TargetDatabaseID: targetID,
		Confirm:          req.Confirm,
		IdempotencyKey:   req.IdempotencyKey,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"restore": toRestoreResponse(rest)})
}

func (h *Handler) GetRestore(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("restoreId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid restore id", nil))
		return
	}
	rest, err := h.svc.GetRestore(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restore": toRestoreResponse(rest)})
}

func (h *Handler) ListRestores(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("organizationId")))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	page := pagination.FromRequest(r)
	var backupID, targetID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("backupId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid backupId", nil))
			return
		}
		backupID = &id
	}
	if v := strings.TrimSpace(r.URL.Query().Get("targetDatabaseId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid targetDatabaseId", nil))
			return
		}
		targetID = &id
	}
	items, total, err := h.svc.ListRestores(r.Context(), user.ID, orgID, backupID, targetID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]restoreResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toRestoreResponse(item))
	}
	writeJSON(w, http.StatusOK, pagination.Page[restoreResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func toBackupResponse(b Backup) backupResponse {
	out := backupResponse{
		ID: b.ID.String(), OrganizationID: b.OrganizationID.String(), ServerID: b.ServerID.String(),
		ResourceType: b.ResourceType, ResourceID: b.ResourceID.String(), Type: b.Type, Status: b.Status,
		DurationMs: b.DurationMs, SizeBytes: b.SizeBytes, Checksum: b.Checksum,
		DestinationType: b.DestinationType, DestinationURI: b.DestinationURI,
		LastError: b.LastError, Metadata: b.Metadata,
		CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: b.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.StartedAt = timePtr(b.StartedAt)
	out.CompletedAt = timePtr(b.CompletedAt)
	out.RetentionUntil = timePtr(b.RetentionUntil)
	out.JobID = uuidPtr(b.JobID)
	out.CommandID = uuidPtr(b.CommandID)
	out.CreatedBy = uuidPtr(b.CreatedBy)
	return out
}

func toRestoreResponse(r Restore) restoreResponse {
	out := restoreResponse{
		ID: r.ID.String(), OrganizationID: r.OrganizationID.String(), BackupID: r.BackupID.String(),
		TargetResourceType: r.TargetResourceType, TargetResourceID: r.TargetResourceID.String(),
		ServerID: r.ServerID.String(), Status: r.Status, DurationMs: r.DurationMs,
		ValidationPassed: r.ValidationPassed, LastError: r.LastError, Metadata: r.Metadata,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.StartedAt = timePtr(r.StartedAt)
	out.CompletedAt = timePtr(r.CompletedAt)
	out.JobID = uuidPtr(r.JobID)
	out.CommandID = uuidPtr(r.CommandID)
	out.CreatedBy = uuidPtr(r.CreatedBy)
	return out
}

func actorAndBackup(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("backupId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid backup id", nil)
	}
	return user, id, nil
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func uuidPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func auditMeta(r *http.Request) AuditMeta {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	return AuditMeta{IP: ip, UserAgent: r.UserAgent()}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.Validation("invalid JSON body", nil)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	apierror.WriteError(w, requestid.FromContext(r.Context()), err)
}
