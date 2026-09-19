package secrets

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
	mux.Handle("POST /secrets", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /secrets", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /secrets/{secretId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /secrets/{secretId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /secrets/{secretId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
}

type createRequest struct {
	OrganizationID string  `json:"organizationId"`
	Scope          string  `json:"scope"`
	ProjectID      *string `json:"projectId"`
	EnvironmentID  *string `json:"environmentId"`
	ApplicationID  *string `json:"applicationId"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
}

type updateRequest struct {
	Value string `json:"value"`
}

type secretResponse struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Scope          string  `json:"scope"`
	ProjectID      *string `json:"projectId,omitempty"`
	EnvironmentID  *string `json:"environmentId,omitempty"`
	ApplicationID  *string `json:"applicationId,omitempty"`
	Name           string  `json:"name"`
	Version        int     `json:"version"`
	KeyID          string  `json:"keyId"`
	Algorithm      string  `json:"algorithm"`
	CreatedBy      *string `json:"createdBy,omitempty"`
	UpdatedBy      *string `json:"updatedBy,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	in, err := parseCreate(req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	m, err := h.svc.Create(r.Context(), user.ID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"secret": toResponse(m)})
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
	scope, projectID, environmentID, applicationID, err := parseScopeFilters(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, scope, projectID, environmentID, applicationID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]secretResponse, 0, len(items))
	for _, m := range items {
		out = append(out, toResponse(m))
	}
	writeJSON(w, http.StatusOK, pagination.Page[secretResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	m, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": toResponse(m)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	m, err := h.svc.Rotate(r.Context(), user.ID, id, req.Value, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": toResponse(m)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func parseCreate(req createRequest) (CreateInput, error) {
	in := CreateInput{
		Scope: req.Scope,
		Name:  req.Name,
		Value: req.Value,
	}
	if strings.TrimSpace(req.OrganizationID) != "" {
		orgID, err := uuid.Parse(req.OrganizationID)
		if err != nil {
			return CreateInput{}, apierror.Validation("invalid organizationId", nil)
		}
		in.OrganizationID = orgID
	}
	if req.ProjectID != nil && strings.TrimSpace(*req.ProjectID) != "" {
		id, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			return CreateInput{}, apierror.Validation("invalid projectId", nil)
		}
		in.ProjectID = &id
	}
	if req.EnvironmentID != nil && strings.TrimSpace(*req.EnvironmentID) != "" {
		id, err := uuid.Parse(*req.EnvironmentID)
		if err != nil {
			return CreateInput{}, apierror.Validation("invalid environmentId", nil)
		}
		in.EnvironmentID = &id
	}
	if req.ApplicationID != nil && strings.TrimSpace(*req.ApplicationID) != "" {
		id, err := uuid.Parse(*req.ApplicationID)
		if err != nil {
			return CreateInput{}, apierror.Validation("invalid applicationId", nil)
		}
		in.ApplicationID = &id
	}
	return in, nil
}

func parseScopeFilters(r *http.Request) (scope *string, projectID, environmentID, applicationID *uuid.UUID, err error) {
	if v := r.URL.Query().Get("scope"); v != "" {
		scope = &v
	}
	if v := r.URL.Query().Get("projectId"); v != "" {
		id, e := uuid.Parse(v)
		if e != nil {
			return nil, nil, nil, nil, apierror.Validation("invalid projectId", nil)
		}
		projectID = &id
	}
	if v := r.URL.Query().Get("environmentId"); v != "" {
		id, e := uuid.Parse(v)
		if e != nil {
			return nil, nil, nil, nil, apierror.Validation("invalid environmentId", nil)
		}
		environmentID = &id
	}
	if v := r.URL.Query().Get("applicationId"); v != "" {
		id, e := uuid.Parse(v)
		if e != nil {
			return nil, nil, nil, nil, apierror.Validation("invalid applicationId", nil)
		}
		applicationID = &id
	}
	return scope, projectID, environmentID, applicationID, nil
}

func actorAndID(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("secretId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid secret id", nil)
	}
	return user, id, nil
}

func toResponse(m Metadata) secretResponse {
	out := secretResponse{
		ID:             m.ID.String(),
		OrganizationID: m.OrganizationID.String(),
		Scope:          m.Scope,
		Name:           m.Name,
		Version:        m.Version,
		KeyID:          m.KeyID,
		Algorithm:      m.Algorithm,
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      m.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.ProjectID = uuidPtr(m.ProjectID)
	out.EnvironmentID = uuidPtr(m.EnvironmentID)
	out.ApplicationID = uuidPtr(m.ApplicationID)
	out.CreatedBy = uuidPtr(m.CreatedBy)
	out.UpdatedBy = uuidPtr(m.UpdatedBy)
	return out
}

func uuidPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
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
