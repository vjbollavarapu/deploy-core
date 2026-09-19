package audit

import (
	"encoding/json"
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
	// Read-only. No PATCH/DELETE — audit_logs are append-only.
	mux.Handle("GET /audit-logs", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /audit-logs/{auditId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
}

type recordResponse struct {
	ID             string         `json:"id"`
	OrganizationID *string        `json:"organizationId,omitempty"`
	ActorUserID    *string        `json:"actorUserId,omitempty"`
	ActorType      string         `json:"actorType"`
	Action         string         `json:"action"`
	ResourceType   string         `json:"resourceType"`
	ResourceID     *string        `json:"resourceId,omitempty"`
	RequestID      *string        `json:"requestId,omitempty"`
	IPAddress      *string        `json:"ipAddress,omitempty"`
	UserAgent      *string        `json:"userAgent,omitempty"`
	Before         map[string]any `json:"before,omitempty"`
	After          map[string]any `json:"after,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
	f := ListFilter{
		OrganizationID: orgID,
		Action:         strings.TrimSpace(r.URL.Query().Get("action")),
		ResourceType:   strings.TrimSpace(r.URL.Query().Get("resourceType")),
		ResourceID:     strings.TrimSpace(r.URL.Query().Get("resourceId")),
		Limit:          page.Limit,
		Offset:         page.Offset,
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("actorUserId")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid actorUserId", nil))
			return
		}
		f.ActorUserID = &id
	}
	items, total, err := h.svc.List(r.Context(), user.ID, f)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]recordResponse, 0, len(items))
	for _, rec := range items {
		out = append(out, toResponse(rec))
	}
	writeJSON(w, http.StatusOK, pagination.Page[recordResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("auditId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid audit id", nil))
		return
	}
	rec, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auditLog": toResponse(rec)})
}

func toResponse(rec Record) recordResponse {
	out := recordResponse{
		ID:           rec.ID.String(),
		ActorType:    rec.ActorType,
		Action:       rec.Action,
		ResourceType: rec.ResourceType,
		ResourceID:   rec.ResourceID,
		RequestID:    rec.RequestID,
		IPAddress:    rec.IPAddress,
		UserAgent:    rec.UserAgent,
		Before:       rec.Before,
		After:        rec.After,
		CreatedAt:    rec.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if rec.OrganizationID != nil {
		s := rec.OrganizationID.String()
		out.OrganizationID = &s
	}
	if rec.ActorUserID != nil {
		s := rec.ActorUserID.String()
		out.ActorUserID = &s
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	apierror.WriteError(w, requestid.FromContext(r.Context()), err)
}
