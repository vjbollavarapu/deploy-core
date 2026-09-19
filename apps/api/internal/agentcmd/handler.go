package agentcmd

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/pagination"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type Handler struct {
	svc          *Service
	auth         *auth.Handler
	requireAgent func(http.Handler) http.Handler
}

func NewHandler(svc *Service, authHandler *auth.Handler, requireAgent func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, auth: authHandler, requireAgent: requireAgent}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("POST /servers/{serverId}/commands", h.auth.RequireAuth(http.HandlerFunc(h.Issue)))
	mux.Handle("GET /servers/{serverId}/commands", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /commands/{commandId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("POST /commands/{commandId}/cancel", h.auth.RequireAuth(http.HandlerFunc(h.Cancel)))

	mux.Handle("GET /agents/commands", h.requireAgent(http.HandlerFunc(h.Poll)))
	mux.Handle("POST /agents/commands/{commandId}/status", h.requireAgent(http.HandlerFunc(h.ReportStatus)))
}

type issueRequest struct {
	Operation     string         `json:"operation"`
	Payload       map[string]any `json:"payload"`
	CorrelationID string         `json:"correlationId"`
	TTLSeconds    *int           `json:"ttlSeconds"`
}

type statusRequest struct {
	Status       string         `json:"status"`
	Result       map[string]any `json:"result"`
	ErrorCode    *string        `json:"errorCode"`
	ErrorMessage *string        `json:"errorMessage"`
}

type commandResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	ServerID       string         `json:"serverId"`
	Operation      string         `json:"operation"`
	SchemaVersion  int            `json:"schemaVersion"`
	Payload        map[string]any `json:"payload"`
	Status         string         `json:"status"`
	IssuedAt       string         `json:"issuedAt"`
	ExpiresAt      string         `json:"expiresAt"`
	RequestID      *string        `json:"requestId,omitempty"`
	CorrelationID  *string        `json:"correlationId,omitempty"`
	IssuedBy       *string        `json:"issuedBy,omitempty"`
	Result         map[string]any `json:"result,omitempty"`
	ErrorCode      *string        `json:"errorCode,omitempty"`
	ErrorMessage   *string        `json:"errorMessage,omitempty"`
	AcceptedAt     *string        `json:"acceptedAt,omitempty"`
	StartedAt      *string        `json:"startedAt,omitempty"`
	FinishedAt     *string        `json:"finishedAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

func (h *Handler) Issue(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	serverID, err := uuid.Parse(r.PathValue("serverId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid server id", nil))
		return
	}
	var req issueRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	var ttl time.Duration
	if req.TTLSeconds != nil && *req.TTLSeconds > 0 {
		ttl = time.Duration(*req.TTLSeconds) * time.Second
	}
	cmd, err := h.svc.Issue(r.Context(), user.ID, IssueInput{
		ServerID:      serverID,
		Operation:     req.Operation,
		Payload:       req.Payload,
		CorrelationID: req.CorrelationID,
		TTL:           ttl,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"command": toResponse(cmd)})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	serverID, err := uuid.Parse(r.PathValue("serverId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid server id", nil))
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListForServer(r.Context(), user.ID, serverID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]commandResponse, 0, len(items))
	for _, c := range items {
		out = append(out, toResponse(c))
	}
	writeJSON(w, http.StatusOK, pagination.Page[commandResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("commandId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid command id", nil))
		return
	}
	cmd, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": toResponse(cmd)})
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("commandId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid command id", nil))
		return
	}
	cmd, err := h.svc.Cancel(r.Context(), user.ID, id, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": toResponse(cmd)})
}

func (h *Handler) Poll(w http.ResponseWriter, r *http.Request) {
	agent, ok := agents.AgentFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	items, err := h.svc.PollPending(r.Context(), agent, 20)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]commandResponse, 0, len(items))
	for _, c := range items {
		out = append(out, toResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schemaVersion": SchemaVersion,
		"commands":      out,
	})
}

func (h *Handler) ReportStatus(w http.ResponseWriter, r *http.Request) {
	agent, ok := agents.AgentFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("commandId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid command id", nil))
		return
	}
	var req statusRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	cmd, err := h.svc.ReportStatus(r.Context(), agent, id, req.Status, req.Result, req.ErrorCode, req.ErrorMessage)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": toResponse(cmd)})
}

func toResponse(c Command) commandResponse {
	out := commandResponse{
		ID:             c.ID.String(),
		OrganizationID: c.OrganizationID.String(),
		ServerID:       c.ServerID.String(),
		Operation:      c.Operation,
		SchemaVersion:  c.SchemaVersion,
		Payload:        c.Payload,
		Status:         c.Status,
		IssuedAt:       c.IssuedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:      c.ExpiresAt.UTC().Format(time.RFC3339Nano),
		RequestID:      c.RequestID,
		CorrelationID:  c.CorrelationID,
		Result:         c.Result,
		ErrorCode:      c.ErrorCode,
		ErrorMessage:   c.ErrorMessage,
		CreatedAt:      c.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.IssuedBy != nil {
		s := c.IssuedBy.String()
		out.IssuedBy = &s
	}
	if c.AcceptedAt != nil {
		s := c.AcceptedAt.UTC().Format(time.RFC3339Nano)
		out.AcceptedAt = &s
	}
	if c.StartedAt != nil {
		s := c.StartedAt.UTC().Format(time.RFC3339Nano)
		out.StartedAt = &s
	}
	if c.FinishedAt != nil {
		s := c.FinishedAt.UTC().Format(time.RFC3339Nano)
		out.FinishedAt = &s
	}
	if out.Payload == nil {
		out.Payload = map[string]any{}
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
