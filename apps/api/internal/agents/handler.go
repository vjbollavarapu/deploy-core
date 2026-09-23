package agents

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

type Handler struct {
	svc     *Service
	auth    *auth.Handler
	limiter *security.RateLimiter
}

func NewHandler(svc *Service, authHandler *auth.Handler, limiter *security.RateLimiter) *Handler {
	if limiter == nil {
		limiter = security.NewRateLimiter(30, time.Minute)
	}
	return &Handler{svc: svc, auth: authHandler, limiter: limiter}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("POST /servers/{serverId}/registration-token", h.auth.RequireAuth(http.HandlerFunc(h.IssueRegistrationToken)))
	mux.Handle("DELETE /servers/{serverId}/registration-token", h.auth.RequireAuth(http.HandlerFunc(h.RevokeRegistrationToken)))
	mux.Handle("DELETE /servers/{serverId}/agent-credential", h.auth.RequireAuth(http.HandlerFunc(h.RevokeAgentCredential)))
	mux.HandleFunc("POST /agents/register", h.Register)
	mux.Handle("POST /agents/heartbeat", h.RequireAgent(http.HandlerFunc(h.Heartbeat)))
}

func (h *Handler) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		agent, err := h.svc.AuthenticateCredential(r.Context(), raw)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAgent(r.Context(), agent)))
	})
}

// Types replaced by protocol-go

func (h *Handler) IssueRegistrationToken(w http.ResponseWriter, r *http.Request) {
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
	res, err := h.svc.IssueRegistrationToken(r.Context(), user.ID, serverID, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"registrationToken": map[string]any{
			"serverId":  res.ServerID.String(),
			"agentId":   res.AgentID.String(),
			"token":     res.Token,
			"expiresAt": res.ExpiresAt.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (h *Handler) RevokeRegistrationToken(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.RevokeRegistrationToken(r.Context(), user.ID, serverID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) RevokeAgentCredential(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.RevokeAgentCredential(r.Context(), user.ID, serverID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow("agent-register:"+clientIP(r), time.Now()) {
		writeErr(w, r, apierror.New(http.StatusTooManyRequests, apierror.CodeRateLimited, "rate limit exceeded"))
		return
	}
	var req protocol.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	res, err := h.svc.Register(r.Context(), req.RegistrationToken, req.AgentVersion, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"agent": protocol.RegisterResult{
			AgentID:    res.AgentID.String(),
			ServerID:   res.ServerID.String(),
			Credential: res.Credential,
			TokenType:  "Bearer",
		},
	})
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	agent, ok := AgentFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req protocol.HeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.Heartbeat(r.Context(), agent, HeartbeatInput{
		Timestamp:         req.Timestamp,
		AgentVersion:      strings.TrimSpace(req.AgentVersion),
		DockerStatus:      strings.TrimSpace(req.DockerStatus),
		CPUPercent:        req.CPUPercent,
		MemoryUsedBytes:   req.MemoryUsedBytes,
		DiskUsedBytes:     req.DiskUsedBytes,
		Load1:             req.Load1,
		ContainerCount:    req.ContainerCount,
		UptimeSeconds:     req.UptimeSeconds,
		DockerVersion:     req.DockerVersion,
		Hostname:          strings.TrimSpace(req.Hostname),
		OS:                strings.TrimSpace(req.OS),
		Architecture:      strings.TrimSpace(req.Architecture),
		CPUCores:          req.CPUCores,
		MemoryTotalBytes:  req.MemoryTotalBytes,
		DiskTotalBytes:    req.DiskTotalBytes,
		RunningContainers: req.RunningContainers,
		ImageCount:        req.ImageCount,
		VolumeCount:       req.VolumeCount,
		NetworkCount:      req.NetworkCount,
		AgentState:        strings.TrimSpace(req.AgentState),
	}); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
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
