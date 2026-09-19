package replicas

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

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
	mux.Handle("GET /applications/{applicationId}/replicas", h.auth.RequireAuth(http.HandlerFunc(h.GetSummary)))
	mux.Handle("POST /applications/{applicationId}/replicas/scale", h.auth.RequireAuth(http.HandlerFunc(h.Scale)))
	mux.Handle("POST /applications/{applicationId}/replicas/{replicaIndex}/unhealthy", h.auth.RequireAuth(http.HandlerFunc(h.MarkUnhealthy)))
}

type scaleRequest struct {
	DesiredReplicas int `json:"desiredReplicas"`
}

type replicaResponse struct {
	ID             string  `json:"id"`
	ApplicationID  string  `json:"applicationId"`
	RevisionID     *string `json:"revisionId,omitempty"`
	ServerID       *string `json:"serverId,omitempty"`
	ReplicaIndex   int     `json:"replicaIndex"`
	ContainerName  string  `json:"containerName"`
	ContainerID    *string `json:"containerId,omitempty"`
	Status         string  `json:"status"`
	Healthy        bool    `json:"healthy"`
	RoutingEnabled bool    `json:"routingEnabled"`
	LastProbeAt    *string `json:"lastProbeAt,omitempty"`
	LastError      string  `json:"lastError,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type summaryResponse struct {
	ApplicationID    string            `json:"applicationId"`
	DesiredReplicas  int               `json:"desiredReplicas"`
	ObservedReplicas int               `json:"observedReplicas"`
	HealthyReplicas  int               `json:"healthyReplicas"`
	RoutingReplicas  int               `json:"routingReplicas"`
	Replicas         []replicaResponse `json:"replicas"`
}

func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	sum, err := h.svc.GetSummary(r.Context(), user.ID, appID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": toSummary(sum)})
}

func (h *Handler) Scale(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req scaleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	sum, err := h.svc.Scale(r.Context(), user.ID, appID, req.DesiredReplicas, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": toSummary(sum)})
}

func (h *Handler) MarkUnhealthy(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	idx, err := strconv.Atoi(r.PathValue("replicaIndex"))
	if err != nil || idx < 0 {
		writeErr(w, r, apierror.Validation("invalid replicaIndex", nil))
		return
	}
	rep, err := h.svc.MarkUnhealthy(r.Context(), user.ID, appID, idx, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"replica": toReplica(rep)})
}

func actorAndApp(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("applicationId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid application id", nil)
	}
	return user, id, nil
}

func toSummary(s Summary) summaryResponse {
	items := make([]replicaResponse, 0, len(s.Replicas))
	for _, r := range s.Replicas {
		items = append(items, toReplica(r))
	}
	return summaryResponse{
		ApplicationID:    s.ApplicationID.String(),
		DesiredReplicas:  s.DesiredReplicas,
		ObservedReplicas: s.ObservedReplicas,
		HealthyReplicas:  s.HealthyReplicas,
		RoutingReplicas:  s.RoutingReplicas,
		Replicas:         items,
	}
}

func toReplica(r Replica) replicaResponse {
	out := replicaResponse{
		ID:             r.ID.String(),
		ApplicationID:  r.ApplicationID.String(),
		ReplicaIndex:   r.ReplicaIndex,
		ContainerName:  r.ContainerName,
		ContainerID:    r.ContainerID,
		Status:         r.Status,
		Healthy:        r.Healthy,
		RoutingEnabled: r.RoutingEnabled,
		LastError:      r.LastError,
		CreatedAt:      r.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:      r.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if r.RevisionID != nil {
		v := r.RevisionID.String()
		out.RevisionID = &v
	}
	if r.ServerID != nil {
		v := r.ServerID.String()
		out.ServerID = &v
	}
	if r.LastProbeAt != nil {
		v := r.LastProbeAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		out.LastProbeAt = &v
	}
	return out
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

func auditMeta(r *http.Request) AuditMeta {
	return AuditMeta{IP: clientIP(r), UserAgent: r.UserAgent()}
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
