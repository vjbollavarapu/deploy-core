package volumes

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
	mux.Handle("POST /volumes", h.auth.RequireAuth(http.HandlerFunc(h.Create)))
	mux.Handle("GET /volumes", h.auth.RequireAuth(http.HandlerFunc(h.List)))
	mux.Handle("GET /volumes/{volumeId}", h.auth.RequireAuth(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /volumes/{volumeId}", h.auth.RequireAuth(http.HandlerFunc(h.Update)))
	mux.Handle("POST /volumes/{volumeId}/inspect", h.auth.RequireAuth(http.HandlerFunc(h.Inspect)))
	mux.Handle("POST /volumes/{volumeId}/attach", h.auth.RequireAuth(http.HandlerFunc(h.Attach)))
	mux.Handle("POST /volumes/{volumeId}/detach", h.auth.RequireAuth(http.HandlerFunc(h.Detach)))
	mux.Handle("DELETE /volumes/{volumeId}", h.auth.RequireAuth(http.HandlerFunc(h.Delete)))
}

type createRequest struct {
	OrganizationID string         `json:"organizationId"`
	ServerID       string         `json:"serverId"`
	Name           string         `json:"name"`
	Driver         string         `json:"driver"`
	MountPath      string         `json:"mountPath"`
	BackupPolicy   map[string]any `json:"backupPolicy"`
	Protected      bool           `json:"protected"`
	Labels         map[string]any `json:"labels"`
}

type attachRequest struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	MountPath    string `json:"mountPath"`
}

type updateRequest struct {
	MountPath    *string        `json:"mountPath"`
	BackupPolicy map[string]any `json:"backupPolicy"`
	Labels       map[string]any `json:"labels"`
}

type volumeResponse struct {
	ID                   string         `json:"id"`
	OrganizationID       string         `json:"organizationId"`
	ServerID             string         `json:"serverId"`
	Name                 string         `json:"name"`
	Driver               string         `json:"driver"`
	MountPath            string         `json:"mountPath"`
	State                string         `json:"state"`
	AttachedResourceType *string        `json:"attachedResourceType,omitempty"`
	AttachedResourceID   *string        `json:"attachedResourceId,omitempty"`
	BackupPolicy         map[string]any `json:"backupPolicy"`
	Protected            bool           `json:"protected"`
	DockerName           *string        `json:"dockerName,omitempty"`
	UsageBytes           *int64         `json:"usageBytes,omitempty"`
	Labels               map[string]any `json:"labels"`
	LastCommandID        *string        `json:"lastCommandId,omitempty"`
	LastError            string         `json:"lastError,omitempty"`
	CreatedBy            *string        `json:"createdBy,omitempty"`
	CreatedAt            string         `json:"createdAt"`
	UpdatedAt            string         `json:"updatedAt"`
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
	serverID, err := uuid.Parse(strings.TrimSpace(req.ServerID))
	if err != nil {
		writeErr(w, r, apierror.Validation("serverId is required", nil))
		return
	}
	in := CreateInput{
		ServerID:     serverID,
		Name:         req.Name,
		Driver:       req.Driver,
		MountPath:    req.MountPath,
		BackupPolicy: req.BackupPolicy,
		Protected:    req.Protected,
		Labels:       req.Labels,
	}
	if v := strings.TrimSpace(req.OrganizationID); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid organizationId", nil))
			return
		}
		in.OrganizationID = id
	}
	vol, err := h.svc.Create(r.Context(), user.ID, in, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"volume": toResponse(vol)})
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
	var serverID *uuid.UUID
	if v := strings.TrimSpace(r.URL.Query().Get("serverId")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, r, apierror.Validation("invalid serverId", nil))
			return
		}
		serverID = &id
	}
	items, total, err := h.svc.List(r.Context(), user.ID, orgID, serverID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]volumeResponse, 0, len(items))
	for _, v := range items {
		out = append(out, toResponse(v))
	}
	writeJSON(w, http.StatusOK, pagination.Page[volumeResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	vol, err := h.svc.Get(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": toResponse(vol)})
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
	vol, err := h.svc.Update(r.Context(), user.ID, id, UpdateInput{
		MountPath: req.MountPath, BackupPolicy: req.BackupPolicy, Labels: req.Labels,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": toResponse(vol)})
}

func (h *Handler) Inspect(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	vol, err := h.svc.Inspect(r.Context(), user.ID, id, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": toResponse(vol)})
}

func (h *Handler) Attach(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req attachRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	resID, err := uuid.Parse(strings.TrimSpace(req.ResourceID))
	if err != nil {
		writeErr(w, r, apierror.Validation("resourceId is required", nil))
		return
	}
	vol, err := h.svc.Attach(r.Context(), user.ID, id, AttachInput{
		ResourceType: req.ResourceType,
		ResourceID:   resID,
		MountPath:    req.MountPath,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": toResponse(vol)})
}

func (h *Handler) Detach(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndID(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	vol, err := h.svc.Detach(r.Context(), user.ID, id, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"volume": toResponse(vol)})
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
	w.WriteHeader(http.StatusAccepted)
}

func toResponse(v Volume) volumeResponse {
	out := volumeResponse{
		ID:             v.ID.String(),
		OrganizationID: v.OrganizationID.String(),
		ServerID:       v.ServerID.String(),
		Name:           v.Name,
		Driver:         v.Driver,
		MountPath:      v.MountPath,
		State:          v.State,
		BackupPolicy:   v.BackupPolicy,
		Protected:      v.Protected,
		DockerName:     v.DockerName,
		UsageBytes:     v.UsageBytes,
		Labels:         v.Labels,
		LastError:      v.LastError,
		CreatedAt:      v.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      v.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	out.AttachedResourceType = v.AttachedResourceType
	if v.AttachedResourceID != nil {
		s := v.AttachedResourceID.String()
		out.AttachedResourceID = &s
	}
	if v.LastCommandID != nil {
		s := v.LastCommandID.String()
		out.LastCommandID = &s
	}
	if v.CreatedBy != nil {
		s := v.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func actorAndID(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("volumeId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid volume id", nil)
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
