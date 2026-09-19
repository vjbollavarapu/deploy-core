package gitproviders

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/pagination"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/google/uuid"
)

type Handler struct {
	svc     *Service
	auth    *auth.Handler
	limiter *security.RateLimiter
}

func NewHandler(svc *Service, authHandler *auth.Handler, limiter *security.RateLimiter) *Handler {
	if limiter == nil {
		limiter = security.NewRateLimiter(120, time.Minute)
	}
	return &Handler{svc: svc, auth: authHandler, limiter: limiter}
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("POST /integrations/git/connections", h.auth.RequireAuth(http.HandlerFunc(h.CreateConnection)))
	mux.Handle("GET /integrations/git/connections", h.auth.RequireAuth(http.HandlerFunc(h.ListConnections)))
	mux.Handle("GET /integrations/git/connections/{connectionId}", h.auth.RequireAuth(http.HandlerFunc(h.GetConnection)))
	mux.Handle("PATCH /integrations/git/connections/{connectionId}", h.auth.RequireAuth(http.HandlerFunc(h.UpdateConnection)))
	mux.Handle("DELETE /integrations/git/connections/{connectionId}", h.auth.RequireAuth(http.HandlerFunc(h.DeleteConnection)))
	mux.Handle("POST /integrations/git/connections/{connectionId}/sync", h.auth.RequireAuth(http.HandlerFunc(h.SyncConnection)))
	mux.Handle("GET /integrations/git/connections/{connectionId}/repositories", h.auth.RequireAuth(http.HandlerFunc(h.ListRepositories)))
	// Public webhook receiver — authenticated via provider signature.
	mux.HandleFunc("POST /webhooks/git/{connectionId}", h.Webhook)
}

type createConnectionRequest struct {
	OrganizationID string         `json:"organizationId"`
	Provider       string         `json:"provider"`
	AccountLogin   string         `json:"accountLogin"`
	DisplayName    string         `json:"displayName"`
	AccessToken    string         `json:"accessToken"`
	WebhookSecret  *string        `json:"webhookSecret"`
	Metadata       map[string]any `json:"metadata"`
}

type updateConnectionRequest struct {
	AccountLogin  *string        `json:"accountLogin"`
	DisplayName   *string        `json:"displayName"`
	Status        *string        `json:"status"`
	AccessToken   *string        `json:"accessToken"`
	WebhookSecret *string        `json:"webhookSecret"`
	Metadata      map[string]any `json:"metadata"`
}

type syncRequest struct {
	Repositories []repoUpsertRequest `json:"repositories"`
}

type repoUpsertRequest struct {
	ExternalID    string         `json:"externalId"`
	FullName      string         `json:"fullName"`
	DefaultBranch string         `json:"defaultBranch"`
	CloneURL      string         `json:"cloneUrl"`
	HTMLURL       string         `json:"htmlUrl"`
	Metadata      map[string]any `json:"metadata"`
}

type connectionResponse struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organizationId"`
	Provider           string         `json:"provider"`
	AccountLogin       string         `json:"accountLogin"`
	DisplayName        string         `json:"displayName"`
	Status             string         `json:"status"`
	LastSyncAt         *string        `json:"lastSyncAt,omitempty"`
	HasWebhookSecret   bool           `json:"hasWebhookSecret"`
	WebhookSecret      *string        `json:"webhookSecret,omitempty"`
	WebhookURLHint     string         `json:"webhookUrlHint,omitempty"`
	Metadata           map[string]any `json:"metadata"`
	CreatedBy          *string        `json:"createdBy,omitempty"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type repositoryResponse struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organizationId"`
	ConnectionID   string         `json:"connectionId"`
	ExternalID     string         `json:"externalId"`
	FullName       string         `json:"fullName"`
	DefaultBranch  string         `json:"defaultBranch"`
	CloneURL       string         `json:"cloneUrl"`
	HTMLURL        string         `json:"htmlUrl"`
	Metadata       map[string]any `json:"metadata"`
	LastSyncAt     *string        `json:"lastSyncAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
	UpdatedAt      string         `json:"updatedAt"`
}

func (h *Handler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeErr(w, r, apierror.Unauthorized("not authenticated"))
		return
	}
	var req createConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	orgID, err := uuid.Parse(req.OrganizationID)
	if err != nil {
		writeErr(w, r, apierror.Validation("organizationId is required", nil))
		return
	}
	c, err := h.svc.CreateConnection(r.Context(), user.ID, CreateConnectionInput{
		OrganizationID: orgID,
		Provider:       req.Provider,
		AccountLogin:   req.AccountLogin,
		DisplayName:    req.DisplayName,
		AccessToken:    req.AccessToken,
		WebhookSecret:  req.WebhookSecret,
		Metadata:       req.Metadata,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"connection": toConnectionResponse(c)})
}

func (h *Handler) ListConnections(w http.ResponseWriter, r *http.Request) {
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
	items, total, err := h.svc.ListConnections(r.Context(), user.ID, orgID, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]connectionResponse, 0, len(items))
	for _, c := range items {
		out = append(out, toConnectionResponse(c))
	}
	writeJSON(w, http.StatusOK, pagination.Page[connectionResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) GetConnection(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndConnection(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	c, err := h.svc.GetConnection(r.Context(), user.ID, id)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": toConnectionResponse(c)})
}

func (h *Handler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndConnection(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req updateConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	c, err := h.svc.UpdateConnection(r.Context(), user.ID, id, UpdateConnectionInput{
		AccountLogin:  req.AccountLogin,
		DisplayName:   req.DisplayName,
		Status:        req.Status,
		AccessToken:   req.AccessToken,
		WebhookSecret: req.WebhookSecret,
		Metadata:      req.Metadata,
	}, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": toConnectionResponse(c)})
}

func (h *Handler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndConnection(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.DeleteConnection(r.Context(), user.ID, id, auditMeta(r)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) SyncConnection(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndConnection(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	var req syncRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	repos := make([]UpsertRepositoryInput, 0, len(req.Repositories))
	for _, rr := range req.Repositories {
		repos = append(repos, UpsertRepositoryInput{
			ExternalID: rr.ExternalID, FullName: rr.FullName, DefaultBranch: rr.DefaultBranch,
			CloneURL: rr.CloneURL, HTMLURL: rr.HTMLURL, Metadata: rr.Metadata,
		})
	}
	c, synced, err := h.svc.SyncConnection(r.Context(), user.ID, id, repos, auditMeta(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]repositoryResponse, 0, len(synced))
	for _, repo := range synced {
		out = append(out, toRepositoryResponse(repo))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": toConnectionResponse(c), "repositories": out})
}

func (h *Handler) ListRepositories(w http.ResponseWriter, r *http.Request) {
	user, id, err := actorAndConnection(r)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	page := pagination.FromRequest(r)
	items, total, err := h.svc.ListRepositories(r.Context(), user.ID, id, page.Limit, page.Offset)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]repositoryResponse, 0, len(items))
	for _, repo := range items {
		out = append(out, toRepositoryResponse(repo))
	}
	writeJSON(w, http.StatusOK, pagination.Page[repositoryResponse]{
		Items: out, Limit: page.Limit, Offset: page.Offset, TotalCount: &total,
	})
}

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("connectionId"))
	if err != nil {
		writeErr(w, r, apierror.Validation("invalid connection id", nil))
		return
	}
	if !h.limiter.Allow("git-webhook:"+id.String()+":"+clientIP(r), time.Now()) {
		writeErr(w, r, apierror.New(http.StatusTooManyRequests, apierror.CodeRateLimited, "rate limit exceeded"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, r, apierror.Validation("unable to read body", nil))
		return
	}
	_ = r.Body.Close()
	result, err := h.svc.HandleWebhook(r.Context(), id, r.Header, body)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	status := http.StatusOK
	writeJSON(w, status, map[string]any{
		"status":        result.Status,
		"duplicate":     result.Duplicate,
		"ignored":       result.Ignored,
		"message":       result.Message,
		"deploymentIds": uuidStrings(result.DeploymentIDs),
	})
}

func actorAndConnection(r *http.Request) (auth.User, uuid.UUID, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return auth.User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	id, err := uuid.Parse(r.PathValue("connectionId"))
	if err != nil {
		return auth.User{}, uuid.Nil, apierror.Validation("invalid connection id", nil)
	}
	return user, id, nil
}

func toConnectionResponse(c Connection) connectionResponse {
	out := connectionResponse{
		ID:               c.ID.String(),
		OrganizationID:   c.OrganizationID.String(),
		Provider:         c.Provider,
		AccountLogin:     c.AccountLogin,
		DisplayName:      c.DisplayName,
		Status:           c.Status,
		HasWebhookSecret: c.HasWebhookSecret,
		WebhookSecret:    c.WebhookSecretPlain,
		WebhookURLHint:   "/api/v1/webhooks/git/" + c.ID.String(),
		Metadata:         mapOrEmpty(c.Metadata),
		CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.LastSyncAt != nil {
		s := c.LastSyncAt.UTC().Format(time.RFC3339Nano)
		out.LastSyncAt = &s
	}
	if c.CreatedBy != nil {
		s := c.CreatedBy.String()
		out.CreatedBy = &s
	}
	return out
}

func toRepositoryResponse(repo Repository) repositoryResponse {
	out := repositoryResponse{
		ID:             repo.ID.String(),
		OrganizationID: repo.OrganizationID.String(),
		ConnectionID:   repo.ConnectionID.String(),
		ExternalID:     repo.ExternalID,
		FullName:       repo.FullName,
		DefaultBranch:  repo.DefaultBranch,
		CloneURL:       repo.CloneURL,
		HTMLURL:        repo.HTMLURL,
		Metadata:       mapOrEmpty(repo.Metadata),
		CreatedAt:      repo.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:      repo.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if repo.LastSyncAt != nil {
		s := repo.LastSyncAt.UTC().Format(time.RFC3339Nano)
		out.LastSyncAt = &s
	}
	return out
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
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
