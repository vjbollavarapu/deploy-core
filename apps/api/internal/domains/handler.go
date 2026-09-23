package domains

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
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
	mux.Handle("POST /applications/{applicationId}/domains", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /applications/{applicationId}/domains", h.auth.RequireAuth(http.HandlerFunc(h.ListByApplication)))
	mux.Handle("GET /domains", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("PATCH /domains/{domainId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /domains/{domainId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
}

type createRequest struct {
	Hostname     string `json:"hostname"`
	InternalPort *int   `json:"internalPort"`
	IsPrimary    *bool  `json:"isPrimary"`
	ForceHTTPS   *bool  `json:"forceHttps"`
}

type updateRequest struct {
	Hostname     *string `json:"hostname"`
	InternalPort *int    `json:"internalPort"`
	IsPrimary    *bool   `json:"isPrimary"`
	ForceHTTPS   *bool   `json:"forceHttps"`
	DNSStatus    *string `json:"dnsStatus"`
	TLSStatus    *string `json:"tlsStatus"`
}

type domainResponse struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"organizationId"`
	ApplicationID  string        `json:"applicationId"`
	EnvironmentID  string        `json:"environmentId"`
	Hostname       string        `json:"hostname"`
	InternalPort   int           `json:"internalPort"`
	IsPrimary      bool          `json:"isPrimary"`
	ForceHTTPS     bool          `json:"forceHttps"`
	DNSStatus      string        `json:"dnsStatus"`
	TLSStatus      string        `json:"tlsStatus"`
	Routing        RoutingConfig `json:"routing"`
	CreatedAt      string        `json:"createdAt"`
	UpdatedAt      string        `json:"updatedAt"`
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
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	d, err := h.svc.Create(r.Context(), user.ID, CreateInput{
		ApplicationID: appID,
		Hostname:      req.Hostname,
		InternalPort:  req.InternalPort,
		IsPrimary:     req.IsPrimary,
		ForceHTTPS:    req.ForceHTTPS,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"domain": toResponse(d)})
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
	items, err := h.svc.List(r.Context(), user.ID, orgID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]domainResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toResponse(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": out})
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
	items, err := h.svc.ListByApplication(r.Context(), user.ID, appID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]domainResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toResponse(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": out})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndDomain(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	d, err := h.svc.Update(r.Context(), user.ID, id, UpdateInput{
		Hostname:     req.Hostname,
		InternalPort: req.InternalPort,
		IsPrimary:    req.IsPrimary,
		ForceHTTPS:   req.ForceHTTPS,
		DNSStatus:    req.DNSStatus,
		TLSStatus:    req.TLSStatus,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domain": toResponse(d)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndDomain(r)
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

func actorAndDomain(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("domainId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid domain id", nil)
	}
	return user, id, nil
}

func toResponse(d Domain) domainResponse {
	return domainResponse{
		ID:             d.ID.String(),
		OrganizationID: d.OrganizationID.String(),
		ApplicationID:  d.ApplicationID.String(),
		EnvironmentID:  d.EnvironmentID.String(),
		Hostname:       d.Hostname,
		InternalPort:   d.InternalPort,
		IsPrimary:      d.IsPrimary,
		ForceHTTPS:     d.ForceHTTPS,
		DNSStatus:      d.DNSStatus,
		TLSStatus:      d.TLSStatus,
		Routing:        d.Routing,
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      d.UpdatedAt.UTC().Format(time.RFC3339Nano),
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
