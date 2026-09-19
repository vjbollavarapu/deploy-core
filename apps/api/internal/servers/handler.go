package servers

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
	mux.Handle("POST /servers", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /servers", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /servers/capacity", h.auth.RequireAuth(http.HandlerFunc(h.ListCapacity)))
	mux.Handle("POST /placement/preview", h.auth.RequireAuth(http.HandlerFunc(h.PreviewPlacement)))
	mux.Handle("GET /servers/{serverId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("GET /servers/{serverId}/capacity", h.auth.RequireAuth(http.HandlerFunc(h.GetCapacity)))
	mux.Handle("PATCH /servers/{serverId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /servers/{serverId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /servers/{serverId}/maintenance", h.auth.RequireAuth(http.HandlerFunc(h.EnterMaintenance)))
	mux.Handle("DELETE /servers/{serverId}/maintenance", h.auth.RequireAuth(http.HandlerFunc(h.ExitMaintenance)))
}

type createRequest struct {
	OrganizationID  string            `json:"organizationId"`
	Name            string            `json:"name"`
	Provider        string            `json:"provider"`
	Region          string            `json:"region"`
	Hostname        string            `json:"hostname"`
	PublicIP        *string           `json:"publicIp"`
	PrivateIP       *string           `json:"privateIp"`
	Architecture    string            `json:"architecture"`
	OperatingSystem string            `json:"operatingSystem"`
	CPUCores        *int              `json:"cpuCores"`
	MemoryBytes     *int64            `json:"memoryBytes"`
	DiskBytes       *int64            `json:"diskBytes"`
	DockerVersion   *string           `json:"dockerVersion"`
	Labels          map[string]string `json:"labels"`
}

type updateRequest struct {
	Name            *string           `json:"name"`
	Provider        *string           `json:"provider"`
	Region          *string           `json:"region"`
	Hostname        *string           `json:"hostname"`
	PublicIP        *string           `json:"publicIp"`
	PrivateIP       *string           `json:"privateIp"`
	Architecture    *string           `json:"architecture"`
	OperatingSystem *string           `json:"operatingSystem"`
	CPUCores        *int              `json:"cpuCores"`
	MemoryBytes     *int64            `json:"memoryBytes"`
	DiskBytes       *int64            `json:"diskBytes"`
	DockerVersion   *string           `json:"dockerVersion"`
	Labels          map[string]string `json:"labels"`
	Disabled        *bool             `json:"disabled"`
}

type serverResponse struct {
	ID                   string            `json:"id"`
	OrganizationID       string            `json:"organizationId"`
	Name                 string            `json:"name"`
	Provider             string            `json:"provider"`
	Region               string            `json:"region"`
	Hostname             string            `json:"hostname"`
	PublicIP             *string           `json:"publicIp,omitempty"`
	PrivateIP            *string           `json:"privateIp,omitempty"`
	Architecture         string            `json:"architecture"`
	OperatingSystem      string            `json:"operatingSystem"`
	CPUCores             *int              `json:"cpuCores,omitempty"`
	MemoryBytes          *int64            `json:"memoryBytes,omitempty"`
	DiskBytes            *int64            `json:"diskBytes,omitempty"`
	CPUAllocatedMillis   int               `json:"cpuAllocatedMillis"`
	MemoryAllocatedBytes int64             `json:"memoryAllocatedBytes"`
	DiskAllocatedBytes   int64             `json:"diskAllocatedBytes"`
	DockerVersion        *string           `json:"dockerVersion,omitempty"`
	Status               string            `json:"status"`
	MaintenanceMode      bool              `json:"maintenanceMode"`
	LastHeartbeatAt      *string           `json:"lastHeartbeatAt,omitempty"`
	Labels               map[string]string `json:"labels"`
	CreatedBy            *string           `json:"createdBy,omitempty"`
	CreatedAt            string            `json:"createdAt"`
	UpdatedAt            string            `json:"updatedAt"`
}

type capacityResponse struct {
	ServerID             string            `json:"serverId"`
	Name                 string            `json:"name"`
	Status               string            `json:"status"`
	MaintenanceMode      bool              `json:"maintenanceMode"`
	Labels               map[string]string `json:"labels"`
	CPUTotalMillis       int               `json:"cpuTotalMillis"`
	CPUAllocatedMillis   int               `json:"cpuAllocatedMillis"`
	CPUAvailableMillis   int               `json:"cpuAvailableMillis"`
	MemoryTotalBytes     int64             `json:"memoryTotalBytes"`
	MemoryAllocatedBytes int64             `json:"memoryAllocatedBytes"`
	MemoryAvailableBytes int64             `json:"memoryAvailableBytes"`
	DiskTotalBytes       int64             `json:"diskTotalBytes"`
	DiskAllocatedBytes   int64             `json:"diskAllocatedBytes"`
	DiskAvailableBytes   int64             `json:"diskAvailableBytes"`
}

type previewPlacementRequest struct {
	OrganizationID string         `json:"organizationId"`
	CPUMillis      int            `json:"cpuMillis"`
	MemoryBytes    int64          `json:"memoryBytes"`
	DiskBytes      int64          `json:"diskBytes"`
	ForcedServerID *string        `json:"forcedServerId"`
	Policy         map[string]any `json:"policy"`
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
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid organizationId", nil))
		return
	}
	srv, err := h.svc.Create(r.Context(), user.ID, CreateInput{
		OrganizationID:  orgID,
		Name:            strings.TrimSpace(req.Name),
		Provider:        strings.TrimSpace(req.Provider),
		Region:          strings.TrimSpace(req.Region),
		Hostname:        strings.TrimSpace(req.Hostname),
		PublicIP:        req.PublicIP,
		PrivateIP:       req.PrivateIP,
		Architecture:    strings.TrimSpace(req.Architecture),
		OperatingSystem: strings.TrimSpace(req.OperatingSystem),
		CPUCores:        req.CPUCores,
		MemoryBytes:     req.MemoryBytes,
		DiskBytes:       req.DiskBytes,
		DockerVersion:   req.DockerVersion,
		Labels:          req.Labels,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"server": toResponse(srv)})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	orgID, err := uuid.Parse(r.URL.Query().Get("organizationId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId query parameter is required", nil))
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]serverResponse, 0, len(items))
	for _, s := range items {
		out = append(out, toResponse(s))
	}
	writeJSON(w, http.StatusOK, pagination.Page[serverResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	srv, err := h.svc.Get(r.Context(), user.ID, serverID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": toResponse(srv)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	raw := map[string]json.RawMessage{}
	if err := decodeJSON(r, &raw); err != nil {
		writeErr(w, r, err)
		return
	}
	// Reject client-driven operational status fields.
	if _, ok := raw["status"]; ok {
		writeErr(w, r, apierror.Validation("status cannot be set directly; use maintenance endpoints or agent heartbeats", nil))
		return
	}
	if _, ok := raw["maintenanceMode"]; ok {
		writeErr(w, r, apierror.Validation("maintenanceMode cannot be set directly; use maintenance endpoints", nil))
		return
	}

	var req updateRequest
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &req); err != nil {
		writeErr(w, r, apierror.Validation("invalid JSON body", nil))
		return
	}
	in := UpdateInput{
		Name:            trimPtr(req.Name),
		Provider:        trimPtr(req.Provider),
		Region:          trimPtr(req.Region),
		Hostname:        trimPtr(req.Hostname),
		PublicIP:        req.PublicIP,
		PrivateIP:       req.PrivateIP,
		Architecture:    trimPtr(req.Architecture),
		OperatingSystem: trimPtr(req.OperatingSystem),
		CPUCores:        req.CPUCores,
		MemoryBytes:     req.MemoryBytes,
		DiskBytes:       req.DiskBytes,
		DockerVersion:   req.DockerVersion,
		Labels:          req.Labels,
		Disabled:        req.Disabled,
	}
	if v, ok := raw["publicIp"]; ok && string(v) == "null" {
		in.ClearPublicIP = true
		in.PublicIP = nil
	}
	if v, ok := raw["privateIp"]; ok && string(v) == "null" {
		in.ClearPrivateIP = true
		in.PrivateIP = nil
	}

	srv, err := h.svc.Update(r.Context(), user.ID, serverID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": toResponse(srv)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), user.ID, serverID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) EnterMaintenance(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	srv, err := h.svc.EnterMaintenance(r.Context(), user.ID, serverID, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": toResponse(srv)})
}

func (h *Handler) ExitMaintenance(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	srv, err := h.svc.ExitMaintenance(r.Context(), user.ID, serverID, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": toResponse(srv)})
}

func (h *Handler) GetCapacity(w http.ResponseWriter, r *http.Request) {
	user, serverID, err := actorAndServer(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	cap, err := h.svc.GetCapacity(r.Context(), user.ID, serverID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capacity": toCapacityResponse(cap)})
}

func (h *Handler) ListCapacity(w http.ResponseWriter, r *http.Request) {
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
	list, err := h.svc.ListCapacity(r.Context(), user.ID, orgID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	items := make([]capacityResponse, 0, len(list))
	for _, c := range list {
		items = append(items, toCapacityResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) PreviewPlacement(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req previewPlacementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(strings.TrimSpace(req.OrganizationID))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid organizationId", nil))
		return
	}
	in := PreviewPlacementInput{
		OrganizationID: orgID,
		CPUMillis:      req.CPUMillis,
		MemoryBytes:    req.MemoryBytes,
		DiskBytes:      req.DiskBytes,
		Policy:         req.Policy,
	}
	if req.ForcedServerID != nil && strings.TrimSpace(*req.ForcedServerID) != "" {
		sid, err := uuid.Parse(strings.TrimSpace(*req.ForcedServerID))
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid forcedServerId", nil))
			return
		}
		in.ForcedServerID = &sid
	}
	res, err := h.svc.PreviewPlacement(r.Context(), user.ID, in)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"placement": map[string]any{
			"serverId": res.ServerID.String(),
			"score":    res.Score,
			"reason":   res.Reason,
		},
	})
}

func actorAndServer(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("serverId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid server id", nil)
	}
	return user, id, nil
}

func toResponse(s Server) serverResponse {
	out := serverResponse{
		ID:                   s.ID.String(),
		OrganizationID:       s.OrganizationID.String(),
		Name:                 s.Name,
		Provider:             s.Provider,
		Region:               s.Region,
		Hostname:             s.Hostname,
		PublicIP:             s.PublicIP,
		PrivateIP:            s.PrivateIP,
		Architecture:         s.Architecture,
		OperatingSystem:      s.OperatingSystem,
		CPUCores:             s.CPUCores,
		MemoryBytes:          s.MemoryBytes,
		DiskBytes:            s.DiskBytes,
		CPUAllocatedMillis:   s.CPUAllocatedMillis,
		MemoryAllocatedBytes: s.MemoryAllocatedBytes,
		DiskAllocatedBytes:   s.DiskAllocatedBytes,
		DockerVersion:        s.DockerVersion,
		Status:               s.Status,
		MaintenanceMode:      s.MaintenanceMode,
		Labels:               labelsOrEmpty(s.Labels),
		CreatedAt:            s.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:            s.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if s.LastHeartbeatAt != nil {
		v := s.LastHeartbeatAt.UTC().Format(time.RFC3339Nano)
		out.LastHeartbeatAt = &v
	}
	if s.CreatedBy != nil {
		v := s.CreatedBy.String()
		out.CreatedBy = &v
	}
	return out
}

func toCapacityResponse(c CapacityView) capacityResponse {
	return capacityResponse{
		ServerID:             c.ServerID.String(),
		Name:                 c.Name,
		Status:               c.Status,
		MaintenanceMode:      c.MaintenanceMode,
		Labels:               labelsOrEmpty(c.Labels),
		CPUTotalMillis:       c.CPUTotalMillis,
		CPUAllocatedMillis:   c.CPUAllocatedMillis,
		CPUAvailableMillis:   c.CPUAvailableMillis,
		MemoryTotalBytes:     c.MemoryTotalBytes,
		MemoryAllocatedBytes: c.MemoryAllocatedBytes,
		MemoryAvailableBytes: c.MemoryAvailableBytes,
		DiskTotalBytes:       c.DiskTotalBytes,
		DiskAllocatedBytes:   c.DiskAllocatedBytes,
		DiskAvailableBytes:   c.DiskAvailableBytes,
	}
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	return &s
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
