package registries

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
	mux.Handle("POST /integrations/registries", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /integrations/registries", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /integrations/registries/{registryId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /integrations/registries/{registryId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /integrations/registries/{registryId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

type createRequest struct {
	OrganizationID string              `json:"organizationId"`
	Name           string              `json:"name"`
	Provider       string              `json:"provider"`
	RegistryURL    string              `json:"registryUrl"`
	Username       string              `json:"username"`
	Credentials    *credentialsRequest `json:"credentials"`
	Metadata       map[string]any      `json:"metadata"`
}

type updateRequest struct {
	Name             *string             `json:"name"`
	RegistryURL      *string             `json:"registryUrl"`
	Username         *string             `json:"username"`
	Status           *string             `json:"status"`
	Credentials      *credentialsRequest `json:"credentials"`
	ClearCredentials bool                `json:"clearCredentials"`
	Metadata         map[string]any      `json:"metadata"`
}

type registryResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Provider       string         `json:"provider"`
	RegistryURL    string         `json:"registryUrl"`
	Username       string         `json:"username,omitempty"`
	Status         string         `json:"status"`
	HasCredentials bool           `json:"hasCredentials"`
	Metadata       map[string]any `json:"metadata"`
	CreatedBy      *string        `json:"createdBy,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
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
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	reg, err := h.svc.Create(r.Context(), user.ID, CreateInput{
		OrganizationID: orgID,
		Name:           req.Name,
		Provider:       req.Provider,
		RegistryURL:    req.RegistryURL,
		Username:       req.Username,
		Credentials:    toCredentials(req.Credentials),
		Metadata:       req.Metadata,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"registry": toResponse(reg)})
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
	page := pagination.FromRequest(r)
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]registryResponse, 0, len(items))
	for _, reg := range items {
		out = append(out, toResponse(reg))
	}
	writeJSON(w, http.StatusOK, pagination.Page[registryResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndRegistry(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	reg, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registry": toResponse(reg)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndRegistry(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	reg, err := h.svc.Update(r.Context(), user.ID, id, UpdateInput{
		Name:             req.Name,
		RegistryURL:      req.RegistryURL,
		Username:         req.Username,
		Status:           req.Status,
		Credentials:      toCredentials(req.Credentials),
		ClearCredentials: req.ClearCredentials,
		Metadata:         req.Metadata,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registry": toResponse(reg)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndRegistry(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func actorAndRegistry(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("registryId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid registry id", nil)
	}
	return user, id, nil
}

func toCredentials(req *credentialsRequest) *Credentials {
	if req == nil {
		return nil
	}
	return &Credentials{Username: req.Username, Password: req.Password, Token: req.Token}
}

func toResponse(reg Registry) registryResponse {
	out := registryResponse{
		ID:             reg.ID.String(),
		OrganizationID: reg.OrganizationID.String(),
		Name:           reg.Name,
		Provider:       reg.Provider,
		RegistryURL:    reg.RegistryURL,
		Username:       reg.Username,
		Status:         reg.Status,
		HasCredentials: reg.HasCredentials,
		Metadata:       mapOrEmpty(reg.Metadata),
		CreatedAt:      reg.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      reg.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if reg.CreatedBy != nil {
		s := reg.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
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
