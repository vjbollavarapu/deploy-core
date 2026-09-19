package logs

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type Handler struct {
	svc         *Service
	auth        *auth.Handler
	requireAgent func(http.Handler) http.Handler
}

func NewHandler(svc *Service, authHandler *auth.Handler, requireAgent func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, auth: authHandler, requireAgent: requireAgent}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("GET /applications/{applicationId}/logs", h.auth.RequireAuth(http.HandlerFunc(h.StreamApplication)))
	mux.Handle("GET /deployments/{deploymentId}/logs", h.auth.RequireAuth(http.HandlerFunc(h.StreamDeployment)))
	mux.Handle("GET /deployments/{deploymentId}/events/stream", h.auth.RequireAuth(http.HandlerFunc(h.StreamDeploymentEvents)))
	mux.Handle("POST /agents/logs", h.requireAgent(http.HandlerFunc(h.IngestAgent)))
}

func (h *Handler) StreamApplication(w http.ResponseWriter, r *http.Request) {
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
	req, err := parseStreamQuery(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if req.Kind == "" {
		req.Kind = KindRuntime
	}
	if req.Kind == KindDeploymentEvents {
		writeErr(w, r, apierror.Validation("use /deployments/{id}/events/stream for deployment events", nil))
		return
	}
	req.ApplicationID = &appID
	h.stream(w, r, user.ID, req)
}

func (h *Handler) StreamDeployment(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	depID, err := uuid.Parse(r.PathValue("deploymentId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid deployment id", nil))
		return
	}
	req, err := parseStreamQuery(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if req.Kind == "" {
		req.Kind = KindBuild
	}
	req.DeploymentID = &depID
	h.stream(w, r, user.ID, req)
}

func (h *Handler) StreamDeploymentEvents(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	depID, err := uuid.Parse(r.PathValue("deploymentId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid deployment id", nil))
		return
	}
	req, err := parseStreamQuery(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	req.Kind = KindDeploymentEvents
	req.DeploymentID = &depID
	req.Follow = true
	if v := strings.TrimSpace(r.URL.Query().Get("follow")); v == "0" || strings.EqualFold(v, "false") {
		req.Follow = false
	}
	h.stream(w, r, user.ID, req)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request, actorID uuid.UUID, req StreamRequest) {
	if !req.Follow {
		entries, next, err := h.svc.QueryForUser(r.Context(), actorID, req)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out := make([]entryResponse, 0, len(entries))
		for _, e := range entries {
			out = append(out, toEntryResponse(e))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"entries":    out,
			"nextCursor": next,
			"kind":       req.Kind,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, r, apierror.Internal("streaming not supported"))
		return
	}

	ch, unsubscribe, err := h.svc.SubscribeForUser(r.Context(), actorID, req)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	defer unsubscribe()

	writeSSEHeaders(w)
	w.WriteHeader(http.StatusOK)
	_ = writeSSEEvent(w, flusher, "ready", map[string]any{"kind": req.Kind})

	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if err := writeSSEComment(w, flusher, "keepalive"); err != nil {
				return
			}
		case e, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, flusher, "log", toEntryResponse(e)); err != nil {
				return
			}
		}
	}
}

type ingestRequest struct {
	Kind          string       `json:"kind"`
	ApplicationID string       `json:"applicationId"`
	DeploymentID  *string      `json:"deploymentId"`
	RevisionID    *string      `json:"revisionId"`
	Entries       []IngestLine `json:"entries"`
}

func (h *Handler) IngestAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := agents.AgentFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("agent not authenticated"))
		return
	}
	var req ingestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	appID, err := uuid.Parse(strings.TrimSpace(req.ApplicationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid applicationId", nil))
		return
	}
	in := IngestInput{
		Kind:          req.Kind,
		ApplicationID: appID,
		Entries:       req.Entries,
	}
	if req.DeploymentID != nil && strings.TrimSpace(*req.DeploymentID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.DeploymentID))
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid deploymentId", nil))
			return
		}
		in.DeploymentID = &id
	}
	if req.RevisionID != nil && strings.TrimSpace(*req.RevisionID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.RevisionID))
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid revisionId", nil))
			return
		}
		in.RevisionID = &id
	}
	n, err := h.svc.IngestFromAgent(r.Context(), agent, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": n})
}

func parseStreamQuery(r *http.Request) (StreamRequest, error) {
	q := r.URL.Query()
	req := StreamRequest{
		Kind:   strings.TrimSpace(q.Get("kind")),
		Cursor: strings.TrimSpace(q.Get("cursor")),
		Follow: true,
	}
	if v := strings.TrimSpace(q.Get("follow")); v == "0" || strings.EqualFold(v, "false") {
		req.Follow = false
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return StreamRequest{}, apierror.Validation("invalid limit", nil)
		}
		req.Limit = n
	}
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			t, err = time.Parse(time.RFC3339, v)
		}
		if err != nil {
			return StreamRequest{}, apierror.Validation("invalid since (RFC3339)", nil)
		}
		utc := t.UTC()
		req.Since = &utc
	}
	return req, nil
}

type entryResponse struct {
	Cursor         string  `json:"cursor"`
	Timestamp      string  `json:"timestamp"`
	OrganizationID string  `json:"organizationId"`
	ApplicationID  *string `json:"applicationId,omitempty"`
	DeploymentID   *string `json:"deploymentId,omitempty"`
	RevisionID     *string `json:"revisionId,omitempty"`
	Kind           string  `json:"kind"`
	Stream         string  `json:"stream"`
	Message        string  `json:"message"`
	Sequence       uint64  `json:"sequence,omitempty"`
}

func toEntryResponse(e Entry) entryResponse {
	out := entryResponse{
		Cursor:         e.Cursor,
		Timestamp:      e.Timestamp.UTC().Format(time.RFC3339Nano),
		OrganizationID: e.OrganizationID.String(),
		Kind:           e.Kind,
		Stream:         e.Stream,
		Message:        e.Message,
		Sequence:       e.Sequence,
	}
	out.ApplicationID = uuidPtr(e.ApplicationID)
	out.DeploymentID = uuidPtr(e.DeploymentID)
	out.RevisionID = uuidPtr(e.RevisionID)
	return out
}

func uuidPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
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
