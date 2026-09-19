package webhooks

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
	mux.Handle("POST /integrations/webhooks", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /integrations/webhooks", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /integrations/webhooks/meta", h.auth.RequireAuth(http.HandlerFunc(h.Meta)))
	mux.Handle("POST /integrations/webhooks/emit", h.auth.RequireAuth(http.HandlerFunc(h.Emit)))
	mux.Handle("GET /integrations/webhooks/deliveries", h.auth.RequireAuth(http.HandlerFunc(h.ListDeliveries)))
	mux.Handle("GET /integrations/webhooks/{webhookId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /integrations/webhooks/{webhookId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /integrations/webhooks/{webhookId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
	mux.Handle("GET /integrations/webhooks/{webhookId}/deliveries", h.auth.RequireAuth(http.HandlerFunc(h.ListWebhookDeliveries)))
}

type createRequest struct {
	OrganizationID   string   `json:"organizationId"`
	Name             string   `json:"name"`
	URL              string   `json:"url"`
	Events           []string `json:"events"`
	Secret           string   `json:"secret"`
	Enabled          *bool    `json:"enabled"`
	FailureThreshold *int     `json:"failureThreshold"`
}

type updateRequest struct {
	Name             *string  `json:"name"`
	URL              *string  `json:"url"`
	Events           []string `json:"events"`
	Enabled          *bool    `json:"enabled"`
	FailureThreshold *int     `json:"failureThreshold"`
	RotateSecret     bool     `json:"rotateSecret"`
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

type webhookResponse struct {
	ID                  string   `json:"id"`
	OrganizationID      string   `json:"organizationId"`
	Name                string   `json:"name"`
	URL                 string   `json:"url"`
	Events              []string `json:"events"`
	Enabled             bool     `json:"enabled"`
	Status              string   `json:"status"`
	ConsecutiveFailures int      `json:"consecutiveFailures"`
	FailureThreshold    int      `json:"failureThreshold"`
	LastError           string   `json:"lastError,omitempty"`
	DisabledAt          *string  `json:"disabledAt,omitempty"`
	Secret              *string  `json:"secret,omitempty"`
	CreatedBy           *string  `json:"createdBy,omitempty"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
}

type deliveryResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	WebhookID      string         `json:"webhookId"`
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
	orgID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	wh, err := h.svc.Create(r.Context(), user.ID, CreateInput{
		OrganizationID:   orgID,
		Name:             req.Name,
		URL:              req.URL,
		Events:           req.Events,
		Secret:           req.Secret,
		Enabled:          req.Enabled,
		FailureThreshold: req.FailureThreshold,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"webhook": toWebhookResponse(wh)})
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
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]webhookResponse, 0, len(items))
	for _, wh := range items {
		out = append(out, toWebhookResponse(wh))
	}
	writeJSON(w, http.StatusOK, pagination.Page[webhookResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	wh, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhook": toWebhookResponse(wh)})
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
	wh, err := h.svc.Update(r.Context(), user.ID, id, UpdateInput{
		Name:             req.Name,
		URL:              req.URL,
		Events:           req.Events,
		Enabled:          req.Enabled,
		FailureThreshold: req.FailureThreshold,
		RotateSecret:     req.RotateSecret,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhook": toWebhookResponse(wh)})
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
	items, total, err := h.svc.ListDeliveries(r.Context(), user.ID, orgID, nil, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeDeliveries(w, items, total, page)
}

func (h *Handler) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	wh, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListDeliveries(r.Context(), user.ID, wh.OrganizationID, &id, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeDeliveries(w, items, total, page)
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
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"eventTypes": KnownEvents()})
}

func writeDeliveries(w http.ResponseWriter, items []Delivery, total int64, page pagination.Params) {
	out := make([]deliveryResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toDeliveryResponse(d))
	}
	writeJSON(w, http.StatusOK, pagination.Page[deliveryResponse]{
		Items: out, TotalCount: &total, Limit: page.Limit, Offset: page.Offset,
	})
}

func toWebhookResponse(wh Webhook) webhookResponse {
	out := webhookResponse{
		ID:                  wh.ID.String(),
		OrganizationID:      wh.OrganizationID.String(),
		Name:                wh.Name,
		URL:                 wh.URL,
		Events:              wh.Events,
		Enabled:             wh.Enabled,
		Status:              wh.Status,
		ConsecutiveFailures: wh.ConsecutiveFailures,
		FailureThreshold:    wh.FailureThreshold,
		LastError:           wh.LastError,
		Secret:              wh.SecretPlain,
		CreatedAt:           wh.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:           wh.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if wh.DisabledAt != nil {
		s := wh.DisabledAt.UTC().Format(time.RFC3339Nano)
		out.DisabledAt = &s
	}
	if wh.CreatedBy != nil {
		s := wh.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toDeliveryResponse(d Delivery) deliveryResponse {
	out := deliveryResponse{
		ID:             d.ID.String(),
		OrganizationID: d.OrganizationID.String(),
		WebhookID:      d.WebhookID.String(),
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

func actorAndID(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("webhookId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid webhook id", nil)
	}
	return user, id, nil
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
