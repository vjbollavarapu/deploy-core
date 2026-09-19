package applications

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
	mux.Handle("POST /applications", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /applications", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /applications/{applicationId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /applications/{applicationId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /applications/{applicationId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
}

type configRequest struct {
	SourceType         string         `json:"sourceType"`
	RepositoryURL      *string        `json:"repositoryUrl"`
	GitBranch          *string        `json:"gitBranch"`
	DockerfilePath     *string        `json:"dockerfilePath"`
	BuildContext       *string        `json:"buildContext"`
	ImageReference     *string        `json:"imageReference"`
	InternalPort       *int           `json:"internalPort"`
	Command            *string        `json:"command"`
	Entrypoint         *string        `json:"entrypoint"`
	CPULimitMillis     *int           `json:"cpuLimitMillis"`
	MemoryLimitBytes   *int64         `json:"memoryLimitBytes"`
	RestartPolicy      string         `json:"restartPolicy"`
	HealthCheck        map[string]any `json:"healthCheck"`
	RuntimeConfig      map[string]any `json:"runtimeConfig"`
	AutoDeployEnabled  *bool          `json:"autoDeployEnabled"`
	GitConnectionID    *string        `json:"gitConnectionId"`
	ClearGitConnection bool           `json:"clearGitConnection"`
}

type createRequest struct {
	OrganizationID  string         `json:"organizationId"`
	ProjectID       string         `json:"projectId"`
	EnvironmentID   string         `json:"environmentId"`
	Name            string         `json:"name"`
	Slug            string         `json:"slug"`
	Type            string         `json:"type"`
	TargetServerID  *string        `json:"targetServerId"`
	PlacementPolicy map[string]any `json:"placementPolicy"`
	Config          configRequest  `json:"config"`
}

type updateRequest struct {
	Name            *string        `json:"name"`
	Slug            *string        `json:"slug"`
	TargetServerID  *string        `json:"targetServerId"`
	PlacementPolicy map[string]any `json:"placementPolicy"`
	Config          *configRequest `json:"config"`
}

type configResponse struct {
	ID                string         `json:"id"`
	Version           int            `json:"version"`
	SourceType        string         `json:"sourceType"`
	RepositoryURL     *string        `json:"repositoryUrl,omitempty"`
	GitBranch         *string        `json:"gitBranch,omitempty"`
	DockerfilePath    *string        `json:"dockerfilePath,omitempty"`
	BuildContext      *string        `json:"buildContext,omitempty"`
	ImageReference    *string        `json:"imageReference,omitempty"`
	InternalPort      *int           `json:"internalPort,omitempty"`
	Command           *string        `json:"command,omitempty"`
	Entrypoint        *string        `json:"entrypoint,omitempty"`
	CPULimitMillis    *int           `json:"cpuLimitMillis,omitempty"`
	MemoryLimitBytes  *int64         `json:"memoryLimitBytes,omitempty"`
	RestartPolicy     string         `json:"restartPolicy"`
	HealthCheck       map[string]any `json:"healthCheck"`
	RuntimeConfig     map[string]any `json:"runtimeConfig"`
	AutoDeployEnabled bool           `json:"autoDeployEnabled"`
	GitConnectionID   *string        `json:"gitConnectionId,omitempty"`
	CreatedAt         string         `json:"createdAt"`
	UpdatedAt         string         `json:"updatedAt"`
}

type applicationResponse struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organizationId"`
	ProjectID       string         `json:"projectId"`
	EnvironmentID   string         `json:"environmentId"`
	Name            string         `json:"name"`
	Slug            string         `json:"slug"`
	Type            string         `json:"type"`
	Status          string         `json:"status"`
	TargetServerID  *string        `json:"targetServerId,omitempty"`
	PlacementPolicy map[string]any `json:"placementPolicy"`
	CreatedBy       *string        `json:"createdBy,omitempty"`
	CreatedAt       string         `json:"createdAt"`
	UpdatedAt       string         `json:"updatedAt"`
	Config          configResponse `json:"config"`
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
	envID, err := uuid.Parse(req.EnvironmentID)
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid environmentId", nil))
		return
	}
	in := CreateInput{
		EnvironmentID:   envID,
		Name:            req.Name,
		Slug:            req.Slug,
		Type:            req.Type,
		PlacementPolicy: req.PlacementPolicy,
		Config:          toConfigInput(req.Config),
	}
	if req.ProjectID != "" {
		pid, err := uuid.Parse(req.ProjectID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid projectId", nil))
			return
		}
		in.ProjectID = pid
	}
	if req.OrganizationID != "" {
		oid, err := uuid.Parse(req.OrganizationID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid organizationId", nil))
			return
		}
		in.OrganizationID = oid
	}
	if req.TargetServerID != nil && strings.TrimSpace(*req.TargetServerID) != "" {
		sid, err := uuid.Parse(*req.TargetServerID)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid targetServerId", nil))
			return
		}
		in.TargetServerID = &sid
	}
	app, err := h.svc.Create(r.Context(), user.ID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"application": toResponse(app)})
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
	var projectID, environmentID *uuid.UUID
	if v := r.URL.Query().Get("projectId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid projectId", nil))
			return
		}
		projectID = &id
	}
	if v := r.URL.Query().Get("environmentId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid environmentId", nil))
			return
		}
		environmentID = &id
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, projectID, environmentID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]applicationResponse, 0, len(items))
	for _, a := range items {
		out = append(out, toResponse(a))
	}
	writeJSON(w, http.StatusOK, pagination.Page[applicationResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	app, err := h.svc.Get(r.Context(), user.ID, appID)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": toResponse(app)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	in := UpdateInput{
		Name:            req.Name,
		Slug:            req.Slug,
		PlacementPolicy: req.PlacementPolicy,
	}
	if req.Config != nil {
		cfg := toConfigInput(*req.Config)
		in.Config = &cfg
	}
	if req.TargetServerID != nil {
		if strings.TrimSpace(*req.TargetServerID) == "" {
			in.ClearTarget = true
		} else {
			sid, err := uuid.Parse(*req.TargetServerID)
			if err != nil {
				writeErr(w, r, apierror.Validation("invalid targetServerId", nil))
				return
			}
			in.TargetServerID = &sid
		}
	}
	app, err := h.svc.Update(r.Context(), user.ID, appID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": toResponse(app)})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, appID, err := actorAndApp(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), user.ID, appID, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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

func toConfigInput(req configRequest) ConfigInput {
	in := ConfigInput{
		SourceType:         req.SourceType,
		RepositoryURL:      req.RepositoryURL,
		GitBranch:          req.GitBranch,
		DockerfilePath:     req.DockerfilePath,
		BuildContext:       req.BuildContext,
		ImageReference:     req.ImageReference,
		InternalPort:       req.InternalPort,
		Command:            req.Command,
		Entrypoint:         req.Entrypoint,
		CPULimitMillis:     req.CPULimitMillis,
		MemoryLimitBytes:   req.MemoryLimitBytes,
		RestartPolicy:      req.RestartPolicy,
		HealthCheck:        req.HealthCheck,
		RuntimeConfig:      req.RuntimeConfig,
		AutoDeployEnabled:  req.AutoDeployEnabled,
		ClearGitConnection: req.ClearGitConnection,
	}
	if req.GitConnectionID != nil && strings.TrimSpace(*req.GitConnectionID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*req.GitConnectionID))
		if err == nil {
			in.GitConnectionID = &id
		}
	}
	return in
}

func toResponse(a Application) applicationResponse {
	out := applicationResponse{
		ID:              a.ID.String(),
		OrganizationID:  a.OrganizationID.String(),
		ProjectID:       a.ProjectID.String(),
		EnvironmentID:   a.EnvironmentID.String(),
		Name:            a.Name,
		Slug:            a.Slug,
		Type:            a.Type,
		Status:          a.Status,
		PlacementPolicy: mapOrEmpty(a.PlacementPolicy),
		CreatedAt:       a.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:       a.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Config:          toConfigResponse(a.Config),
	}
	if a.TargetServerID != nil {
		s := a.TargetServerID.String()
		out.TargetServerID = &s
	}
	if a.CreatedBy != nil {
		s := a.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toConfigResponse(c Config) configResponse {
	out := configResponse{
		ID:                c.ID.String(),
		Version:           c.Version,
		SourceType:        c.SourceType,
		RepositoryURL:     c.RepositoryURL,
		GitBranch:         c.GitBranch,
		DockerfilePath:    c.DockerfilePath,
		BuildContext:      c.BuildContext,
		ImageReference:    c.ImageReference,
		InternalPort:      c.InternalPort,
		Command:           c.Command,
		Entrypoint:        c.Entrypoint,
		CPULimitMillis:    c.CPULimitMillis,
		MemoryLimitBytes:  c.MemoryLimitBytes,
		RestartPolicy:     c.RestartPolicy,
		HealthCheck:       mapOrEmpty(c.HealthCheck),
		RuntimeConfig:     mapOrEmpty(c.RuntimeConfig),
		AutoDeployEnabled: c.AutoDeployEnabled,
		CreatedAt:         c.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:         c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.GitConnectionID != nil {
		s := c.GitConnectionID.String()
		out.GitConnectionID = &s
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
