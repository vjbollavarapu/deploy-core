package deployments

import (
	"encoding/json"
	"errors"
	"io"
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
	mux.Handle("POST /applications/{applicationId}/deployments", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("POST /applications/{applicationId}/rollback", h.auth.RequireAuth(http.HandlerFunc(h.Rollback)))
	mux.Handle("GET /deployments", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /deployments/{deploymentId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("POST /deployments/{deploymentId}/cancel", h.auth.RequireAuth(http.HandlerFunc(h.Cancel)))
}

type createRequest struct {
	Trigger        string  `json:"trigger"`
	IdempotencyKey *string `json:"idempotencyKey"`
	CorrelationID  *string `json:"correlationId"`
}

type rollbackRequest struct {
	TargetRevisionID string  `json:"targetRevisionId"`
	CorrelationID    *string `json:"correlationId"`
}

type deploymentResponse struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	ApplicationID    string          `json:"applicationId"`
	EnvironmentID    string          `json:"environmentId"`
	ServerID         *string         `json:"serverId,omitempty"`
	Status           string          `json:"status"`
	Trigger          string          `json:"trigger"`
	IdempotencyKey   *string         `json:"idempotencyKey,omitempty"`
	RequestID        *string         `json:"requestId,omitempty"`
	CorrelationID    *string         `json:"correlationId,omitempty"`
	CreatedBy        *string         `json:"createdBy,omitempty"`
	StartedAt        *string         `json:"startedAt,omitempty"`
	FinishedAt       *string         `json:"finishedAt,omitempty"`
	ErrorCode        *string         `json:"errorCode,omitempty"`
	ErrorMessage     *string         `json:"errorMessage,omitempty"`
	ActiveRevisionID *string         `json:"activeRevisionId,omitempty"`
	TargetRevisionID *string         `json:"targetRevisionId,omitempty"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
	Events           []eventResponse `json:"events,omitempty"`
}

type eventResponse struct {
	ID         string         `json:"id"`
	FromStatus *string        `json:"fromStatus,omitempty"`
	ToStatus   string         `json:"toStatus"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata"`
	RequestID  *string        `json:"requestId,omitempty"`
	CreatedAt  string         `json:"createdAt"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	appID, err := uuid.Parse(r.PathValue("applicationId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid application id", nil))
		return
	}
	var req createRequest
	if err := decodeJSONOptional(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	// Prefer Idempotency-Key header when body omits it.
	if req.IdempotencyKey == nil || strings.TrimSpace(*req.IdempotencyKey) == "" {
		if hdr := strings.TrimSpace(r.Header.Get("Idempotency-Key")); hdr != "" {
			req.IdempotencyKey = &hdr
		}
	}
	d, err := h.svc.Create(r.Context(), user.ID, CreateInput{
		ApplicationID:  appID,
		Trigger:        req.Trigger,
		IdempotencyKey: req.IdempotencyKey,
		CorrelationID:  req.CorrelationID,
		RequestID:      requestid.FromContext(r.Context()),
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deployment": toResponse(d, true)})
}

func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	appID, err := uuid.Parse(r.PathValue("applicationId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid application id", nil))
		return
	}
	var req rollbackRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	targetID, err := uuid.Parse(strings.TrimSpace(req.TargetRevisionID))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid targetRevisionId", map[string]any{"targetRevisionId": "must be a UUID"}))
		return
	}
	d, err := h.svc.Rollback(r.Context(), user.ID, RollbackInput{
		ApplicationID:    appID,
		TargetRevisionID: targetID,
		CorrelationID:    req.CorrelationID,
		RequestID:        requestid.FromContext(r.Context()),
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"deployment": toResponse(d, true)})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	orgID, err := uuid.Parse(r.URL.Query().Get("organizationId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId query parameter is required", nil))
		return
	}
	var applicationID *uuid.UUID
	if v := r.URL.Query().Get("applicationId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid applicationId", nil))
			return
		}
		applicationID = &id
	}
	var status *string
	if v := r.URL.Query().Get("status"); v != "" {
		status = &v
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, applicationID, status, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]deploymentResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toResponse(d, false))
	}
	writeJSON(w, http.StatusOK, pagination.Page[deploymentResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndDeployment(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	d, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment": toResponse(d, true)})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndDeployment(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	d, err := h.svc.Cancel(r.Context(), user.ID, id, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployment": toResponse(d, true)})
}

func actorAndDeployment(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("deploymentId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid deployment id", nil)
	}
	return user, id, nil
}

func toResponse(d Deployment, withEvents bool) deploymentResponse {
	out := deploymentResponse{
		ID:             d.ID.String(),
		OrganizationID: d.OrganizationID.String(),
		ApplicationID:  d.ApplicationID.String(),
		EnvironmentID:  d.EnvironmentID.String(),
		Status:         d.Status,
		Trigger:        d.Trigger,
		IdempotencyKey: d.IdempotencyKey,
		RequestID:      d.RequestID,
		CorrelationID:  d.CorrelationID,
		ErrorCode:      d.ErrorCode,
		ErrorMessage:   d.ErrorMessage,
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.ServerID = uuidPtr(d.ServerID)
	out.CreatedBy = uuidPtr(d.CreatedBy)
	out.ActiveRevisionID = uuidPtr(d.ActiveRevisionID)
	out.TargetRevisionID = uuidPtr(d.TargetRevisionID)
	out.StartedAt = timePtr(d.StartedAt)
	out.FinishedAt = timePtr(d.FinishedAt)
	if withEvents {
		out.Events = make([]eventResponse, 0, len(d.Events))
		for _, e := range d.Events {
			out.Events = append(out.Events, eventResponse{
				ID:         e.ID.String(),
				FromStatus: e.FromStatus,
				ToStatus:   e.ToStatus,
				Message:    e.Message,
				Metadata:   e.Metadata,
				RequestID:  e.RequestID,
				CreatedAt:  e.CreatedAt.UTC().Format(time.RFC3339Nano),
			})
		}
	}
	return out
}

func uuidPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func auditMeta(r *http.Request) AuditMeta {
	return AuditMeta{IP: clientIP(r), UserAgent: r.UserAgent()}
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

func decodeJSONOptional(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
