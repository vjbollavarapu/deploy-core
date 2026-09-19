package revisions

import (
	"encoding/json"
	"net/http"
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
	mux.Handle("GET /applications/{applicationId}/revisions", h.auth.RequireAuth(http.HandlerFunc(h.ListByApplication)))
	mux.Handle("GET /revisions/{revisionId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
}

type revisionResponse struct {
	ID               string         `json:"id"`
	OrganizationID   string         `json:"organizationId"`
	ApplicationID    string         `json:"applicationId"`
	DeploymentID     *string        `json:"deploymentId,omitempty"`
	RevisionNumber   int            `json:"revisionNumber"`
	Status           string         `json:"status"`
	CommitSHA        *string        `json:"commitSha,omitempty"`
	ImageDigest      *string        `json:"imageDigest,omitempty"`
	ImageTag         *string        `json:"imageTag,omitempty"`
	EffectiveConfig  map[string]any `json:"effectiveConfig"`
	VariableSnapshot map[string]any `json:"variableSnapshot"`
	SecretRefs       []any          `json:"secretRefs"`
	HealthCheck      map[string]any `json:"healthCheck"`
	ResourceLimits   map[string]any `json:"resourceLimits"`
	CreatedBy        *string        `json:"createdBy,omitempty"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
}

func (h *Handler) ListByApplication(w http.ResponseWriter, r *http.Request) {
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
	var status *string
	if v := r.URL.Query().Get("status"); v != "" {
		status = &v
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListByApplication(r.Context(), user.ID, appID, status, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]revisionResponse, 0, len(items))
	for _, rev := range items {
		out = append(out, toResponse(rev))
	}
	writeJSON(w, http.StatusOK, pagination.Page[revisionResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("revisionId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid revision id", nil))
		return
	}
	rev, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revision": toResponse(rev)})
}

func toResponse(rev Revision) revisionResponse {
	out := revisionResponse{
		ID:               rev.ID.String(),
		OrganizationID:   rev.OrganizationID.String(),
		ApplicationID:    rev.ApplicationID.String(),
		RevisionNumber:   rev.RevisionNumber,
		Status:           rev.Status,
		CommitSHA:        rev.CommitSHA,
		ImageDigest:      rev.ImageDigest,
		ImageTag:         rev.ImageTag,
		EffectiveConfig:  mapOrEmpty(rev.EffectiveConfig),
		VariableSnapshot: mapOrEmpty(rev.VariableSnapshot),
		SecretRefs:       rev.SecretRefs,
		HealthCheck:      mapOrEmpty(rev.HealthCheck),
		ResourceLimits:   mapOrEmpty(rev.ResourceLimits),
		CreatedAt:        rev.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:        rev.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if out.SecretRefs == nil {
		out.SecretRefs = []any{}
	}
	if rev.DeploymentID != nil {
		s := rev.DeploymentID.String()
		out.DeploymentID = &s
	}
	if rev.CreatedBy != nil {
		s := rev.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	apierror.WriteError(w, requestid.FromContext(r.Context()), err)
}
