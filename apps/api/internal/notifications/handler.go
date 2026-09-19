package notifications

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
	mux.Handle("POST /integrations/notifications/channels", h.auth.RequireAuth(http.HandlerFunc(h.CreateChannel)))
	mux.Handle("GET /integrations/notifications/channels", h.auth.RequireAuth(http.HandlerFunc(h.ListChannels)))
	mux.Handle("GET /integrations/notifications/channels/{channelId}", h.auth.RequireAuth(http.HandlerFunc(h.GetChannel)))
	mux.Handle("PATCH /integrations/notifications/channels/{channelId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdateChannel)))
	mux.Handle("DELETE /integrations/notifications/channels/{channelId}", h.auth.RequireAuth(http.HandlerFunc(h.DeleteChannel)))

	mux.Handle("POST /integrations/notifications/policies", h.auth.RequireAuth(http.HandlerFunc(h.CreatePolicy)))
	mux.Handle("GET /integrations/notifications/policies", h.auth.RequireAuth(http.HandlerFunc(h.ListPolicies)))
	mux.Handle("GET /integrations/notifications/policies/{policyId}", h.auth.RequireAuth(http.HandlerFunc(h.GetPolicy)))
	mux.Handle("PATCH /integrations/notifications/policies/{policyId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdatePolicy)))
	mux.Handle("DELETE /integrations/notifications/policies/{policyId}", h.auth.RequireAuth(http.HandlerFunc(h.DeletePolicy)))

	mux.Handle("GET /integrations/notifications/deliveries", h.auth.RequireAuth(http.HandlerFunc(h.ListDeliveries)))
	mux.Handle("POST /integrations/notifications/emit", h.auth.RequireAuth(http.HandlerFunc(h.Emit)))
	mux.Handle("GET /integrations/notifications/meta", h.auth.RequireAuth(http.HandlerFunc(h.Meta)))
}

type createChannelRequest struct {
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Config         map[string]any `json:"config"`
	Credential     string         `json:"credential"`
	Enabled        *bool          `json:"enabled"`
}

type updateChannelRequest struct {
	Name            *string        `json:"name"`
	Config          map[string]any `json:"config"`
	Credential      *string        `json:"credential"`
	ClearCredential bool           `json:"clearCredential"`
	Enabled         *bool          `json:"enabled"`
	Status          *string        `json:"status"`
}

type createPolicyRequest struct {
	OrganizationID     string         `json:"organizationId"`
	Name               string         `json:"name"`
	EventTypes         []string       `json:"eventTypes"`
	ResourceFilters    map[string]any `json:"resourceFilters"`
	EnvironmentFilters map[string]any `json:"environmentFilters"`
	ChannelIDs         []string       `json:"channelIds"`
	Enabled            *bool          `json:"enabled"`
}

type updatePolicyRequest struct {
	Name               *string        `json:"name"`
	EventTypes         []string       `json:"eventTypes"`
	ResourceFilters    map[string]any `json:"resourceFilters"`
	EnvironmentFilters map[string]any `json:"environmentFilters"`
	ChannelIDs         []string       `json:"channelIds"`
	Enabled            *bool          `json:"enabled"`
}

type emitRequest struct {
	OrganizationID string         `json:"organizationId"`
	EventType      string         `json:"eventType"`
	Payload        map[string]any `json:"payload"`
	ApplicationID  *string        `json:"applicationId"`
	EnvironmentID  *string        `json:"environmentId"`
	ServerID       *string        `json:"serverId"`
	ResourceType   string         `json:"resourceType"`
	ResourceID     *string        `json:"resourceId"`
}

type channelResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Config         map[string]any `json:"config"`
	Enabled        bool           `json:"enabled"`
	Status         string         `json:"status"`
	LastError      string         `json:"lastError,omitempty"`
	HasCredential  bool           `json:"hasCredential"`
	CreatedBy      *string        `json:"createdBy,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

type policyResponse struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organizationId"`
	Name               string         `json:"name"`
	EventTypes         []string       `json:"eventTypes"`
	ResourceFilters    map[string]any `json:"resourceFilters"`
	EnvironmentFilters map[string]any `json:"environmentFilters"`
	ChannelIDs         []string       `json:"channelIds"`
	Enabled            bool           `json:"enabled"`
	CreatedBy          *string        `json:"createdBy,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type deliveryResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	PolicyID       *string        `json:"policyId,omitempty"`
	ChannelID      string         `json:"channelId"`
	EventType      string         `json:"eventType"`
	Payload        map[string]any `json:"payload"`
	Status         string         `json:"status"`
	AttemptCount   int            `json:"attemptCount"`
	JobID          *string        `json:"jobId,omitempty"`
	ResponseCode   *int           `json:"responseCode,omitempty"`
	LatencyMs      *int           `json:"latencyMs,omitempty"`
	LastError      string         `json:"lastError,omitempty"`
	DeliveredAt    *string        `json:"deliveredAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createChannelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	ch, err := h.svc.CreateChannel(r.Context(), user.ID, CreateChannelInput{
		OrganizationID: orgID,
		Name:           req.Name,
		Type:           req.Type,
		Config:         req.Config,
		Credential:     req.Credential,
		Enabled:        req.Enabled,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"channel": toChannelResponse(ch)})
}

func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.svc.ListChannels(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]channelResponse, 0, len(items))
	for _, ch := range items {
		out = append(out, toChannelResponse(ch))
	}
	writeJSON(w, http.StatusOK, pagination.Page[channelResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func (h *Handler) GetChannel(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "channelId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	ch, err := h.svc.GetChannel(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": toChannelResponse(ch)})
}

func (h *Handler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "channelId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateChannelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	ch, err := h.svc.UpdateChannel(r.Context(), user.ID, id, UpdateChannelInput{
		Name:            req.Name,
		Config:          req.Config,
		Credential:      req.Credential,
		ClearCredential: req.ClearCredential,
		Enabled:         req.Enabled,
		Status:          req.Status,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": toChannelResponse(ch)})
}

func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "channelId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.DeleteChannel(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createPolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	channelIDs, err := parseUUIDs(req.ChannelIDs)
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid channelIds", nil))
		return
	}
	p, err := h.svc.CreatePolicy(r.Context(), user.ID, CreatePolicyInput{
		OrganizationID:     orgID,
		Name:               req.Name,
		EventTypes:         req.EventTypes,
		ResourceFilters:    req.ResourceFilters,
		EnvironmentFilters: req.EnvironmentFilters,
		ChannelIDs:         channelIDs,
		Enabled:            req.Enabled,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"policy": toPolicyResponse(p)})
}

func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.svc.ListPolicies(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]policyResponse, 0, len(items))
	for _, p := range items {
		out = append(out, toPolicyResponse(p))
	}
	writeJSON(w, http.StatusOK, pagination.Page[policyResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "policyId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	p, err := h.svc.GetPolicy(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": toPolicyResponse(p)})
}

func (h *Handler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "policyId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updatePolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	var channelIDs []uuid.UUID
	if req.ChannelIDs != nil {
		channelIDs, err = parseUUIDs(req.ChannelIDs)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid channelIds", nil))
			return
		}
	}
	p, err := h.svc.UpdatePolicy(r.Context(), user.ID, id, UpdatePolicyInput{
		Name:               req.Name,
		EventTypes:         req.EventTypes,
		ResourceFilters:    req.ResourceFilters,
		EnvironmentFilters: req.EnvironmentFilters,
		ChannelIDs:         channelIDs,
		Enabled:            req.Enabled,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": toPolicyResponse(p)})
}

func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r, "policyId")
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.DeletePolicy(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.svc.ListDeliveries(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]deliveryResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toDeliveryResponse(d))
	}
	writeJSON(w, http.StatusOK, pagination.Page[deliveryResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func (h *Handler) Emit(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req emitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	in := EmitInput{
		OrganizationID: orgID,
		EventType:      req.EventType,
		Payload:        req.Payload,
		ResourceType:   req.ResourceType,
	}
	if req.ApplicationID != nil {
		id, err := uuid.Parse(*req.ApplicationID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid applicationId", nil))
			return
		}
		in.ApplicationID = &id
	}
	if req.EnvironmentID != nil {
		id, err := uuid.Parse(*req.EnvironmentID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid environmentId", nil))
			return
		}
		in.EnvironmentID = &id
	}
	if req.ServerID != nil {
		id, err := uuid.Parse(*req.ServerID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid serverId", nil))
			return
		}
		in.ServerID = &id
	}
	if req.ResourceID != nil {
		id, err := uuid.Parse(*req.ResourceID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid resourceId", nil))
			return
		}
		in.ResourceID = &id
	}
	n, err := h.svc.EmitForActor(r.Context(), user.ID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"enqueued": n})
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	_ = user
	writeJSON(w, http.StatusOK, map[string]any{
		"enabledChannelTypes":  EnabledChannelTypes(),
		"reservedChannelTypes": ReservedChannelTypes(),
		"eventTypes":           KnownEvents(),
	})
}

func toChannelResponse(ch Channel) channelResponse {
	out := channelResponse{
		ID:             ch.ID.String(),
		OrganizationID: ch.OrganizationID.String(),
		Name:           ch.Name,
		Type:           ch.Type,
		Config:         ch.Config,
		Enabled:        ch.Enabled,
		Status:         ch.Status,
		LastError:      ch.LastError,
		HasCredential:  ch.HasCredential,
		CreatedAt:      ch.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      ch.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if ch.CreatedBy != nil {
		s := ch.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toPolicyResponse(p Policy) policyResponse {
	ids := make([]string, 0, len(p.ChannelIDs))
	for _, id := range p.ChannelIDs {
		ids = append(ids, id.String())
	}
	out := policyResponse{
		ID:                 p.ID.String(),
		OrganizationID:     p.OrganizationID.String(),
		Name:               p.Name,
		EventTypes:         p.EventTypes,
		ResourceFilters:    p.ResourceFilters,
		EnvironmentFilters: p.EnvironmentFilters,
		ChannelIDs:         ids,
		Enabled:            p.Enabled,
		CreatedAt:          p.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:          p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if p.CreatedBy != nil {
		s := p.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toDeliveryResponse(d Delivery) deliveryResponse {
	out := deliveryResponse{
		ID:             d.ID.String(),
		OrganizationID: d.OrganizationID.String(),
		ChannelID:      d.ChannelID.String(),
		EventType:      d.EventType,
		Payload:        d.Payload,
		Status:         d.Status,
		AttemptCount:   d.AttemptCount,
		ResponseCode:   d.ResponseCode,
		LatencyMs:      d.LatencyMs,
		LastError:      d.LastError,
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if d.PolicyID != nil {
		s := d.PolicyID.String()
		out.PolicyID = &s
	}
	if d.JobID != nil {
		s := d.JobID.String()
		out.JobID = &s
	}
	if d.DeliveredAt != nil {
		s := d.DeliveredAt.UTC().Format(time.RFC3339Nano)
		out.DeliveredAt = &s
	}
	return out
}

func actorAndID(r *http.Request, pathKey string) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue(pathKey))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid id", nil)
	}
	return user, id, nil
}

func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(strings.TrimSpace(s))
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
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
