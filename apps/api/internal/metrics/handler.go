package metrics

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
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
	mux.Handle("GET /servers/{serverId}/metrics", h.auth.RequireAuth(http.HandlerFunc(h.GetServer)))
	mux.Handle("GET /servers/{serverId}/metrics/containers", h.auth.RequireAuth(http.HandlerFunc(h.ListContainers)))
	mux.Handle("GET /servers/{serverId}/metrics/series", h.auth.RequireAuth(http.HandlerFunc(h.QuerySeries)))
	mux.Handle("GET /applications/{applicationId}/metrics", h.auth.RequireAuth(http.HandlerFunc(h.GetApplication)))
	mux.Handle("POST /agents/metrics", h.requireAgent(http.HandlerFunc(h.IngestAgent)))
}

func (h *Handler) GetServer(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	snap, err := h.svc.GetServer(r.Context(), user.ID, serverID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": toServerResponse(snap)})
}

func (h *Handler) ListContainers(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items, err := h.svc.ListContainers(r.Context(), user.ID, serverID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]containerResponse, 0, len(items))
	for _, c := range items {
		out = append(out, toContainerResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"containers": out})
}

func (h *Handler) QuerySeries(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	q := RangeQuery{
		Metric:    strings.TrimSpace(r.URL.Query().Get("metric")),
		MaxPoints: 256,
	}
	if v := strings.TrimSpace(r.URL.Query().Get("start")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid start", nil))
			return
		}
		q.Start = t
	}
	if v := strings.TrimSpace(r.URL.Query().Get("end")); v != "" {
		t, err := parseTime(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid end", nil))
			return
		}
		q.End = t
	}
	series, err := h.svc.QuerySeries(r.Context(), user.ID, serverID, q)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]seriesResponse, 0, len(series))
	for _, s := range series {
		pts := make([]pointResponse, 0, len(s.Points))
		for _, p := range s.Points {
			pts = append(pts, pointResponse{
				Timestamp: p.Timestamp.UTC().Format(time.RFC3339Nano),
				Value:     p.Value,
			})
		}
		out = append(out, seriesResponse{Metric: s.Metric, Labels: s.Labels, Points: pts})
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": out})
}

func (h *Handler) GetApplication(w http.ResponseWriter, r *http.Request) {
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
	c, err := h.svc.GetApplication(r.Context(), user.ID, appID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": toContainerResponse(c)})
}

type ingestRequest struct {
	Server     *serverIngestRequest     `json:"server"`
	Containers []containerIngestRequest `json:"containers"`
}

type serverIngestRequest struct {
	CPUPercent       *float64       `json:"cpuPercent"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes"`
	MemoryTotalBytes *int64         `json:"memoryTotalBytes"`
	DiskUsedBytes    *int64         `json:"diskUsedBytes"`
	DiskTotalBytes   *int64         `json:"diskTotalBytes"`
	Load1            *float64       `json:"load1"`
	Load5            *float64       `json:"load5"`
	Load15           *float64       `json:"load15"`
	UptimeSeconds    *int64         `json:"uptimeSeconds"`
	NetworkRxBytes   *int64         `json:"networkRxBytes"`
	NetworkTxBytes   *int64         `json:"networkTxBytes"`
	ContainerCount   *int           `json:"containerCount"`
	RecordedAt       *time.Time     `json:"recordedAt"`
	Payload          map[string]any `json:"payload"`
}

type containerIngestRequest struct {
	ContainerID      string         `json:"containerId"`
	ContainerName    string         `json:"containerName"`
	ApplicationID    *string        `json:"applicationId"`
	CPUPercent       *float64       `json:"cpuPercent"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes"`
	MemoryLimitBytes *int64         `json:"memoryLimitBytes"`
	NetworkRxBytes   *int64         `json:"networkRxBytes"`
	NetworkTxBytes   *int64         `json:"networkTxBytes"`
	RestartCount     *int           `json:"restartCount"`
	Status           string         `json:"status"`
	RecordedAt       *time.Time     `json:"recordedAt"`
	Payload          map[string]any `json:"payload"`
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
	in := AgentIngestInput{}
	if req.Server != nil {
		in.Server = &ServerIngest{
			CPUPercent:       req.Server.CPUPercent,
			MemoryUsedBytes:  req.Server.MemoryUsedBytes,
			MemoryTotalBytes: req.Server.MemoryTotalBytes,
			DiskUsedBytes:    req.Server.DiskUsedBytes,
			DiskTotalBytes:   req.Server.DiskTotalBytes,
			Load1:            req.Server.Load1,
			Load5:            req.Server.Load5,
			Load15:           req.Server.Load15,
			UptimeSeconds:    req.Server.UptimeSeconds,
			NetworkRxBytes:   req.Server.NetworkRxBytes,
			NetworkTxBytes:   req.Server.NetworkTxBytes,
			ContainerCount:   req.Server.ContainerCount,
			RecordedAt:       req.Server.RecordedAt,
			Payload:          req.Server.Payload,
		}
	}
	for _, c := range req.Containers {
		item := ContainerIngest{
			ContainerID:      c.ContainerID,
			ContainerName:    c.ContainerName,
			CPUPercent:       c.CPUPercent,
			MemoryUsedBytes:  c.MemoryUsedBytes,
			MemoryLimitBytes: c.MemoryLimitBytes,
			NetworkRxBytes:   c.NetworkRxBytes,
			NetworkTxBytes:   c.NetworkTxBytes,
			RestartCount:     c.RestartCount,
			Status:           c.Status,
			RecordedAt:       c.RecordedAt,
			Payload:          c.Payload,
		}
		if c.ApplicationID != nil && strings.TrimSpace(*c.ApplicationID) != "" {
			id, err := uuid.Parse(strings.TrimSpace(*c.ApplicationID))
			if err != nil {
				writeErr(w, r, apierror.Validation("invalid applicationId", nil))
				return
			}
			item.ApplicationID = &id
		}
		in.Containers = append(in.Containers, item)
	}
	if err := h.svc.IngestFromAgent(r.Context(), agent, in); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

type serverResponse struct {
	ServerID         string         `json:"serverId"`
	OrganizationID   string         `json:"organizationId"`
	RecordedAt       string         `json:"recordedAt"`
	CPUPercent       *float64       `json:"cpuPercent,omitempty"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes,omitempty"`
	MemoryTotalBytes *int64         `json:"memoryTotalBytes,omitempty"`
	DiskUsedBytes    *int64         `json:"diskUsedBytes,omitempty"`
	DiskTotalBytes   *int64         `json:"diskTotalBytes,omitempty"`
	Load1            *float64       `json:"load1,omitempty"`
	Load5            *float64       `json:"load5,omitempty"`
	Load15           *float64       `json:"load15,omitempty"`
	UptimeSeconds    *int64         `json:"uptimeSeconds,omitempty"`
	NetworkRxBytes   *int64         `json:"networkRxBytes,omitempty"`
	NetworkTxBytes   *int64         `json:"networkTxBytes,omitempty"`
	ContainerCount   *int           `json:"containerCount,omitempty"`
	Source           string         `json:"source"`
	Payload          map[string]any `json:"payload,omitempty"`
	UpdatedAt        string         `json:"updatedAt"`
}

type containerResponse struct {
	ID               string         `json:"id"`
	OrganizationID   string         `json:"organizationId"`
	ServerID         string         `json:"serverId"`
	ApplicationID    *string        `json:"applicationId,omitempty"`
	ContainerID      string         `json:"containerId"`
	ContainerName    string         `json:"containerName"`
	RecordedAt       string         `json:"recordedAt"`
	CPUPercent       *float64       `json:"cpuPercent,omitempty"`
	MemoryUsedBytes  *int64         `json:"memoryUsedBytes,omitempty"`
	MemoryLimitBytes *int64         `json:"memoryLimitBytes,omitempty"`
	NetworkRxBytes   *int64         `json:"networkRxBytes,omitempty"`
	NetworkTxBytes   *int64         `json:"networkTxBytes,omitempty"`
	RestartCount     *int           `json:"restartCount,omitempty"`
	Status           string         `json:"status"`
	Payload          map[string]any `json:"payload,omitempty"`
	UpdatedAt        string         `json:"updatedAt"`
}

type seriesResponse struct {
	Metric string            `json:"metric"`
	Labels map[string]string `json:"labels"`
	Points []pointResponse   `json:"points"`
}

type pointResponse struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

func toServerResponse(s ServerSnapshot) serverResponse {
	return serverResponse{
		ServerID:         s.ServerID.String(),
		OrganizationID:   s.OrganizationID.String(),
		RecordedAt:       s.RecordedAt.UTC().Format(time.RFC3339Nano),
		CPUPercent:       s.CPUPercent,
		MemoryUsedBytes:  s.MemoryUsedBytes,
		MemoryTotalBytes: s.MemoryTotalBytes,
		DiskUsedBytes:    s.DiskUsedBytes,
		DiskTotalBytes:   s.DiskTotalBytes,
		Load1:            s.Load1,
		Load5:            s.Load5,
		Load15:           s.Load15,
		UptimeSeconds:    s.UptimeSeconds,
		NetworkRxBytes:   s.NetworkRxBytes,
		NetworkTxBytes:   s.NetworkTxBytes,
		ContainerCount:   s.ContainerCount,
		Source:           s.Source,
		Payload:          s.Payload,
		UpdatedAt:        s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toContainerResponse(c ContainerSnapshot) containerResponse {
	out := containerResponse{
		ID:               c.ID.String(),
		OrganizationID:   c.OrganizationID.String(),
		ServerID:         c.ServerID.String(),
		ContainerID:      c.ContainerID,
		ContainerName:    c.ContainerName,
		RecordedAt:       c.RecordedAt.UTC().Format(time.RFC3339Nano),
		CPUPercent:       c.CPUPercent,
		MemoryUsedBytes:  c.MemoryUsedBytes,
		MemoryLimitBytes: c.MemoryLimitBytes,
		NetworkRxBytes:   c.NetworkRxBytes,
		NetworkTxBytes:   c.NetworkTxBytes,
		RestartCount:     c.RestartCount,
		Status:           c.Status,
		Payload:          c.Payload,
		UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.ApplicationID != nil {
		s := c.ApplicationID.String()
		out.ApplicationID = &s
	}
	return out
}

func actorAndServer(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	serverID, err := uuid.Parse(r.PathValue("serverId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid server id", nil)
	}
	return user, serverID, nil
}

func parseTime(v string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		t, err = time.Parse(time.RFC3339, v)
	}
	return t.UTC(), err
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
