package projects

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
	mux.Handle("POST /projects", h.auth.RequireAuth(http.HandlerFunc(h.CreateProject)))
	mux.Handle("GET /projects", h.auth.RequireAuth(http.HandlerFunc(h.ListProjects)))
	mux.Handle("GET /projects/{projectId}", h.auth.RequireAuth(http.HandlerFunc(h.GetProject)))
	mux.Handle("PATCH /projects/{projectId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdateProject)))
	mux.Handle("DELETE /projects/{projectId}", h.auth.RequireAuth(http.HandlerFunc(h.DeleteProject)))

	mux.Handle("POST /projects/{projectId}/environments", h.auth.RequireAuth(http.HandlerFunc(h.CreateEnvironment)))
	mux.Handle("GET /projects/{projectId}/environments", h.auth.RequireAuth(http.HandlerFunc(h.ListEnvironments)))
	mux.Handle("GET /environments/{environmentId}", h.auth.RequireAuth(http.HandlerFunc(h.GetEnvironment)))
	mux.Handle("PATCH /environments/{environmentId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdateEnvironment)))
	mux.Handle("DELETE /environments/{environmentId}", h.auth.RequireAuth(http.HandlerFunc(h.DeleteEnvironment)))
}

type createProjectRequest struct {
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Description    string `json:"description"`
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Slug        *string `json:"slug"`
	Description *string `json:"description"`
}

type createEnvironmentRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
	Kind string `json:"kind"`
}

type updateEnvironmentRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
	Kind *string `json:"kind"`
}

type projectResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Description    string  `json:"description"`
	CreatedBy      *string `json:"createdBy,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type environmentResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Kind           string `json:"kind"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid organizationId", nil))
		return
	}
	p, err := h.svc.CreateProject(r.Context(), user.ID, orgID, req.Name, req.Slug, req.Description, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": toProjectResponse(p)})
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
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
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListProjects(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]projectResponse, 0, len(items))
	for _, p := range items {
		out = append(out, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, pagination.Page[projectResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	user, projectID, err := actorAndProject(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	p, err := h.svc.GetProject(r.Context(), user.ID, projectID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": toProjectResponse(p)})
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	user, projectID, err := actorAndProject(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	p, err := h.svc.UpdateProject(r.Context(), user.ID, projectID, req.Name, req.Slug, req.Description, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": toProjectResponse(p)})
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	user, projectID, err := actorAndProject(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.DeleteProject(r.Context(), user.ID, projectID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	user, projectID, err := actorAndProject(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req createEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	env, err := h.svc.CreateEnvironment(r.Context(), user.ID, projectID, req.Name, req.Slug, req.Kind, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"environment": toEnvironmentResponse(env)})
}

func (h *Handler) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	user, projectID, err := actorAndProject(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListEnvironments(r.Context(), user.ID, projectID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]environmentResponse, 0, len(items))
	for _, e := range items {
		out = append(out, toEnvironmentResponse(e))
	}
	writeJSON(w, http.StatusOK, pagination.Page[environmentResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	envID, err := uuid.Parse(r.PathValue("environmentId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid environment id", nil))
		return
	}
	env, err := h.svc.GetEnvironment(r.Context(), user.ID, envID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"environment": toEnvironmentResponse(env)})
}

func (h *Handler) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	envID, err := uuid.Parse(r.PathValue("environmentId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid environment id", nil))
		return
	}
	var req updateEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	env, err := h.svc.UpdateEnvironment(r.Context(), user.ID, envID, req.Name, req.Slug, req.Kind, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"environment": toEnvironmentResponse(env)})
}

func (h *Handler) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	envID, err := uuid.Parse(r.PathValue("environmentId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid environment id", nil))
		return
	}
	if err := h.svc.DeleteEnvironment(r.Context(), user.ID, envID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func actorAndProject(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	projectID, err := uuid.Parse(r.PathValue("projectId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid project id", nil)
	}
	return user, projectID, nil
}

func toProjectResponse(p Project) projectResponse {
	out := projectResponse{
		ID:             p.ID.String(),
		OrganizationID: p.OrganizationID.String(),
		Name:           p.Name,
		Slug:           p.Slug,
		Description:    p.Description,
		CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if p.CreatedBy != nil {
		s := p.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toEnvironmentResponse(e Environment) environmentResponse {
	return environmentResponse{
		ID:             e.ID.String(),
		OrganizationID: e.OrganizationID.String(),
		ProjectID:      e.ProjectID.String(),
		Name:           e.Name,
		Slug:           e.Slug,
		Kind:           e.Kind,
		CreatedAt:      e.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      e.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
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
