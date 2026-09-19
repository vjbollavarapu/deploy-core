package healthchecks

import (
	"encoding/json"
	"net/http"
	"strconv"
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
	mux.Handle("GET /applications/{applicationId}/health", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("GET /applications/{applicationId}/health/samples", h.auth.RequireAuth(http.HandlerFunc(h.ListSamples)))
	mux.Handle("POST /applications/{applicationId}/health/probes", h.auth.RequireAuth(http.HandlerFunc(h.ReportProbe)))
}

type statusResponse struct {
	ApplicationID        string  `json:"applicationId"`
	OrganizationID       string  `json:"organizationId"`
	RevisionID           *string `json:"revisionId,omitempty"`
	DeploymentID         *string `json:"deploymentId,omitempty"`
	State                string  `json:"state"`
	ConsecutiveSuccesses int     `json:"consecutiveSuccesses"`
	ConsecutiveFailures  int     `json:"consecutiveFailures"`
	LastProbeAt          *string `json:"lastProbeAt,omitempty"`
	LastSuccessAt        *string `json:"lastSuccessAt,omitempty"`
	LastFailureAt        *string `json:"lastFailureAt,omitempty"`
	LastMessage          string  `json:"lastMessage"`
	ProbeType            string  `json:"probeType,omitempty"`
	UpdatedAt            string  `json:"updatedAt"`
}

type sampleResponse struct {
	ID           string  `json:"id"`
	Success      bool    `json:"success"`
	ProbeType    string  `json:"probeType"`
	Message      string  `json:"message"`
	LatencyMs    *int    `json:"latencyMs,omitempty"`
	RevisionID   *string `json:"revisionId,omitempty"`
	DeploymentID *string `json:"deploymentId,omitempty"`
	CreatedAt    string  `json:"createdAt"`
}

type probeRequest struct {
	Success      bool    `json:"success"`
	Message      string  `json:"message"`
	LatencyMs    *int    `json:"latencyMs"`
	RevisionID   *string `json:"revisionId"`
	DeploymentID *string `json:"deploymentId"`
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	st, policy, err := h.svc.Get(r.Context(), user.ID, appID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"health": toStatusResponse(st),
		"policy": policy,
	})
}

func (h *Handler) ListSamples(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	limit := MaxSamplesRetained
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	items, err := h.svc.ListSamples(r.Context(), user.ID, appID, limit)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]sampleResponse, 0, len(items))
	for _, s := range items {
		out = append(out, toSampleResponse(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": out})
}

func (h *Handler) ReportProbe(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req probeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	var revID, depID *uuid.UUID
	if req.RevisionID != nil && strings.TrimSpace(*req.RevisionID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.RevisionID))
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid revisionId", nil))
			return
		}
		revID = &id
	}
	if req.DeploymentID != nil && strings.TrimSpace(*req.DeploymentID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.DeploymentID))
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid deploymentId", nil))
			return
		}
		depID = &id
	}
	st, err := h.svc.ReportProbe(r.Context(), user.ID, appID, req.Success, req.Message, req.LatencyMs, revID, depID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"health": toStatusResponse(st)})
}

func actorAndApp(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	appID, err := uuid.Parse(r.PathValue("applicationId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid application id", nil)
	}
	return user, appID, nil
}

func toStatusResponse(st Status) statusResponse {
	out := statusResponse{
		ApplicationID:        st.ApplicationID.String(),
		OrganizationID:       st.OrganizationID.String(),
		State:                st.State,
		ConsecutiveSuccesses: st.ConsecutiveSuccesses,
		ConsecutiveFailures:  st.ConsecutiveFailures,
		LastMessage:          st.LastMessage,
		ProbeType:            st.ProbeType,
		UpdatedAt:            st.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.RevisionID = uuidPtr(st.RevisionID)
	out.DeploymentID = uuidPtr(st.DeploymentID)
	out.LastProbeAt = timePtr(st.LastProbeAt)
	out.LastSuccessAt = timePtr(st.LastSuccessAt)
	out.LastFailureAt = timePtr(st.LastFailureAt)
	return out
}

func toSampleResponse(s Sample) sampleResponse {
	return sampleResponse{
		ID:           s.ID.String(),
		Success:      s.Success,
		ProbeType:    s.ProbeType,
		Message:      s.Message,
		LatencyMs:    s.LatencyMs,
		RevisionID:   uuidPtr(s.RevisionID),
		DeploymentID: uuidPtr(s.DeploymentID),
		CreatedAt:    s.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func uuidPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
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
