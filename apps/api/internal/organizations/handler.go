package organizations

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
	mux.Handle("POST /organizations", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /organizations", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /organizations/{orgId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /organizations/{orgId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))

	mux.Handle("GET /organizations/{orgId}/members", h.auth.RequireAuth(http.HandlerFunc(h.ListMembers)))
	mux.Handle("PATCH /organizations/{orgId}/members/{memberId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdateMemberRoles)))
	mux.Handle("DELETE /organizations/{orgId}/members/{memberId}", h.auth.RequireAuth(http.HandlerFunc(h.RemoveMember)))

	mux.Handle("POST /organizations/{orgId}/invitations", h.auth.RequireAuth(http.HandlerFunc(h.Invite)))
	mux.Handle("POST /invitations/accept", h.auth.RequireAuth(http.HandlerFunc(h.AcceptInvitation)))
}

type createOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type updateOrgRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

type inviteRequest struct {
	Email    string   `json:"email"`
	RoleKeys []string `json:"roleKeys"`
}

type updateRolesRequest struct {
	RoleKeys []string `json:"roleKeys"`
}

type acceptInviteRequest struct {
	Token string `json:"token"`
}

type orgResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Slug      string  `json:"slug"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	CreatedBy *string `json:"createdBy,omitempty"`
}

type roleResponse struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type memberResponse struct {
	ID          string         `json:"id"`
	UserID      string         `json:"userId"`
	Email       string         `json:"email"`
	DisplayName string         `json:"displayName"`
	Status      string         `json:"status"`
	Roles       []roleResponse `json:"roles"`
	JoinedAt    *string        `json:"joinedAt,omitempty"`
	CreatedAt   string         `json:"createdAt"`
}

type invitationResponse struct {
	ID        string         `json:"id"`
	Email     string         `json:"email"`
	Roles     []roleResponse `json:"roles"`
	ExpiresAt string         `json:"expiresAt"`
	CreatedAt string         `json:"createdAt"`
	Token     string         `json:"token,omitempty"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createOrgRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	org, err := h.svc.Create(r.Context(), user.ID, req.Name, req.Slug, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"organization": toOrgResponse(org)})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListForUser(r.Context(), user.ID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]orgResponse, 0, len(items))
	for _, org := range items {
		out = append(out, toOrgResponse(org))
	}
	writeJSON(w, http.StatusOK, pagination.Page[orgResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	org, err := h.svc.Get(r.Context(), user.ID, orgID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organization": toOrgResponse(org)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateOrgRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	org, err := h.svc.Update(r.Context(), user.ID, orgID, req.Name, req.Slug, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organization": toOrgResponse(org)})
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListMembers(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]memberResponse, 0, len(items))
	for _, m := range items {
		out = append(out, toMemberResponse(m))
	}
	writeJSON(w, http.StatusOK, pagination.Page[memberResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req inviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	inv, err := h.svc.Invite(r.Context(), user.ID, orgID, req.Email, req.RoleKeys, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	resp := toInvitationResponse(inv)
	// Token returned only so clients/tests can complete accept without email infra.
	resp.Token = inv.RawToken
	writeJSON(w, http.StatusCreated, map[string]any{"invitation": resp})
}

func (h *Handler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req acceptInviteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	mem, err := h.svc.AcceptInvitation(r.Context(), user.ID, user.Email, req.Token, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": toMemberResponse(mem)})
}

func (h *Handler) UpdateMemberRoles(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	memberID, err := uuid.Parse(r.PathValue("memberId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid member id", nil))
		return
	}
	var req updateRolesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	mem, err := h.svc.UpdateMemberRoles(r.Context(), user.ID, orgID, memberID, req.RoleKeys, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": toMemberResponse(mem)})
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	user, orgID, err := actorAndOrg(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	memberID, err := uuid.Parse(r.PathValue("memberId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid member id", nil))
		return
	}
	if err := h.svc.RemoveMember(r.Context(), user.ID, orgID, memberID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func actorAndOrg(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	orgID, err := uuid.Parse(r.PathValue("orgId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid organization id", nil)
	}
	return user, orgID, nil
}

func toOrgResponse(o Organization) orgResponse {
	out := orgResponse{
		ID:        o.ID.String(),
		Name:      o.Name,
		Slug:      o.Slug,
		Status:    o.Status,
		CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt: o.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if o.CreatedBy != nil {
		s := o.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toMemberResponse(m Member) memberResponse {
	out := memberResponse{
		ID:          m.ID.String(),
		UserID:      m.UserID.String(),
		Email:       m.Email,
		DisplayName: m.DisplayName,
		Status:      m.Status,
		Roles:       toRoleResponses(m.Roles),
		CreatedAt:   m.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if m.JoinedAt != nil {
		s := m.JoinedAt.UTC().Format(time.RFC3339Nano)
		out.JoinedAt = &s
	}
	return out
}

func toInvitationResponse(inv Invitation) invitationResponse {
	return invitationResponse{
		ID:        inv.ID.String(),
		Email:     inv.Email,
		Roles:     toRoleResponses(inv.Roles),
		ExpiresAt: inv.ExpiresAt.UTC().Format(time.RFC3339Nano),
		CreatedAt: inv.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toRoleResponses(roles []Role) []roleResponse {
	out := make([]roleResponse, 0, len(roles))
	for _, role := range roles {
		out = append(out, roleResponse{Key: role.Key, Name: role.Name, Description: role.Description})
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
