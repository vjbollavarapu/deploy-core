package databases

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
	mux.Handle("POST /databases", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /databases", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /databases/{databaseId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /databases/{databaseId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /databases/{databaseId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /databases/{databaseId}/credentials/reveal", h.auth.RequireAuth(http.HandlerFunc(h.Reveal)))
	mux.Handle("GET /agents/databases/{databaseId}/bootstrap", h.requireAgent(http.HandlerFunc(h.Bootstrap)))
}

type createRequest struct {
	OrganizationID string         `json:"organizationId"`
	ProjectID      string         `json:"projectId"`
	EnvironmentID  string         `json:"environmentId"`
	ServerID       string         `json:"serverId"`
	Name           string         `json:"name"`
	Engine         string         `json:"engine"`
	EngineVersion  string         `json:"engineVersion"`
	DatabaseName   string         `json:"databaseName"`
	Username       string         `json:"username"`
	Password       string         `json:"password"`
	StorageVolume  string         `json:"storageVolume"`
	CPUMillis      *int           `json:"cpuMillis"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	BackupPolicy   map[string]any `json:"backupPolicy"`
}

type updateRequest struct {
	Name           *string        `json:"name"`
	CPUMillis      *int           `json:"cpuMillis"`
	MemoryBytes    *int64         `json:"memoryBytes"`
	BackupPolicy   map[string]any `json:"backupPolicy"`
	Status         *string        `json:"status"`
	RotatePassword *string        `json:"rotatePassword"`
}

type databaseResponse struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organizationId"`
	ProjectID          string         `json:"projectId"`
	EnvironmentID      string         `json:"environmentId"`
	ServerID           string         `json:"serverId"`
	Name               string         `json:"name"`
	Engine             string         `json:"engine"`
	EngineVersion      string         `json:"engineVersion"`
	DatabaseName       string         `json:"databaseName"`
	Username           string         `json:"username"`
	StorageVolumeName  string         `json:"storageVolumeName"`
	VolumeProtected    bool           `json:"volumeProtected"`
	CPUMillis          *int           `json:"cpuMillis,omitempty"`
	MemoryBytes        *int64         `json:"memoryBytes,omitempty"`
	ContainerRuntimeID *string        `json:"containerRuntimeId,omitempty"`
	Status             string         `json:"status"`
	BackupPolicy       map[string]any `json:"backupPolicy"`
	ProvisionCommandID *string        `json:"provisionCommandId,omitempty"`
	LastError          string         `json:"lastError,omitempty"`
	HasCredential      bool           `json:"hasCredential"`
	CreatedBy          *string        `json:"createdBy,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
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
	envID, err := uuid.Parse(strings.TrimSpace(req.EnvironmentID))
	if err != nil {
		writeErr(w, r, apierror.Validation("environmentId is required", nil))
		return
	}
	serverID, err := uuid.Parse(strings.TrimSpace(req.ServerID))
	if err != nil {
		writeErr(w, r, apierror.Validation("serverId is required", nil))
		return
	}
	in := CreateInput{
		EnvironmentID: envID,
		ServerID:      serverID,
		Name:          req.Name,
		Engine:        req.Engine,
		EngineVersion: req.EngineVersion,
		DatabaseName:  req.DatabaseName,
		Username:      req.Username,
		Password:      req.Password,
		StorageVolume: req.StorageVolume,
		CPUMillis:     req.CPUMillis,
		MemoryBytes:   req.MemoryBytes,
		BackupPolicy:  req.BackupPolicy,
	}
	if v := strings.TrimSpace(req.OrganizationID); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid organizationId", nil))
			return
		}
		in.OrganizationID = id
	}
	if v := strings.TrimSpace(req.ProjectID); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid projectId", nil))
			return
		}
		in.ProjectID = id
	}
	d, err := h.svc.Create(r.Context(), user.ID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"database": toResponse(d)})
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
	var projectID, envID, serverID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("projectId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid projectId", nil))
			return
		}
		projectID = &id
	}
	if v := strings.TrimSpace(r.URL.Query().Get("environmentId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid environmentId", nil))
			return
		}
		envID = &id
	}
	if v := strings.TrimSpace(r.URL.Query().Get("serverId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid serverId", nil))
			return
		}
		serverID = &id
	}
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, projectID, envID, serverID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]databaseResponse, 0, len(items))
	for _, d := range items {
		out = append(out, toResponse(d))
	}
	writeJSON(w, http.StatusOK, pagination.Page[databaseResponse]{
		Items:      out,
		Limit:      page.Limit,
		Offset:     page.Offset,
		TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	d, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": toResponse(d)})
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
	d, err := h.svc.Update(r.Context(), user.ID, id, UpdateInput{
		Name:           req.Name,
		CPUMillis:      req.CPUMillis,
		MemoryBytes:    req.MemoryBytes,
		BackupPolicy:   req.BackupPolicy,
		Status:         req.Status,
		RotatePassword: req.RotatePassword,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": toResponse(d)})
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

func (h *Handler) Reveal(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	password, d, err := h.svc.RevealPassword(r.Context(), user.ID, id, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"databaseId":   d.ID.String(),
		"username":     d.Username,
		"databaseName": d.DatabaseName,
		"password":     password,
	})
}

func (h *Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	agent, ok := agents.AgentFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("agent not authenticated"))
		return
	}
	id, err := uuid.Parse(r.PathValue("databaseId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid database id", nil))
		return
	}
	boot, err := h.svc.BootstrapForAgent(r.Context(), agent, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"databaseId":        boot.DatabaseID.String(),
		"engine":            boot.Engine,
		"engineVersion":     boot.EngineVersion,
		"databaseName":      boot.DatabaseName,
		"username":          boot.Username,
		"password":          boot.Password,
		"storageVolumeName": boot.StorageVolumeName,
		"cpuMillis":         boot.CPUMillis,
		"memoryBytes":       boot.MemoryBytes,
		"volumeProtected":   true,
		"note":              "Persistent volume must outlive container recreation; do not delete volume on container replace.",
	})
}

func toResponse(d Database) databaseResponse {
	out := databaseResponse{
		ID:                d.ID.String(),
		OrganizationID:    d.OrganizationID.String(),
		ProjectID:         d.ProjectID.String(),
		EnvironmentID:     d.EnvironmentID.String(),
		ServerID:          d.ServerID.String(),
		Name:              d.Name,
		Engine:            d.Engine,
		EngineVersion:     d.EngineVersion,
		DatabaseName:      d.DatabaseName,
		Username:          d.Username,
		StorageVolumeName: d.StorageVolumeName,
		VolumeProtected:   d.VolumeProtected,
		CPUMillis:         d.CPUMillis,
		MemoryBytes:       d.MemoryBytes,
		ContainerRuntimeID: d.ContainerRuntimeID,
		Status:            d.Status,
		BackupPolicy:      d.BackupPolicy,
		LastError:         d.LastError,
		HasCredential:     d.HasCredential,
		CreatedAt:         d.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:         d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if d.ProvisionCommandID != nil {
		s := d.ProvisionCommandID.String()
		out.ProvisionCommandID = &s
	}
	if d.CreatedBy != nil {
		s := d.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func actorAndID(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("databaseId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid database id", nil)
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
