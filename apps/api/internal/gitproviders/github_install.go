package gitproviders

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

const installStateTTL = 10 * time.Minute

type githubAppBinding struct {
	app config.GitHubAppConfig
	api GitHubAppAPI
}

// WithGitHubApp attaches the process GitHub App configuration and API client.
// The handler never receives the private key.
func (s *Service) WithGitHubApp(app config.GitHubAppConfig, api GitHubAppAPI) *Service {
	if api == nil {
		api = NewGitHubAppClient("")
	}
	s.github = &githubAppBinding{app: app, api: api}
	return s
}

// BeginInstallResult is the browser handoff for a GitHub App installation.
type BeginInstallResult struct {
	InstallationURL string
	ExpiresAt       time.Time
}

// CompleteInstallInput is the minimum GitHub setup redirect payload.
// Account identity is not accepted from the browser.
type CompleteInstallInput struct {
	InstallationID int64
	SetupAction    string
	State          string
}

func (s *Service) BeginGitHubInstallation(ctx context.Context, actorID, orgID uuid.UUID, meta AuditMeta) (BeginInstallResult, error) {
	if err := s.requireGitHubApp(); err != nil {
		return BeginInstallResult{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.GitConnectionManage); err != nil {
		return BeginInstallResult{}, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		raw, err := newInstallStateToken()
		if err != nil {
			return BeginInstallResult{}, apierror.Internal("could not create installation state")
		}
		expires := s.now().UTC().Add(installStateTTL)
		err = s.repo.InsertInstallState(ctx, sha256Hex(raw), orgID, actorID, expires)
		if errors.Is(err, ErrConflict) {
			continue
		}
		if err != nil {
			return BeginInstallResult{}, err
		}
		s.writeAudit(ctx, &orgID, &actorID, "git_connection.github_app_install_begin", "organization", orgID.String(), meta, nil, map[string]any{
			"provider": ProviderGitHub,
			"authMode": AuthModeGitHubApp,
			"result":   "pending",
		})
		return BeginInstallResult{
			InstallationURL: githubInstallURL(s.github.app.Slug, raw),
			ExpiresAt:       expires,
		}, nil
	}
	return BeginInstallResult{}, apierror.Internal("could not create installation state")
}

func (s *Service) CompleteGitHubInstallation(ctx context.Context, actorID uuid.UUID, in CompleteInstallInput, meta AuditMeta) (Connection, int, error) {
	if err := s.requireGitHubApp(); err != nil {
		return Connection{}, 0, err
	}
	if in.InstallationID <= 0 {
		return Connection{}, 0, apierror.Validation("installationId is invalid", nil)
	}
	action := strings.ToLower(strings.TrimSpace(in.SetupAction))
	if action != "install" && action != "update" {
		return Connection{}, 0, apierror.Validation("setupAction is invalid", nil)
	}
	rawState := strings.TrimSpace(in.State)
	if rawState == "" {
		return Connection{}, 0, apierror.Validation("installation state is required", nil)
	}
	hash := sha256Hex(rawState)
	st, err := s.repo.GetInstallState(ctx, hash)
	if err != nil {
		return Connection{}, 0, s.installStateError(err)
	}
	now := s.now().UTC()
	if st.UserID != actorID {
		return Connection{}, 0, apierror.Forbidden("installation state belongs to another user")
	}
	if st.OrganizationID == uuid.Nil {
		return Connection{}, 0, apierror.Validation("installation state is invalid", nil)
	}
	if st.ConsumedAt != nil {
		return Connection{}, 0, apierror.Conflict("installation state has already been used")
	}
	if !st.ExpiresAt.After(now) {
		return Connection{}, 0, apierror.Validation("installation state has expired", nil)
	}
	if err := s.authz.RequirePermission(ctx, actorID, st.OrganizationID, rbac.GitConnectionManage); err != nil {
		return Connection{}, 0, err
	}
	consumed, err := s.repo.ConsumeInstallState(ctx, hash, actorID, st.OrganizationID, now)
	if err != nil {
		return Connection{}, 0, err
	}
	if !consumed {
		return Connection{}, 0, s.classifyUnconsumed(ctx, hash, actorID, st.OrganizationID, now)
	}

	inst, err := s.verifyInstallation(ctx, in.InstallationID)
	if err != nil {
		s.auditInstallation(ctx, st.OrganizationID, actorID, in.InstallationID, "", action, "error", 0, meta)
		return Connection{}, 0, err
	}
	existing, err := s.repo.FindLiveConnectionByInstallation(ctx, inst.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Connection{}, 0, err
	}
	var current *Connection
	if err == nil {
		if existing.OrganizationID != st.OrganizationID {
			s.auditInstallation(ctx, st.OrganizationID, actorID, inst.ID, inst.AccountLogin, action, "rejected", 0, meta)
			return Connection{}, 0, apierror.Conflict("GitHub installation is already connected to another organization")
		}
		current = &existing
	}
	status := StatusError
	if current != nil {
		status = current.Status
	}
	if inst.Suspended {
		status = StatusDisabled
	}
	saved, err := s.persistInstallation(ctx, st.OrganizationID, actorID, current, inst, status)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			s.auditInstallation(ctx, st.OrganizationID, actorID, inst.ID, inst.AccountLogin, action, "rejected", 0, meta)
			return Connection{}, 0, apierror.Conflict("GitHub installation is already connected to another organization")
		}
		return Connection{}, 0, err
	}
	if inst.Suspended {
		s.auditInstallation(ctx, saved.OrganizationID, actorID, inst.ID, inst.AccountLogin, action, StatusDisabled, 0, meta)
		suspended := apierror.Conflict("GitHub installation is suspended")
		suspended.Details = map[string]any{"connectionId": saved.ID.String(), "status": StatusDisabled}
		return Connection{}, 0, suspended
	}
	synced, repos, err := s.pullInstallationRepositories(ctx, saved)
	if err != nil {
		s.auditInstallation(ctx, saved.OrganizationID, actorID, inst.ID, inst.AccountLogin, action, StatusError, 0, meta)
		return Connection{}, 0, withConnectionFailure(err, saved.ID)
	}
	s.auditInstallation(ctx, synced.OrganizationID, actorID, inst.ID, inst.AccountLogin, action, synced.Status, len(repos), meta)
	s.log.Info("github app installation completed",
		slog.String("connection_id", synced.ID.String()),
		slog.Int64("installation_id", inst.ID),
		slog.String("account_login", inst.AccountLogin),
		slog.String("status", synced.Status),
	)
	return synced, len(repos), nil
}

func (s *Service) syncGitHubAppConnection(ctx context.Context, actorID uuid.UUID, c Connection, meta AuditMeta) (Connection, []Repository, error) {
	if err := s.requireGitHubApp(); err != nil {
		return Connection{}, nil, err
	}
	if c.InstallationID == nil || *c.InstallationID <= 0 {
		return Connection{}, nil, apierror.Validation("github installation is missing", nil)
	}
	inst, err := s.verifyInstallation(ctx, *c.InstallationID)
	if err != nil {
		_ = s.repo.SetConnectionStatus(ctx, c.ID, StatusError)
		s.auditSync(ctx, c, actorID, StatusError, 0, meta)
		return Connection{}, nil, err
	}
	status := c.Status
	if inst.Suspended {
		status = StatusDisabled
	}
	saved, err := s.persistInstallation(ctx, c.OrganizationID, actorID, &c, inst, status)
	if err != nil {
		return Connection{}, nil, err
	}
	if inst.Suspended {
		s.auditSync(ctx, saved, actorID, StatusDisabled, 0, meta)
		suspended := apierror.Conflict("GitHub installation is suspended")
		suspended.Details = map[string]any{"connectionId": saved.ID.String(), "status": StatusDisabled}
		return Connection{}, nil, suspended
	}
	synced, repos, err := s.pullInstallationRepositories(ctx, saved)
	if err != nil {
		s.auditSync(ctx, saved, actorID, StatusError, 0, meta)
		return Connection{}, nil, withConnectionFailure(err, saved.ID)
	}
	s.auditSync(ctx, synced, actorID, synced.Status, len(repos), meta)
	return synced, repos, nil
}

func (s *Service) pullInstallationRepositories(ctx context.Context, c Connection) (Connection, []Repository, error) {
	installationToken, err := s.MintInstallationToken(ctx, *c.InstallationID)
	if err != nil {
		_ = s.repo.SetConnectionStatus(ctx, c.ID, StatusError)
		return Connection{}, nil, err
	}
	remote, err := s.github.api.ListInstallationRepositories(ctx, installationToken)
	installationToken = ""
	if err != nil {
		_ = s.repo.SetConnectionStatus(ctx, c.ID, StatusError)
		return Connection{}, nil, s.mapGitHub(err)
	}
	inputs, err := installationRepositoryInputs(remote)
	if err != nil {
		_ = s.repo.SetConnectionStatus(ctx, c.ID, StatusError)
		return Connection{}, nil, apierror.New(http.StatusBadGateway, apierror.CodeServiceUnavailable, "GitHub repository sync failed")
	}
	repos, err := s.repo.ReconcileInstallationRepositories(ctx, c.OrganizationID, c.ID, inputs, StatusActive, s.now().UTC())
	if err != nil {
		_ = s.repo.SetConnectionStatus(ctx, c.ID, StatusError)
		s.log.Error("github repository reconcile failed", slog.String("connection_id", c.ID.String()))
		return Connection{}, nil, apierror.New(http.StatusServiceUnavailable, apierror.CodeServiceUnavailable, "GitHub repository sync failed")
	}
	updated, _, err := s.repo.GetConnection(ctx, c.ID)
	if err != nil {
		return Connection{}, nil, err
	}
	return updated, repos, nil
}

func (s *Service) verifyInstallation(ctx context.Context, installationID int64) (GitHubInstallation, error) {
	appJWT, err := s.githubAppJWT()
	if err != nil {
		return GitHubInstallation{}, err
	}
	inst, err := s.github.api.GetInstallation(ctx, appJWT, installationID)
	appJWT = ""
	if err != nil {
		return GitHubInstallation{}, s.mapGitHub(err)
	}
	if inst.ID != installationID {
		return GitHubInstallation{}, apierror.Validation("GitHub installation did not match the requested installation", nil)
	}
	if strconv.FormatInt(inst.AppID, 10) != s.github.app.AppID {
		return GitHubInstallation{}, apierror.Conflict("GitHub installation belongs to a different App")
	}
	if inst.AccountType != AccountTypeUser && inst.AccountType != AccountTypeOrganization {
		return GitHubInstallation{}, apierror.Validation("GitHub installation account type is not supported", nil)
	}
	if inst.RepositorySelection != RepositorySelectionAll && inst.RepositorySelection != RepositorySelectionSelected {
		return GitHubInstallation{}, apierror.Validation("GitHub installation repository selection is not supported", nil)
	}
	return inst, nil
}

func (s *Service) persistInstallation(ctx context.Context, orgID, actorID uuid.UUID, existing *Connection, inst GitHubInstallation, status string) (Connection, error) {
	installationID := inst.ID
	c := Connection{
		OrganizationID:      orgID,
		Provider:            ProviderGitHub,
		AuthMode:            AuthModeGitHubApp,
		InstallationID:      &installationID,
		AccountID:           strconv.FormatInt(inst.AccountID, 10),
		AccountType:         inst.AccountType,
		RepositorySelection: inst.RepositorySelection,
		AccountLogin:        inst.AccountLogin,
		DisplayName:         inst.AccountLogin,
		Status:              status,
		Metadata:            map[string]any{},
	}
	if existing == nil {
		return s.repo.CreateGitHubAppConnection(ctx, c, actorID)
	}
	if existing.OrganizationID != orgID {
		return Connection{}, ErrConflict
	}
	c.ID = existing.ID
	return s.repo.UpdateGitHubAppConnection(ctx, c)
}

// MintInstallationToken requests a short-lived GitHub installation token.
// The token is returned to the caller and is not cached, stored, or logged.
func (s *Service) MintInstallationToken(ctx context.Context, installationID int64) (string, error) {
	if installationID <= 0 {
		return "", apierror.Validation("github installation is missing", nil)
	}
	appJWT, err := s.githubAppJWT()
	if err != nil {
		return "", err
	}
	token, _, err := s.github.api.CreateInstallationToken(ctx, appJWT, installationID)
	appJWT = ""
	if err != nil {
		return "", s.mapGitHub(err)
	}
	if token == "" {
		return "", apierror.New(http.StatusBadGateway, apierror.CodeServiceUnavailable, "GitHub API request failed")
	}
	return token, nil
}

// resolveGitHubAppCloneCredential mints a clone credential for an active GitHub App connection.
// It never decrypts credential_ciphertext.
func (s *Service) resolveGitHubAppCloneCredential(ctx context.Context, conn Connection) (string, string, error) {
	if conn.InstallationID == nil || *conn.InstallationID <= 0 {
		_ = s.repo.SetConnectionStatus(ctx, conn.ID, StatusError)
		return "", "", apierror.Validation("github installation is missing", nil)
	}
	inst, err := s.verifyInstallation(ctx, *conn.InstallationID)
	if err != nil {
		s.reflectCloneInstallationFailure(ctx, conn.ID, err)
		return "", "", err
	}
	if inst.Suspended {
		_ = s.repo.SetConnectionStatus(ctx, conn.ID, StatusDisabled)
		return "", "", apierror.Conflict("git connection is not active")
	}
	token, err := s.MintInstallationToken(ctx, inst.ID)
	if err != nil {
		s.reflectCloneInstallationFailure(ctx, conn.ID, err)
		return "", "", err
	}
	return "x-access-token", token, nil
}

// reflectCloneInstallationFailure records a definite installation failure.
// Rate limits, authentication failures, and network errors leave the connection status unchanged.
func (s *Service) reflectCloneInstallationFailure(ctx context.Context, connectionID uuid.UUID, err error) {
	apiErr, ok := apierror.AsAPIError(err)
	if !ok || apiErr.Status != http.StatusNotFound {
		return
	}
	_ = s.repo.SetConnectionStatus(ctx, connectionID, StatusError)
}

func (s *Service) githubAppJWT() (string, error) {
	if err := s.requireGitHubApp(); err != nil {
		return "", err
	}
	pem := s.github.app.PrivateKeyPEM()
	signed, err := signGitHubAppJWT(s.github.app.AppID, pem, s.now().UTC())
	for i := range pem {
		pem[i] = 0
	}
	return signed, err
}

func (s *Service) requireGitHubApp() error {
	if s.github == nil || !s.github.app.Configured || s.github.api == nil {
		return apierror.New(http.StatusServiceUnavailable, apierror.CodeServiceUnavailable, "GitHub App is not configured")
	}
	return nil
}

func (s *Service) mapGitHub(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := apierror.AsAPIError(err); ok {
		return err
	}
	var status *githubStatusError
	if errors.As(err, &status) {
		switch {
		case status.RateLimited:
			return apierror.New(http.StatusTooManyRequests, apierror.CodeRateLimited, "GitHub rate limit reached")
		case status.Status == http.StatusNotFound:
			return apierror.NotFound("GitHub installation not found")
		case status.Status == http.StatusUnauthorized || status.Status == http.StatusForbidden:
			return apierror.New(http.StatusBadGateway, apierror.CodeServiceUnavailable, "GitHub App authentication failed")
		default:
			return apierror.New(http.StatusBadGateway, apierror.CodeServiceUnavailable, "GitHub API request failed")
		}
	}
	s.log.Error("github api request failed")
	return apierror.New(http.StatusBadGateway, apierror.CodeServiceUnavailable, "GitHub API request failed")
}

func (s *Service) installStateError(err error) error {
	if errors.Is(err, ErrInstallStateNotFound) {
		return apierror.Validation("installation state is invalid", nil)
	}
	return err
}

func (s *Service) classifyUnconsumed(ctx context.Context, hash string, actorID, orgID uuid.UUID, now time.Time) error {
	st, err := s.repo.GetInstallState(ctx, hash)
	if err != nil {
		return s.installStateError(err)
	}
	if st.UserID != actorID {
		return apierror.Forbidden("installation state belongs to another user")
	}
	if st.OrganizationID != orgID {
		return apierror.Forbidden("installation state belongs to another organization")
	}
	if st.ConsumedAt != nil {
		return apierror.Conflict("installation state has already been used")
	}
	if !st.ExpiresAt.After(now) {
		return apierror.Validation("installation state has expired", nil)
	}
	return apierror.Conflict("installation state has already been used")
}

func (s *Service) auditInstallation(ctx context.Context, orgID, actorID uuid.UUID, installationID int64, accountLogin, setupAction, status string, repositoryCount int, meta AuditMeta) {
	s.writeAudit(ctx, &orgID, &actorID, "git_connection.github_app_install", "git_connection", "", meta, nil, map[string]any{
		"provider":        ProviderGitHub,
		"authMode":        AuthModeGitHubApp,
		"accountLogin":    accountLogin,
		"installationId":  installationID,
		"setupAction":     setupAction,
		"status":          status,
		"repositoryCount": repositoryCount,
	})
}

func (s *Service) auditSync(ctx context.Context, c Connection, actorID uuid.UUID, status string, repositoryCount int, meta AuditMeta) {
	var installationID int64
	if c.InstallationID != nil {
		installationID = *c.InstallationID
	}
	s.writeAudit(ctx, &c.OrganizationID, &actorID, "git_connection.sync", "git_connection", c.ID.String(), meta, nil, map[string]any{
		"provider":        c.Provider,
		"authMode":        AuthModeGitHubApp,
		"accountLogin":    c.AccountLogin,
		"installationId":  installationID,
		"status":          status,
		"repositoryCount": repositoryCount,
	})
}

func withConnectionFailure(err error, connectionID uuid.UUID) error {
	apiErr, ok := apierror.AsAPIError(err)
	if !ok {
		return err
	}
	if apiErr.Details == nil {
		apiErr.Details = map[string]any{}
	}
	apiErr.Details["connectionId"] = connectionID.String()
	apiErr.Details["status"] = StatusError
	return apiErr
}

func newInstallStateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func githubInstallURL(slug, state string) string {
	return "https://github.com/apps/" + url.PathEscape(slug) + "/installations/new?state=" + url.QueryEscape(state)
}

func installationRepositoryInputs(remote []GitHubRepository) ([]UpsertRepositoryInput, error) {
	out := make([]UpsertRepositoryInput, 0, len(remote))
	seenID := make(map[string]struct{}, len(remote))
	for _, repo := range remote {
		if repo.ID <= 0 || repo.FullName == "" {
			return nil, errors.New("incomplete github repository")
		}
		externalID := strconv.FormatInt(repo.ID, 10)
		if _, ok := seenID[externalID]; ok {
			return nil, errors.New("duplicate github repository id")
		}
		seenID[externalID] = struct{}{}
		cloneURL, err := credentialFreeHTTPS(repo.CloneURL)
		if err != nil {
			return nil, err
		}
		branch := repo.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		out = append(out, UpsertRepositoryInput{
			ExternalID:    externalID,
			FullName:      repo.FullName,
			DefaultBranch: branch,
			CloneURL:      cloneURL,
			HTMLURL:       credentialFreeURL(repo.HTMLURL),
			Metadata: map[string]any{
				"private":    repo.Private,
				"archived":   repo.Archived,
				"visibility": repo.Visibility,
			},
		})
	}
	return out, nil
}

func credentialFreeHTTPS(raw string) (string, error) {
	cleaned := credentialFreeURL(raw)
	u, err := url.Parse(cleaned)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", errors.New("clone url must be https")
	}
	return u.String(), nil
}

func credentialFreeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	return u.String()
}
